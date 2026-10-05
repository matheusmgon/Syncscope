package kubestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"syncscope/internal/kube"
)

// Object returns the raw object (without managedFields) for detail views.
func (m *Manager) Object(key string) (map[string]any, error) {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	u, err := c.client.Get(ctx, kube.Kinds[kind], ns, name)
	if err != nil {
		return nil, err
	}
	o := u.Object
	kube.StripNoise(o)
	return o, nil
}

func (m *Manager) YAML(key string) (string, error) {
	o, err := m.Object(key)
	if err != nil {
		return "", err
	}
	b, err := yaml.Marshal(o)
	return string(b), err
}

// SaveYAML replaces the object with the edited YAML (optimistic concurrency
// through metadata.resourceVersion).
func (m *Manager) SaveYAML(key, y string) error {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return err
	}
	var o map[string]any
	if err := yaml.Unmarshal([]byte(y), &o); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	u := &unstructured.Unstructured{Object: o}
	if u.GetName() != name || u.GetNamespace() != ns {
		return errors.New("name and namespace cannot be changed")
	}
	ctx, cancel := opCtx()
	defer cancel()
	_, err = c.client.Dyn.Resource(kube.Kinds[kind]).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func (m *Manager) DeleteObject(key string) error {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.Delete(ctx, kube.Kinds[kind], ns, name)
}

// ---- pods / logs / events -------------------------------------------------------------

func (m *Manager) selectorFor(c *kconn, key, kind, name string) string {
	switch kind {
	case "Workflow":
		return "workflows.argoproj.io/workflow=" + name
	case "EventSource":
		return "eventsource-name=" + name
	case "Sensor":
		return "sensor-name=" + name
	case "EventBus":
		return "eventbus-name=" + name
	case "Rollout":
		c.mu.RLock()
		defer c.mu.RUnlock()
		if s, ok := c.sums[key]; ok {
			if sel, _ := s.Fields["selector"].(string); sel != "" {
				return sel
			}
		}
	}
	return ""
}

