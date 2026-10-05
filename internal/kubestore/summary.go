package kubestore

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Problem mirrors store.Problem (same JSON) for Kubernetes-backed objects.
type Problem struct {
	Severity string `json:"severity"`
	Source   string `json:"source"`
	Resource string `json:"resource,omitempty"`
	Message  string `json:"message"`
}

// Obj is the UI summary of one Argo object read from Kubernetes.
type Obj struct {
	Key       string            `json:"key"` // ctx|Kind|ns/name
	Ctx       string            `json:"ctx"`
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Phase     string            `json:"phase"`
	Message   string            `json:"message,omitempty"`
	Severity  int               `json:"severity"`
	Problems  []Problem         `json:"problems,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Created   string            `json:"created"`
	Fields    map[string]any    `json:"fields"`
}

func objKey(ctx, kind, ns, name string) string { return ctx + "|" + kind + "|" + ns + "/" + name }

// ---- tiny helpers over unstructured maps ----------------------------------------

func str(o map[string]any, path ...string) string {
	v, _, _ := unstructured.NestedFieldNoCopy(o, path...)
	switch x := v.(type) {
	case string:
		return x
	case int64:
		return fmt.Sprint(x)
	case float64:
		return fmt.Sprint(int64(x))
	case bool:
		return fmt.Sprint(x)
	}
	return ""
}

func num(o map[string]any, path ...string) int64 {
	v, _, _ := unstructured.NestedFieldNoCopy(o, path...)
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case int:
		return int64(x)
	}
	return 0
}

func boolean(o map[string]any, path ...string) bool {
	v, _, _ := unstructured.NestedFieldNoCopy(o, path...)
	b, _ := v.(bool)
	return b
}

func slice(o map[string]any, path ...string) []any {
	v, _, _ := unstructured.NestedFieldNoCopy(o, path...)
	s, _ := v.([]any)
	return s
}

func obj(o map[string]any, path ...string) map[string]any {
	v, _, _ := unstructured.NestedFieldNoCopy(o, path...)
	m, _ := v.(map[string]any)
	return m
}

func conditions(o map[string]any) []map[string]any {
	var out []map[string]any
	for _, c := range slice(o, "status", "conditions") {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func finalize(s *Obj) {
	sort.SliceStable(s.Problems, func(i, j int) bool { return s.Problems[i].Severity == "error" && s.Problems[j].Severity != "error" })
	for _, p := range s.Problems {
		if p.Severity == "error" {
			s.Severity = 2
			break
		}
		s.Severity = 1
	}
}

// Summarize builds the summary for any supported kind.
func Summarize(ctxName, kind string, u *unstructured.Unstructured) Obj {
	o := u.Object
	s := Obj{Key: objKey(ctxName, kind, u.GetNamespace(), u.GetName()), Ctx: ctxName, Kind: kind, Namespace: u.GetNamespace(),
		Name: u.GetName(), Labels: u.GetLabels(), Created: u.GetCreationTimestamp().Format(time.RFC3339), Fields: map[string]any{}}
	switch kind {
	case "Rollout":
		rollout(&s, o, u)
	case "Workflow":
		workflow(&s, o)
	case "WorkflowTemplate":
		wfTemplate(&s, o)
	case "CronWorkflow":
		cron(&s, o)
	case "EventSource":
		eventSource(&s, o)
	case "Sensor":
		sensor(&s, o)
	case "EventBus":
		eventBus(&s, o)
	}
	if s.Phase == "" {
		s.Phase = "Unknown"
	}
	finalize(&s)
	return s
}

// ---- Argo Rollouts ------------------------------------------------------------------

func rollout(s *Obj, o map[string]any, u *unstructured.Unstructured) {
	f := s.Fields
	s.Phase, s.Message = str(o, "status", "phase"), str(o, "status", "message")
	strategy := "canary"
	steps := slice(o, "spec", "strategy", "canary", "steps")
	if obj(o, "spec", "strategy", "blueGreen") != nil {
		strategy = "blueGreen"
	}
	f["strategy"] = strategy
	idx := num(o, "status", "currentStepIndex")
	f["step"] = idx
	f["steps"] = len(steps)
	// weight reached so far: last setWeight at or before the current step
	weight := int64(-1)
	for i, st := range steps {
		if int64(i) > idx {
			break
		}
		if m, ok := st.(map[string]any); ok {
			if w, ok := m["setWeight"]; ok {
				weight = num(map[string]any{"w": w}, "w")
			}
		}
	}
	if w := num(o, "status", "canary", "weights", "canary", "weight"); w > 0 {
		weight = w
	}
	if boolean(o, "status", "abort") {
		weight = 0 // aborted: all traffic is back on the stable version
	}
	f["weight"] = weight
	for _, k := range []string{"replicas", "readyReplicas", "updatedReplicas", "availableReplicas"} {
		f[k] = num(o, "status", k)
	}
	f["desired"] = num(o, "spec", "replicas")
	f["stableRS"] = str(o, "status", "stableRS")
	f["currentPodHash"] = str(o, "status", "currentPodHash")
	f["paused"] = boolean(o, "spec", "paused") || len(slice(o, "status", "pauseConditions")) > 0
	f["aborted"] = boolean(o, "status", "abort")
	f["revision"] = u.GetAnnotations()["rollout.argoproj.io/revision"]
	var images []string
	for _, c := range slice(o, "spec", "template", "spec", "containers") {
		if m, ok := c.(map[string]any); ok {
			images = append(images, str(m, "image"))
		}
	}
	f["images"] = images
	if ref := obj(o, "spec", "workloadRef"); ref != nil {
		f["workloadRef"] = str(ref, "kind") + "/" + str(ref, "name")
	}
	f["argoApp"] = argoApp(u)
	if sel := obj(o, "spec", "selector", "matchLabels"); sel != nil {
		var parts []string
		for k, v := range sel {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(parts)
		f["selector"] = strings.Join(parts, ",")
	}
	var reasons []string
	for _, pc := range slice(o, "status", "pauseConditions") {
		if m, ok := pc.(map[string]any); ok {
			reasons = append(reasons, str(m, "reason"))
		}
	}
	f["pauseReasons"] = reasons

	if boolean(o, "status", "abort") {
		msg := s.Message
		if msg == "" {
			msg = "rollout aborted"
		}
		s.Problems = append(s.Problems, Problem{Severity: "error", Source: "rollout", Message: "aborted: " + msg})
	} else if s.Phase == "Degraded" {
		s.Problems = append(s.Problems, Problem{Severity: "error", Source: "rollout", Message: nonEmpty(s.Message, "rollout degraded")})
	} else if s.Phase == "Paused" {
		msg := fmt.Sprintf("paused at step %d/%d (%s) — waiting for promotion", idx, len(steps), strings.Join(reasons, ", "))
		if strategy == "blueGreen" {
			msg = "new version is up on the preview service — waiting for promotion (" + strings.Join(reasons, ", ") + ")"
		} else if len(reasons) == 0 {
			msg = fmt.Sprintf("paused at step %d/%d — waiting for promotion", idx, len(steps))
		}
		s.Problems = append(s.Problems, Problem{Severity: "warning", Source: "rollout", Message: msg})
	}
	for _, c := range conditions(o) {
		t, st := str(c, "type"), str(c, "status")
		if (t == "InvalidSpec" && st == "True") || (t == "ReplicaFailure" && st == "True") || (t == "Progressing" && str(c, "reason") == "ProgressDeadlineExceeded") {
			s.Problems = append(s.Problems, Problem{Severity: "error", Source: "condition", Resource: t, Message: str(c, "message")})
		}
	}
}

// argoApp finds the Argo CD application managing an object (tracking id or label).
func argoApp(u *unstructured.Unstructured) string {
	if t := u.GetAnnotations()["argocd.argoproj.io/tracking-id"]; t != "" {
		if i := strings.IndexByte(t, ':'); i > 0 {
			return t[:i]
		}
	}
	return u.GetLabels()["app.kubernetes.io/instance"]
}

// ---- Argo Workflows -----------------------------------------------------------------

type wfNode struct {
	ID, Name, DisplayName, Type, Phase, Message, TemplateName string
}

func workflow(s *Obj, o map[string]any) {
	f := s.Fields
	s.Phase, s.Message = str(o, "status", "phase"), str(o, "status", "message")
	if s.Phase == "" {
		s.Phase = "Pending"
	}
	f["progress"] = str(o, "status", "progress")
	f["startedAt"] = str(o, "status", "startedAt")
	f["finishedAt"] = str(o, "status", "finishedAt")
	f["entrypoint"] = str(o, "spec", "entrypoint")
	f["template"] = str(o, "spec", "workflowTemplateRef", "name")
	if f["template"] == "" {
		f["template"] = s.Labels["workflows.argoproj.io/workflow-template"]
	}
	f["cron"] = s.Labels["workflows.argoproj.io/cron-workflow"]
	f["suspended"] = boolean(o, "spec", "suspend")
	f["shutdown"] = str(o, "spec", "shutdown")
	nodes := obj(o, "status", "nodes")
	f["nodes"] = len(nodes)
	var failed []wfNode
	for id, n := range nodes {
		m, _ := n.(map[string]any)
		if m == nil {
			continue
		}
		ph := str(m, "phase")
		if (ph == "Failed" || ph == "Error") && str(m, "type") == "Pod" {
			failed = append(failed, wfNode{ID: id, DisplayName: str(m, "displayName"), Phase: ph, Message: str(m, "message"), TemplateName: str(m, "templateName")})
		}
	}
	sort.Slice(failed, func(i, j int) bool { return failed[i].DisplayName < failed[j].DisplayName })
	f["failedNodes"] = len(failed)
	if s.Phase == "Failed" || s.Phase == "Error" {
		// the failed steps carry the real cause; the workflow message ("child X failed") goes last
		defer func() {
			s.Problems = append(s.Problems, Problem{Severity: "error", Source: "workflow", Message: nonEmpty(s.Message, "workflow "+strings.ToLower(s.Phase))})
		}()
		for i, n := range failed {
			if i == 10 {
				s.Problems = append(s.Problems, Problem{Severity: "error", Source: "workflow", Message: fmt.Sprintf("… and %d more failed steps", len(failed)-10)})
				break
			}
			s.Problems = append(s.Problems, Problem{Severity: "error", Source: "step", Resource: n.DisplayName, Message: nonEmpty(n.Message, n.Phase)})
		}
	}
	if boolean(o, "spec", "suspend") && s.Phase == "Running" {
		s.Problems = append(s.Problems, Problem{Severity: "warning", Source: "workflow", Message: "suspended — waiting to be resumed"})
	}
}

func wfTemplate(s *Obj, o map[string]any) {
	f := s.Fields
	s.Phase = "Ready"
	f["entrypoint"] = str(o, "spec", "entrypoint")
	f["templates"] = len(slice(o, "spec", "templates"))
	f["params"] = params(slice(o, "spec", "arguments", "parameters"))
	f["description"] = s.annotation(o, "workflows.argoproj.io/description")
}

func (s *Obj) annotation(o map[string]any, k string) string {
	return str(o, "metadata", "annotations", k)
}

func params(list []any) []map[string]any {
	var out []map[string]any
	for _, p := range list {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		e := map[string]any{"name": str(m, "name"), "value": str(m, "value"), "description": str(m, "description")}
		if en := slice(m, "enum"); len(en) > 0 {
			e["enum"] = en
		}
		out = append(out, e)
	}
	return out
}

func cron(s *Obj, o map[string]any) {
	f := s.Fields
	schedule := str(o, "spec", "schedule")
	if schedule == "" {
		var ss []string
		for _, x := range slice(o, "spec", "schedules") {
			if v, ok := x.(string); ok {
				ss = append(ss, v)
			}
		}
		schedule = strings.Join(ss, " | ")
	}
	f["schedule"] = schedule
	f["timezone"] = str(o, "spec", "timezone")
	f["suspended"] = boolean(o, "spec", "suspend")
	f["concurrencyPolicy"] = str(o, "spec", "concurrencyPolicy")
	f["lastScheduled"] = str(o, "status", "lastScheduledTime")
	f["active"] = len(slice(o, "status", "active"))
	f["succeeded"] = num(o, "status", "succeeded")
	f["failed"] = num(o, "status", "failed")
	f["template"] = str(o, "spec", "workflowSpec", "workflowTemplateRef", "name")
	f["params"] = params(slice(o, "spec", "workflowSpec", "arguments", "parameters"))
	s.Phase = "Active"
	if boolean(o, "spec", "suspend") {
		s.Phase = "Suspended"
	}
	for _, c := range conditions(o) {
		if str(c, "status") == "True" && (str(c, "type") == "SubmissionError" || str(c, "type") == "SpecError") {
			s.Problems = append(s.Problems, Problem{Severity: "error", Source: "condition", Resource: str(c, "type"), Message: str(c, "message")})
		}
	}
}

// ---- Argo Events --------------------------------------------------------------------

var esNonSources = map[string]bool{"template": true, "service": true, "eventBusName": true, "replicas": true}

func eventSource(s *Obj, o map[string]any) {
	f := s.Fields
	spec := obj(o, "spec")
	type src struct {
		Type   string   `json:"type"`
		Events []string `json:"events"`
	}
	var sources []src
	for k, v := range spec {
		if esNonSources[k] {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		var evs []string
		for name := range m {
			evs = append(evs, name)
		}
		sort.Strings(evs)
		sources = append(sources, src{Type: k, Events: evs})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Type < sources[j].Type })
	f["sources"] = sources
	f["eventBusName"] = nonEmpty(str(o, "spec", "eventBusName"), "default")
	conditionsHealth(s, o)
}

func sensor(s *Obj, o map[string]any) {
	f := s.Fields
	var deps []map[string]string
	for _, d := range slice(o, "spec", "dependencies") {
		if m, ok := d.(map[string]any); ok {
			deps = append(deps, map[string]string{"name": str(m, "name"), "eventSourceName": str(m, "eventSourceName"), "eventName": str(m, "eventName")})
		}
	}
	var triggers []map[string]string
	for _, t := range slice(o, "spec", "triggers") {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		tpl := obj(m, "template")
		typ := ""
		for k := range tpl {
			if k != "name" && k != "conditions" && k != "conditionsReset" {
				typ = k
			}
		}
		triggers = append(triggers, map[string]string{"name": str(tpl, "name"), "type": typ, "conditions": str(tpl, "conditions")})
	}
	f["dependencies"] = deps
	f["triggers"] = triggers
	f["eventBusName"] = nonEmpty(str(o, "spec", "eventBusName"), "default")
	conditionsHealth(s, o)
}

func eventBus(s *Obj, o map[string]any) {
	f := s.Fields
	typ := ""
	for _, k := range []string{"nats", "jetstream", "kafka"} {
		if obj(o, "spec", k) != nil {
			typ = k
		}
	}
	f["type"] = typ
	conditionsHealth(s, o)
}

// conditionsHealth: Argo Events objects are healthy when all conditions are True.
func conditionsHealth(s *Obj, o map[string]any) {
	cs := conditions(o)
	s.Phase = "Running"
	if len(cs) == 0 {
		s.Phase = "Pending"
	}
	for _, c := range cs {
		if str(c, "status") != "True" {
			s.Phase = "Degraded"
			s.Problems = append(s.Problems, Problem{Severity: "error", Source: "condition", Resource: str(c, "type"),
				Message: nonEmpty(str(c, "message"), str(c, "reason"))})
		}
	}
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
