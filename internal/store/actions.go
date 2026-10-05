package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"syncscope/internal/argocd"
)

type ActionResult struct {
	Key   string `json:"key"`
	Ctx   string `json:"ctx"`
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Info  string `json:"info,omitempty"`
}

type ActionReport struct {
	ID      string         `json:"id"`
	Action  string         `json:"action"`
	Total   int            `json:"total"`
	Failed  int            `json:"failed"`
	Results []ActionResult `json:"results"`
	Started string         `json:"started"`
	Elapsed string         `json:"elapsed"`
}

const bulkConcurrency = 10

func (m *Manager) bulk(action string, keys []string, fn func(ctx context.Context, c *conn, a *argocd.Application) (string, error)) ActionReport {
	start := time.Now()
	id := fmt.Sprintf("%d", start.UnixNano())
	rep := ActionReport{ID: id, Action: action, Total: len(keys), Started: start.Format(time.RFC3339), Results: make([]ActionResult, len(keys))}
	var done atomic.Int64
	sem := make(chan struct{}, bulkConcurrency)
	var wg sync.WaitGroup
	m.emit("action:progress", map[string]any{"id": id, "action": action, "done": 0, "total": len(keys)})
	for i, key := range keys {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, key string) {
			defer wg.Done()
			defer func() { <-sem }()
			res := ActionResult{Key: key}
			c, a, err := m.resolve(key)
			if err == nil {
				res.Ctx, res.Name = c.cfg.ID, a.Metadata.Name
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				res.Info, err = fn(ctx, c, a)
				cancel()
			} else if _, ns, name, ok := splitKey(key); ok {
				res.Name = ns + "/" + name
			}
			res.OK = err == nil
			if err != nil {
				res.Error = err.Error()
			}
			rep.Results[i] = res
			n := done.Add(1)
			if n%5 == 0 || int(n) == len(keys) {
				m.emit("action:progress", map[string]any{"id": id, "action": action, "done": n, "total": len(keys)})
			}
		}(i, key)
	}
	wg.Wait()
	for _, r := range rep.Results {
		if !r.OK {
			rep.Failed++
		}
	}
	rep.Elapsed = time.Since(start).Round(time.Millisecond).String()
	m.emit("action:done", rep)
	return rep
}

func (m *Manager) Sync(keys []string, o argocd.SyncOptions) ActionReport {
	name := "Sync"
	if o.DryRun {
		name = "Sync (dry-run)"
	}
	return m.bulk(name, keys, func(ctx context.Context, c *conn, a *argocd.Application) (string, error) {
		return "", c.client.Sync(ctx, a.Metadata.Name, a.Metadata.Namespace, o)
	})
}

func (m *Manager) Refresh(keys []string, hard bool) ActionReport {
	mode, name := "normal", "Refresh"
	if hard {
		mode, name = "hard", "Hard refresh"
	}
	return m.bulk(name, keys, func(ctx context.Context, c *conn, a *argocd.Application) (string, error) {
		_, err := c.client.GetApplication(ctx, a.Metadata.Name, a.Metadata.Namespace, mode)
		return "", err
	})
}

func (m *Manager) Terminate(keys []string) ActionReport {
	return m.bulk("Terminate", keys, func(ctx context.Context, c *conn, a *argocd.Application) (string, error) {
		return "", c.client.TerminateOperation(ctx, a.Metadata.Name, a.Metadata.Namespace)
	})
}

// Restart runs the "restart" resource action on every Deployment, StatefulSet,
// DaemonSet and Argo Rollout of each app.
func (m *Manager) Restart(keys []string) ActionReport {
	return m.bulk("Restart", keys, func(ctx context.Context, c *conn, a *argocd.Application) (string, error) {
		ws := restartable(a)
		if len(ws) == 0 {
			return "", errors.New("no Deployment/StatefulSet/DaemonSet/Rollout to restart")
		}
		var errs []string
		var names []string
		for _, w := range ws {
			if err := c.client.RunResourceAction(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, w, "restart"); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", w.Kind, w.Name, err))
			} else {
				names = append(names, w.Kind+"/"+w.Name)
			}
		}
		info := "restarted: " + strings.Join(names, ", ")
		if len(errs) > 0 {
			return info, errors.New(strings.Join(errs, "; "))
		}
		return info, nil
	})
}