func (m *Manager) Pods(key string) ([]kube.PodInfo, error) {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	sel := m.selectorFor(c, key, kind, name)
	if sel == "" {
		return nil, nil
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.Pods(ctx, ns, sel)
}

func (m *Manager) Events(key string) ([]kube.Event, error) {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.Events(ctx, ns, kind, name)
}

type LogRequest struct {
	Ctx       string `json:"ctx"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	TailLines int64  `json:"tailLines"`
	Follow    bool   `json:"follow"`
	Previous  bool   `json:"previous"`
}

// StartLogs streams a pod's logs as "logs" events {id, lines, done, error} —
// the same protocol the Argo CD logs use, so the UI viewer is shared.
func (m *Manager) StartLogs(r LogRequest) (string, error) {
	m.mu.RLock()
	c := m.conns[r.Ctx]
	m.mu.RUnlock()
	if c == nil || c.client == nil {
		return "", errors.New("cluster " + r.Ctx + " is not connected")
	}
	id := fmt.Sprintf("klog-%d", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	m.logs.mu.Lock()
	if m.logs.cancel == nil {
		m.logs.cancel = map[string]context.CancelFunc{}
	}
	m.logs.cancel[id] = cancel
	m.logs.mu.Unlock()
	if r.TailLines == 0 {
		r.TailLines = 500
	}
	go func() {
		defer m.StopLogs(id)
		type line struct {
			Pod     string `json:"pod"`
			Time    string `json:"time"`
			Content string `json:"content"`
		}
		var buf []line
		last := time.Now()
		flush := func() {
			if len(buf) > 0 {
				m.emit("logs", map[string]any{"id": id, "lines": buf})
				buf = nil
			}
			last = time.Now()
		}
		err := c.client.Logs(ctx, r.Namespace, r.Pod, kube.LogOptions{Container: r.Container, TailLines: r.TailLines, Follow: r.Follow, Previous: r.Previous},
			func(ts, l string) {
				buf = append(buf, line{Pod: r.Pod, Time: ts, Content: l})
				if len(buf) >= 500 || time.Since(last) > 150*time.Millisecond {
					flush()
				}
			})
		flush()
		msg := ""
		if err != nil && ctx.Err() == nil {
			msg = err.Error()
		}
		m.emit("logs", map[string]any{"id": id, "done": true, "error": msg})
	}()
	return id, nil
}

func (m *Manager) StopLogs(id string) {
	m.logs.mu.Lock()
	defer m.logs.mu.Unlock()
	if cancel := m.logs.cancel[id]; cancel != nil {
		cancel()
		delete(m.logs.cancel, id)
	}
}

// ---- Argo Rollouts actions (same patches as `kubectl argo rollouts`) ---------------

func (m *Manager) RolloutAction(key, action string) error {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return err
	}
	if kind != "Rollout" {
		return errors.New("not a Rollout")
	}
	ctx, cancel := opCtx()
	defer cancel()
	r := kube.Rollouts
	patch := func(p string, sub string) error { return c.client.MergePatch(ctx, r, ns, name, []byte(p), sub) }
	switch action {
	case "promote":
		u, err := c.client.Get(ctx, r, ns, name)
		if err != nil {
			return err
		}
		if err := patch(`{"spec":{"paused":false}}`, ""); err != nil {
			return err
		}
		steps := slice(u.Object, "spec", "strategy", "canary", "steps")
		idx := num(u.Object, "status", "currentStepIndex")
		if len(steps) > 0 && idx < int64(len(steps)) {
			return patch(fmt.Sprintf(`{"status":{"pauseConditions":null,"currentStepIndex":%d}}`, idx+1), "status")
		}
		return patch(`{"status":{"pauseConditions":null}}`, "status")
	case "promote-full":
		return patch(`{"status":{"promoteFull":true}}`, "status")
	case "abort":
		return patch(`{"status":{"abort":true}}`, "status")
	case "retry":
		return patch(`{"status":{"abort":false}}`, "status")
	case "restart":
		return patch(fmt.Sprintf(`{"spec":{"restartAt":%q}}`, time.Now().UTC().Format(time.RFC3339)), "")
	case "pause":
		return patch(`{"spec":{"paused":true}}`, "")
	}
	return fmt.Errorf("unknown rollout action %q", action)
}

// ---- Argo Workflows actions ---------------------------------------------------------

func (m *Manager) WorkflowAction(key, action string) (string, error) {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	if kind != "Workflow" {
		return "", errors.New("not a Workflow")
	}
	ctx, cancel := opCtx()
	defer cancel()
	r := kube.Workflows
	patch := func(p string) error { return c.client.MergePatch(ctx, r, ns, name, []byte(p), "") }
	switch action {
	case "stop":
		return "", patch(`{"spec":{"shutdown":"Stop"}}`)
	case "terminate":
		return "", patch(`{"spec":{"shutdown":"Terminate"}}`)
	case "suspend":
		return "", patch(`{"spec":{"suspend":true}}`)
	case "resume":
		// un-suspend the workflow and complete running suspend nodes, like `argo resume`
		u, err := c.client.Get(ctx, r, ns, name)
		if err != nil {
			return "", err
		}
		nodes := map[string]any{}
		now := time.Now().UTC().Format(time.RFC3339)
		for id, n := range obj(u.Object, "status", "nodes") {
			nm, _ := n.(map[string]any)
			if str(nm, "type") == "Suspend" && str(nm, "phase") == "Running" {
				nodes[id] = map[string]any{"phase": "Succeeded", "finishedAt": now, "message": "Resumed by Syncscope"}
			}
		}
		p := map[string]any{"spec": map[string]any{"suspend": nil}}
		if len(nodes) > 0 {
			p["status"] = map[string]any{"nodes": nodes}
		}
		b, _ := json.Marshal(p)
		return "", patch(string(b))
	case "resubmit":
		u, err := c.client.Get(ctx, r, ns, name)
		if err != nil {
			return "", err
		}
		spec, _ := u.Object["spec"].(map[string]any)
		delete(spec, "shutdown")
		delete(spec, "suspend")
		labels := map[string]any{"workflows.argoproj.io/resubmitted-from-workflow": name}
		for k, v := range u.GetLabels() {
			if strings.HasPrefix(k, "workflows.argoproj.io/workflow-template") || strings.HasPrefix(k, "workflows.argoproj.io/cron-workflow") || !strings.HasPrefix(k, "workflows.argoproj.io/") {
				labels[k] = v
			}
		}
		return m.create(ctx, c, ns, baseName(name)+"-", labels, spec)
	}
	return "", fmt.Errorf("unknown workflow action %q", action)
}

func baseName(n string) string {
	// strip the random suffix added by generateName (e.g. "build-x7k2p" -> "build")
	if i := strings.LastIndexByte(n, '-'); i > 0 && len(n)-i == 6 {
		return n[:i]
	}
	return n
}

func (m *Manager) create(ctx context.Context, c *kconn, ns, generateName string, labels map[string]any, spec map[string]any) (string, error) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Workflow",
		"metadata": map[string]any{"generateName": generateName, "namespace": ns, "labels": labels},
		"spec":     spec,
	}}
	out, err := c.client.Create(ctx, kube.Workflows, ns, u)
	if err != nil {
		return "", err
	}
	return objKey(c.name, "Workflow", ns, out.GetName()), nil
}

// SubmitTemplate creates a Workflow from a WorkflowTemplate with parameter overrides.
func (m *Manager) SubmitTemplate(key string, params map[string]string) (string, error) {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	if kind != "WorkflowTemplate" {
		return "", errors.New("not a WorkflowTemplate")
	}
	ctx, cancel := opCtx()
	defer cancel()
	spec := map[string]any{"workflowTemplateRef": map[string]any{"name": name}}
	if len(params) > 0 {
		var ps []any
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ps = append(ps, map[string]any{"name": k, "value": params[k]})
		}
		spec["arguments"] = map[string]any{"parameters": ps}
	}
	return m.create(ctx, c, ns, name+"-", map[string]any{"workflows.argoproj.io/workflow-template": name}, spec)
}

func (m *Manager) CronAction(key, action string) (string, error) {
	c, kind, ns, name, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	if kind != "CronWorkflow" {
		return "", errors.New("not a CronWorkflow")
	}
	ctx, cancel := opCtx()
	defer cancel()
	switch action {
	case "suspend":
		return "", c.client.MergePatch(ctx, kube.CronWorkflows, ns, name, []byte(`{"spec":{"suspend":true}}`), "")
	case "resume":
		return "", c.client.MergePatch(ctx, kube.CronWorkflows, ns, name, []byte(`{"spec":{"suspend":false}}`), "")
	case "submit":
		u, err := c.client.Get(ctx, kube.CronWorkflows, ns, name)
		if err != nil {
			return "", err
		}
		spec := obj(u.Object, "spec", "workflowSpec")
		if spec == nil {
			return "", errors.New("cron workflow has no workflowSpec")
		}
		return m.create(ctx, c, ns, name+"-", map[string]any{"workflows.argoproj.io/cron-workflow": name}, spec)
	}
	return "", fmt.Errorf("unknown cron action %q", action)
}

// RestartPods deletes the pods of an EventSource / Sensor / EventBus so their
// controller recreates them (the usual way to restart them).
func (m *Manager) RestartPods(key string) (int, error) {
	pods, err := m.Pods(key)
	if err != nil {
		return 0, err
	}
	c, _, ns, _, err := m.resolve(key)
	if err != nil {
		return 0, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	n := 0
	for _, p := range pods {
		if err := c.client.Core.CoreV1().Pods(ns).Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
