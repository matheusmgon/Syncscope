package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"syncscope/internal/argocd"
)

func opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 45*time.Second)
}

// ---- sync policy --------------------------------------------------------------

type SyncPolicy struct {
	Automated bool `json:"automated"`
	Prune     bool `json:"prune"`
	SelfHeal  bool `json:"selfHeal"`
}

// SetSyncPolicy enables/disables automated sync, prune and self-heal.
func (m *Manager) SetSyncPolicy(keys []string, p SyncPolicy) ActionReport {
	name := "Disable auto-sync"
	var patch map[string]any
	if p.Automated {
		name = fmt.Sprintf("Auto-sync (prune %s, self-heal %s)", onOff(p.Prune), onOff(p.SelfHeal))
		patch = map[string]any{"spec": map[string]any{"syncPolicy": map[string]any{
			"automated": map[string]any{"prune": p.Prune, "selfHeal": p.SelfHeal},
		}}}
	} else {
		patch = map[string]any{"spec": map[string]any{"syncPolicy": map[string]any{"automated": nil}}}
	}
	return m.bulk(name, keys, func(ctx context.Context, c *conn, a *argocd.Application) (string, error) {
		return "", c.client.PatchApplication(ctx, a.Metadata.Name, a.Metadata.Namespace, patch)
	})
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// AppSetGuard explains whether the owning ApplicationSet will overwrite manual
// changes to an app's spec.
type AppSetGuard struct {
	AppSet           string   `json:"appSet"`
	ApplicationsSync string   `json:"applicationsSync"` // create-only / create-update / create-delete / sync
	Ignored          []string `json:"ignored"`          // jsonPointers / jq paths ignored for this app
	LoadError        string   `json:"loadError,omitempty"`
}

func (m *Manager) AppSetGuard(key string) (*AppSetGuard, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	g := &AppSetGuard{}
	for _, o := range a.Metadata.OwnerReferences {
		if o.Kind == "ApplicationSet" {
			g.AppSet = o.Name
		}
	}
	if g.AppSet == "" {
		return g, nil
	}
	ctx, cancel := opCtx()
	defer cancel()
	raw, err := c.client.GetApplicationSet(ctx, g.AppSet, a.Metadata.Namespace)
	if err != nil {
		g.LoadError = err.Error()
		return g, nil
	}
	spec, _ := raw["spec"].(map[string]any)
	if sp, ok := spec["syncPolicy"].(map[string]any); ok {
		g.ApplicationsSync, _ = sp["applicationsSync"].(string)
	}
	if list, ok := spec["ignoreApplicationDifferences"].([]any); ok {
		for _, x := range list {
			e, _ := x.(map[string]any)
			if n, _ := e["name"].(string); n != "" && n != a.Metadata.Name {
				continue
			}
			for _, k := range []string{"jsonPointers", "jqPathExpressions"} {
				if ps, ok := e[k].([]any); ok {
					for _, p := range ps {
						if s, ok := p.(string); ok {
							g.Ignored = append(g.Ignored, s)
						}
					}
				}
			}
		}
	}
	return g, nil
}

// ---- diff ---------------------------------------------------------------------

type DiffItem struct {
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Modified  bool   `json:"modified"`
	Hook      bool   `json:"hook"`
	Live      string `json:"live"`   // YAML, empty when the resource does not exist
	Target    string `json:"target"` // YAML, empty when it should be pruned
}

func (m *Manager) Diff(key string) ([]DiffItem, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	items, err := c.client.ManagedResources(ctx, a.Metadata.Name, a.Metadata.Namespace)
	if err != nil {
		return nil, err
	}
	out := make([]DiffItem, 0, len(items))
	for _, r := range items {
		live := r.NormalizedLiveState
		if live == "" || live == "null" {
			live = r.LiveState
		}
		target := r.PredictedLiveState
		if target == "" || target == "null" {
			target = r.TargetState
		}
		out = append(out, DiffItem{Group: r.Group, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name, Modified: r.Modified, Hook: r.Hook,
			Live: toCleanYAML(live), Target: toCleanYAML(target)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Modified != out[j].Modified {
			return out[i].Modified
		}
		return out[i].Kind+out[i].Name < out[j].Kind+out[j].Name
	})
	return out, nil
}

// toCleanYAML converts a JSON manifest to YAML without server-side noise.
func toCleanYAML(js string) string {
	if js == "" || js == "null" {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal([]byte(js), &obj) != nil || obj == nil {
		return ""
	}
	clean(obj, true)
	return toYAML(obj)
}

func clean(obj map[string]any, dropStatus bool) {
	if dropStatus {
		delete(obj, "status")
	}
	if md, ok := obj["metadata"].(map[string]any); ok {
		for _, k := range []string{"managedFields", "resourceVersion", "uid", "generation", "creationTimestamp", "selfLink"} {
			delete(md, k)
		}
		if an, ok := md["annotations"].(map[string]any); ok {
			delete(an, "kubectl.kubernetes.io/last-applied-configuration")
			if len(an) == 0 {
				delete(md, "annotations")
			}
		}
	}
}

func toYAML(v any) string {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	_ = enc.Encode(v)
	return b.String()
}

// ---- events -------------------------------------------------------------------

type EventRow struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Count     int    `json:"count"`
	First     string `json:"first"`
	Last      string `json:"last"`
	Object    string `json:"object"`
	Component string `json:"component"`
}

func (m *Manager) Events(key string, r *argocd.ResourceAction, uid string) ([]EventRow, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	evs, err := c.client.Events(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r, uid)
	if err != nil {
		return nil, err
	}
	out := make([]EventRow, 0, len(evs))
	for _, e := range evs {
		last := e.LastTimestamp
		if last == "" {
			last = e.EventTime
		}
		if last == "" {
			last = e.Metadata.CreationTimestamp
		}
		first := e.FirstTimestamp
		if first == "" {
			first = last
		}
		cnt := e.Count
		if cnt == 0 {
			cnt = 1
		}
		out = append(out, EventRow{Type: e.Type, Reason: e.Reason, Message: e.Message, Count: cnt, First: first, Last: last,
			Object: e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name, Component: e.Source.Component})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last > out[j].Last })
	return out, nil
}

// ---- resource operations ----------------------------------------------------

func (m *Manager) ResourceActions(key string, r argocd.ResourceAction) ([]argocd.ActionDef, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.ListResourceActions(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r)
}

func (m *Manager) RunAction(key string, r argocd.ResourceAction, action string) error {
	c, a, err := m.resolve(key)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.RunResourceAction(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r, action)
}

// ResourceYAML returns the live manifest (without managedFields) as YAML.
func (m *Manager) ResourceYAML(key string, r argocd.ResourceAction) (string, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	man, err := c.client.ResourceManifest(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r)
	if err != nil {
		return "", err
	}
	if md, ok := man["metadata"].(map[string]any); ok {
		delete(md, "managedFields")
	}
	return toYAML(man), nil
}

// PatchResourceYAML applies the edited manifest as a merge patch.
func (m *Manager) PatchResourceYAML(key string, r argocd.ResourceAction, y string) error {
	var obj map[string]any
	if err := yaml.Unmarshal([]byte(y), &obj); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	if obj == nil {
		return errors.New("empty manifest")
	}
	delete(obj, "status")
	if md, ok := obj["metadata"].(map[string]any); ok {
		delete(md, "resourceVersion")
		delete(md, "managedFields")
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	c, a, err := m.resolve(key)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.PatchResource(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r, string(b))
}

func (m *Manager) DeleteResource(key string, r argocd.ResourceAction, force, orphan bool) error {
	c, a, err := m.resolve(key)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.DeleteResource(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r, force, orphan)
}

// ---- application spec / parameters ------------------------------------------

// AppYAML returns the editable part of the Application (metadata labels,
// annotations, finalizers and the whole spec) as YAML.
func (m *Manager) AppYAML(key string) (string, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	raw, err := c.client.GetApplicationRaw(ctx, a.Metadata.Name, a.Metadata.Namespace)
	if err != nil {
		return "", err
	}
	md, _ := raw["metadata"].(map[string]any)
	view := map[string]any{"apiVersion": "argoproj.io/v1alpha1", "kind": "Application"}
	vmd := map[string]any{"name": md["name"], "namespace": md["namespace"]}
	for _, k := range []string{"labels", "annotations", "finalizers"} {
		if v, ok := md[k]; ok {
			vmd[k] = v
		}
	}
	view["metadata"] = vmd
	view["spec"] = raw["spec"]
	return toYAML(view), nil
}

func (m *Manager) SaveAppYAML(key, y string) error {
	var edited map[string]any
	if err := yaml.Unmarshal([]byte(y), &edited); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	spec, ok := edited["spec"].(map[string]any)
	if !ok {
		return errors.New("the YAML must contain a spec")
	}
	return m.updateApp(key, func(raw map[string]any) error {
		raw["spec"] = spec
		md, _ := raw["metadata"].(map[string]any)
		if emd, ok := edited["metadata"].(map[string]any); ok && md != nil {
			for _, k := range []string{"labels", "annotations", "finalizers"} {
				if v, ok := emd[k]; ok {
					md[k] = v
				} else {
					delete(md, k)
				}
			}
		}
		return nil
	})
}

func (m *Manager) updateApp(key string, mutate func(raw map[string]any) error) error {
	c, a, err := m.resolve(key)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	raw, err := c.client.GetApplicationRaw(ctx, a.Metadata.Name, a.Metadata.Namespace)
	if err != nil {
		return err
	}
	if err := mutate(raw); err != nil {
		return err
	}
	delete(raw, "status")
	delete(raw, "operation")
	return c.client.UpdateApplicationRaw(ctx, raw, true)
}

// AppSources returns spec.source / spec.sources as generic objects, plus
// whether the app uses the multi-source form.
type AppSources struct {
	Multi   bool             `json:"multi"`
	Sources []map[string]any `json:"sources"`
}

func (m *Manager) AppSources(key string) (*AppSources, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	raw, err := c.client.GetApplicationRaw(ctx, a.Metadata.Name, a.Metadata.Namespace)
	if err != nil {
		return nil, err
	}
	spec, _ := raw["spec"].(map[string]any)
	out := &AppSources{}
	if list, ok := spec["sources"].([]any); ok && len(list) > 0 {
		out.Multi = true
		for _, x := range list {
			s, _ := x.(map[string]any)
			out.Sources = append(out.Sources, s)
		}
	} else if s, ok := spec["source"].(map[string]any); ok {
		out.Sources = []map[string]any{s}
	}
	return out, nil
}

// SaveSource replaces one source (index in spec.sources, or spec.source).
func (m *Manager) SaveSource(key string, index int, src map[string]any) error {
	return m.updateApp(key, func(raw map[string]any) error {
		spec, _ := raw["spec"].(map[string]any)
		if spec == nil {
			return errors.New("application has no spec")
		}
		if list, ok := spec["sources"].([]any); ok && len(list) > 0 {
			if index < 0 || index >= len(list) {
				return fmt.Errorf("source %d does not exist", index)
			}
			list[index] = src
			return nil
		}
		spec["source"] = src
		return nil
	})
}

// ---- terminal -------------------------------------------------------------------

type termSessions struct {
	mu sync.Mutex
	m  map[string]*argocd.Terminal
}

type TerminalRequest struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}

// StartTerminal opens a shell; output arrives as "term" events {id, data} and
// the end as {id, closed, error}.
func (m *Manager) StartTerminal(key string, r TerminalRequest) (string, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t, err := c.client.OpenTerminal(ctx, argocd.TerminalRequest{App: a.Metadata.Name, AppNamespace: a.Metadata.Namespace, Project: a.Spec.Project,
		Namespace: r.Namespace, Pod: r.Pod, Container: r.Container})
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("term-%d", time.Now().UnixNano())
	m.terms.mu.Lock()
	if m.terms.m == nil {
		m.terms.m = map[string]*argocd.Terminal{}
	}
	m.terms.m[id] = t
	m.terms.mu.Unlock()
	go func() {
		err := t.Read(func(s string) { m.emit("term", map[string]any{"id": id, "data": s}) })
		m.terms.mu.Lock()
		_, open := m.terms.m[id]
		delete(m.terms.m, id)
		m.terms.mu.Unlock()
		msg := ""
		if err != nil && open && !strings.Contains(err.Error(), "close") {
			msg = err.Error()
		}
		m.emit("term", map[string]any{"id": id, "closed": true, "error": msg})
	}()
	return id, nil
}

func (m *Manager) term(id string) *argocd.Terminal {
	m.terms.mu.Lock()
	defer m.terms.mu.Unlock()
	return m.terms.m[id]
}

func (m *Manager) TermInput(id, data string) error {
	if t := m.term(id); t != nil {
		return t.Input(data)
	}
	return errors.New("terminal closed")
}

func (m *Manager) TermResize(id string, rows, cols int) error {
	if t := m.term(id); t != nil {
		return t.Resize(uint16(rows), uint16(cols))
	}
	return nil
}

func (m *Manager) TermClose(id string) {
	m.terms.mu.Lock()
	t := m.terms.m[id]
	delete(m.terms.m, id)
	m.terms.mu.Unlock()
	if t != nil {
		_ = t.Close()
	}
}