// RestartResource restarts a single workload from the detail view.
func (m *Manager) RestartResource(key string, r argocd.ResourceAction) error {
	c, a, err := m.resolve(key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return c.client.RunResourceAction(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r, "restart")
}

// ---- detail ----------------------------------------------------------------

type ResourceRow struct {
	Group       string `json:"group"`
	Version     string `json:"version"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Sync        string `json:"sync"`
	Health      string `json:"health"`
	Message     string `json:"message,omitempty"`
	Restartable bool   `json:"restartable"`
	Hook        bool   `json:"hook"`
	Prune       bool   `json:"prune"`
	Parent      string `json:"parent,omitempty"`
}

type AppDetail struct {
	Summary    AppSummary               `json:"summary"`
	Sources    []argocd.AppSource       `json:"sources"`
	Conditions []argocd.Condition       `json:"conditions"`
	Operation  *argocd.OperationState   `json:"operation"`
	Resources  []ResourceRow            `json:"resources"`
	Pods       []ResourceRow            `json:"pods"`
	History    []argocd.RevisionHistory `json:"history"`
	WebURL     string                   `json:"webURL"`
	Prune      bool                     `json:"prune"`
	SelfHeal   bool                     `json:"selfHeal"`
	TreeError  string                   `json:"treeError,omitempty"`
	Tree       []TreeNode               `json:"tree"`
}

func (m *Manager) Detail(key string) (*AppDetail, error) {
	c, cached, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var (
		a       *argocd.Application
		tree    *argocd.ResourceTree
		aErr    error
		treeErr error
		wg      sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		a, aErr = c.client.GetApplication(ctx, cached.Metadata.Name, cached.Metadata.Namespace, "")
	}()
	go func() {
		defer wg.Done()
		tree, treeErr = c.client.ResourceTree(ctx, cached.Metadata.Name, cached.Metadata.Namespace)
	}()
	wg.Wait()
	if aErr != nil {
		return nil, aErr
	}
	var extra []Problem
	if tree != nil {
		extra = treeProblems(tree)
	}
	c.mu.RLock()
	d := &AppDetail{Summary: summarize(c.cfg.ID, a, c.clusters, extra)}
	c.mu.RUnlock()
	if a.Spec.Source != nil {
		d.Sources = []argocd.AppSource{*a.Spec.Source}
	} else {
		d.Sources = a.Spec.Sources
	}
	d.Conditions = a.Status.Conditions
	d.Operation = a.Status.OperationState
	d.History = a.Status.History
	for i, j := 0, len(d.History)-1; i < j; i, j = i+1, j-1 {
		d.History[i], d.History[j] = d.History[j], d.History[i]
	}
	if p := a.Spec.SyncPolicy; p != nil && p.Automated != nil {
		d.Prune, d.SelfHeal = p.Automated.Prune, p.Automated.SelfHeal
	}
	d.WebURL, _ = m.WebURL(key)

	treeHealth := map[string]*argocd.HealthStatus{}
	if tree != nil {
		for _, n := range tree.Nodes {
			if n.Health != nil {
				treeHealth[n.Group+"/"+n.Kind+"/"+n.Namespace+"/"+n.Name] = n.Health
			}
		}
	}
	for _, r := range a.Status.Resources {
		row := ResourceRow{Group: r.Group, Version: r.Version, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name,
			Sync: r.Status, Hook: r.Hook, Prune: r.RequiresPruning}
		h := r.Health
		if h == nil {
			h = treeHealth[r.Group+"/"+r.Kind+"/"+r.Namespace+"/"+r.Name]
		}
		if h != nil {
			row.Health, row.Message = h.Status, h.Message
		}
		row.Restartable = (r.Group == "apps" && (r.Kind == "Deployment" || r.Kind == "StatefulSet" || r.Kind == "DaemonSet")) ||
			(r.Group == "argoproj.io" && r.Kind == "Rollout")
		d.Resources = append(d.Resources, row)
	}
	if tree != nil {
		for _, n := range tree.Nodes {
			if n.Kind != "Pod" {
				continue
			}
			row := ResourceRow{Kind: n.Kind, Version: n.Version, Namespace: n.Namespace, Name: n.Name}
			if n.Health != nil {
				row.Health, row.Message = n.Health.Status, n.Health.Message
			}
			for _, in := range n.Info {
				if in.Name == "Status Reason" && in.Value != "" {
					if row.Message == "" {
						row.Message = in.Value
					} else if !strings.Contains(row.Message, in.Value) {
						row.Message = in.Value + ": " + row.Message
					}
				}
			}
			if len(n.ParentRefs) > 0 {
				row.Parent = n.ParentRefs[0].Kind + "/" + n.ParentRefs[0].Name
			}
			d.Pods = append(d.Pods, row)
		}
	}
	d.Tree = buildTree(a, tree)
	if treeErr != nil {
		d.TreeError = treeErr.Error()
	}
	return d, nil
}
