package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"argodeck/internal/argocd"
)

type DeleteOptions struct {
	Cascade bool   `json:"cascade"`
	Policy  string `json:"policy"` // foreground | background
}

func (m *Manager) Delete(keys []string, o DeleteOptions) ActionReport {
	return m.bulk("Delete", keys, func(ctx context.Context, c *conn, a *argocd.Application) (string, error) {
		return "", c.client.DeleteApplication(ctx, a.Metadata.Name, a.Metadata.Namespace, o.Cascade, o.Policy)
	})
}

type HistoryEntry struct {
	ID         int64  `json:"id"`
	Revision   string `json:"revision"`
	DeployedAt string `json:"deployedAt"`
	StartedAt  string `json:"startedAt,omitempty"`
	Source     string `json:"source,omitempty"`
	Path       string `json:"path,omitempty"`
	Target     string `json:"target,omitempty"`
	Author     string `json:"author,omitempty"`
	Date       string `json:"date,omitempty"`
	Message    string `json:"message,omitempty"`
	MetaError  string `json:"metaError,omitempty"`
	Current    bool   `json:"current"`
}

// History returns the deploy history (newest first) enriched with the commit
// author / message for each revision.
func (m *Manager) History(key string) ([]HistoryEntry, error) {
	c, cached, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := c.client.GetApplication(ctx, cached.Metadata.Name, cached.Metadata.Namespace, "")
	if err != nil {
		return nil, err
	}
	hist := a.Status.History
	out := make([]HistoryEntry, len(hist))
	for i, h := range hist {
		e := HistoryEntry{ID: h.ID, Revision: h.Revision, DeployedAt: h.DeployedAt, StartedAt: h.DeployStartedAt}
		if e.Revision == "" && len(h.Revisions) > 0 {
			e.Revision = h.Revisions[0]
		}
		if h.Source != nil {
			e.Source, e.Path, e.Target = h.Source.RepoURL, h.Source.Path, h.Source.TargetRevision
			if h.Source.Chart != "" {
				e.Path = h.Source.Chart
			}
		}
		out[i] = e
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > 0 {
		out[0].Current = true
	}
	if len(out) > 25 {
		out = out[:25]
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	sourceIndex := -1
	if len(a.Spec.Sources) > 0 {
		sourceIndex = 0
	}
	for i := range out {
		if out[i].Revision == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(e *HistoryEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			md, err := c.client.RevisionMetadata(ctx, a.Metadata.Name, a.Metadata.Namespace, e.Revision, sourceIndex)
			if err != nil {
				e.MetaError = err.Error()
				return
			}
			e.Author, e.Date, e.Message = md.Author, md.Date, strings.TrimSpace(md.Message)
		}(&out[i])
	}
	wg.Wait()
	return out, nil
}

func (m *Manager) Rollback(key string, id int64, prune, dryRun bool) error {
	c, a, err := m.resolve(key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return c.client.Rollback(ctx, a.Metadata.Name, a.Metadata.Namespace, id, prune, dryRun)
}

// ---- ApplicationSets -------------------------------------------------------

type AppSetDetail struct {
	Summary    AppSetSummary      `json:"summary"`
	Spec       string             `json:"spec"`       // pretty JSON of .spec
	Generators []string           `json:"generators"` // generator kinds
	Conditions []argocd.Condition `json:"conditions"`
	Apps       []string           `json:"apps"`     // app keys
	Preserve   bool               `json:"preserve"` // preserveResourcesOnDeletion
}

func (m *Manager) resolveAppSet(key string) (*conn, AppSetSummary, error) {
	ctxID, _, _, ok := splitKey(key)
	if !ok {
		return nil, AppSetSummary{}, errors.New("invalid key")
	}
	c, err := m.conn(ctxID)
	if err != nil {
		return nil, AppSetSummary{}, err
	}
	c.mu.RLock()
	s, found := c.appsets[key]
	c.mu.RUnlock()
	if !found {
		_, ns, name, _ := splitKey(key)
		s = AppSetSummary{Key: key, Ctx: ctxID, Name: name, Namespace: ns}
	}
	return c, s, nil
}

func (m *Manager) AppSetDetail(key string) (*AppSetDetail, error) {
	c, s, err := m.resolveAppSet(key)
	if err != nil {
		return nil, err
	}
	d := &AppSetDetail{Summary: s}
	c.mu.RLock()
	for k, a := range c.sums {
		if a.AppSet == s.Name {
			d.Apps = append(d.Apps, k)
		}
	}
	c.mu.RUnlock()
	sort.Strings(d.Apps)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw, err := c.client.GetApplicationSet(ctx, s.Name, s.Namespace)
	if err != nil {
		d.Spec = "could not load ApplicationSet: " + err.Error()
		return d, nil
	}
	spec, _ := raw["spec"].(map[string]any)
	d.Spec = argocd.Pretty(spec)
	if gens, ok := spec["generators"].([]any); ok {
		for _, g := range gens {
			if gm, ok := g.(map[string]any); ok {
				for k := range gm {
					d.Generators = append(d.Generators, k)
				}
			}
		}
	}
	if sp, ok := spec["syncPolicy"].(map[string]any); ok {
		d.Preserve, _ = sp["preserveResourcesOnDeletion"].(bool)
	}
	if st, ok := raw["status"].(map[string]any); ok {
		if conds, ok := st["conditions"].([]any); ok {
			for _, x := range conds {
				cm, _ := x.(map[string]any)
				str := func(k string) string { v, _ := cm[k].(string); return v }
				d.Conditions = append(d.Conditions, argocd.Condition{Type: str("type"), Status: str("status"), Reason: str("reason"), Message: str("message"), LastTransitionTime: str("lastTransitionTime")})
			}
		}
	}
	return d, nil
}

func (m *Manager) DeleteAppSet(key string) error {
	c, s, err := m.resolveAppSet(key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := c.client.DeleteApplicationSet(ctx, s.Name, s.Namespace); err != nil {
		return err
	}
	go c.refreshAux(context.Background(), true)
	return nil
}

// ---- Argo CD configuration (read-only) --------------------------------------

type ArgoConfig struct {
	Version      string            `json:"version"`
	Repositories []map[string]any  `json:"repositories"`
	Projects     []map[string]any  `json:"projects"`
	Accounts     []map[string]any  `json:"accounts"`
	Clusters     []map[string]any  `json:"clusters"`
	Settings     string            `json:"settings"`
	Errors       map[string]string `json:"errors"`
}

func (m *Manager) ArgoConfig(ctxID string) (*ArgoConfig, error) {
	c, err := m.conn(ctxID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out := &ArgoConfig{Errors: map[string]string{}, Version: c.getStatus().Version}
	var mu sync.Mutex
	var wg sync.WaitGroup
	run := func(name string, f func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(); err != nil {
				mu.Lock()
				out.Errors[name] = err.Error()
				mu.Unlock()
			}
		}()
	}
	run("repositories", func() (err error) { out.Repositories, err = c.client.ListRepositories(ctx); return })
	run("projects", func() (err error) { out.Projects, err = c.client.ListProjects(ctx); return })
	run("accounts", func() (err error) { out.Accounts, err = c.client.ListAccounts(ctx); return })
	run("clusters", func() (err error) { out.Clusters, err = c.client.ListClustersRaw(ctx); return })
	run("settings", func() error {
		s, err := c.client.RawSettings(ctx)
		out.Settings = argocd.Pretty(s)
		return err
	})
	wg.Wait()
	if len(out.Errors) == 5 {
		return out, fmt.Errorf("could not read the configuration: %s", out.Errors["settings"])
	}
	return out, nil
}
