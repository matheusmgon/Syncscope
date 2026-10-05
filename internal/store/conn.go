package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"syncscope/internal/argocd"
	"syncscope/internal/config"
	"syncscope/internal/core"
)

type enrichEntry struct {
	stamp    string // reconciledAt + health; recomputed when it changes
	problems []Problem
}

// conn keeps a live, in-memory mirror of one Argo CD server.
type conn struct {
	m      *Manager
	cfg    config.Context
	client argocd.API
	cerr   error

	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.RWMutex
	apps     map[string]*argocd.Application
	sums     map[string]AppSummary
	appsets  map[string]AppSetSummary
	clusters clusterInfo
	enrich   map[string]enrichEntry
	status   ContextStatus

	enrichQ chan string
	queued  sync.Map
	dirty   atomic.Bool
}

func newConn(m *Manager, c config.Context) *conn {
	cn := &conn{
		m: m, cfg: c,
		apps: map[string]*argocd.Application{}, sums: map[string]AppSummary{},
		appsets: map[string]AppSetSummary{}, enrich: map[string]enrichEntry{},
		clusters: clusterInfo{byServer: map[string]argocd.Cluster{}, byName: map[string]argocd.Cluster{}},
		enrichQ:  make(chan string, 4096),
		status: ContextStatus{ID: c.ID, Name: c.Name, Server: c.Server, AuthType: c.AuthType,
			Color: c.Color, Disabled: c.Disabled, State: "idle"},
	}
	if c.AuthType == config.AuthCore {
		cn.client, cn.cerr = core.New(c.KubeContext, c.Namespace)
	} else {
		cn.client, cn.cerr = argocd.NewClient(c.ClientOptions(), m.cfg.Credentials(c.ID), func(cr argocd.Credentials) {
			m.cfg.SetCredentials(c.ID, cr)
		})
	}
	return cn
}

func (c *conn) getStatus() ContextStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *conn) setState(state, msg string) {
	c.mu.Lock()
	c.status.State, c.status.Message = state, msg
	c.mu.Unlock()
	c.m.emitStatus()
}

func (c *conn) start() {
	if c.cerr != nil {
		c.setState("error", c.cerr.Error())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	go c.run(ctx)
	for i := 0; i < 4; i++ {
		go c.enrichWorker(ctx)
	}
}

func (c *conn) stop() {
	if c.cancel != nil {
		c.cancel()
		<-c.done
	}
}

func (c *conn) run(ctx context.Context) {
	defer close(c.done)
	c.loadCache()
	backoff := time.Second
	for ctx.Err() == nil {
		c.setState("connecting", "")
		connected, err := c.syncOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, argocd.ErrUnauthorized) {
			c.setState("auth", "session expired or missing — please log in")
			return
		}
		if connected {
			backoff = time.Second
		}
		msg := "connection lost"
		if err != nil {
			msg = err.Error()
		}
		c.setState("error", msg+" — reconnecting…")
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

// syncOnce lists everything, then follows the watch stream until it breaks.
func (c *conn) syncOnce(ctx context.Context) (bool, error) {
	if c.client.Credentials().Token == "" {
		return false, argocd.ErrUnauthorized
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	ver, err := c.client.Version(rctx)
	cancel()
	if err != nil {
		return false, err
	}
	rctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	ui, err := c.client.UserInfo(rctx)
	cancel()
	if err != nil {
		return false, err
	}
	if !ui.LoggedIn && c.client.Renew(ctx) {
		rctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		ui, err = c.client.UserInfo(rctx)
		cancel()
		if err != nil {
			return false, err
		}
	}
	if !ui.LoggedIn {
		return false, argocd.ErrUnauthorized
	}
	c.mu.Lock()
	c.status.Version, c.status.User = ver, ui.Username
	c.mu.Unlock()

	c.refreshAux(ctx, false)

	lctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	list, err := c.client.ListApplications(lctx)
	cancel()
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	c.apps = make(map[string]*argocd.Application, len(list.Items))
	for i := range list.Items {
		a := &list.Items[i]
		c.apps[appKey(c.cfg.ID, a.Metadata.Namespace, a.Metadata.Name)] = a
	}
	c.status.Synced = time.Now().Format(time.RFC3339)
	c.status.CachedAt = ""
	c.mu.Unlock()
	c.rebuildAll()
	c.setState("ok", "")
	go func() { _ = c.saveCache() }()

	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-wctx.Done():
				return
			case <-t.C:
				c.refreshAux(wctx, true)
				if c.dirty.Load() {
					_ = c.saveCache()
				}
			}
		}
	}()
	err = c.client.WatchApplications(wctx, list.Metadata.ResourceVersion, c.onEvent)
	return true, err
}

// refreshAux reloads clusters and ApplicationSets. Errors are tolerated (RBAC
// may forbid them). When cluster health changed, summaries are rebuilt.
func (c *conn) refreshAux(ctx context.Context, rebuild bool) {
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	changed := false
	cls, cerr := c.client.ListClusters(rctx)
	c.mu.Lock()
	c.status.ClustersError = errString(cerr)
	c.mu.Unlock()
	if cerr == nil {
		ci := clusterInfo{byServer: map[string]argocd.Cluster{}, byName: map[string]argocd.Cluster{}}
		for _, cl := range cls {
			ci.byServer[cl.Server] = cl
			if cl.Name != "" {
				ci.byName[cl.Name] = cl
			}
		}
		c.mu.Lock()
		for k, cl := range ci.byServer {
			old, ok := c.clusters.byServer[k]
			s1, _ := ci.state(cl)
			s0, _ := ci.state(old)
			if !ok || s0 != s1 || old.Name != cl.Name {
				changed = true
			}
		}
		if len(ci.byServer) != len(c.clusters.byServer) {
			changed = true
		}
		c.clusters = ci
		c.mu.Unlock()
	}
	sets, serr := c.client.ListApplicationSets(rctx)
	c.mu.Lock()
	c.status.AppSetsError = errString(serr)
	c.mu.Unlock()
	c.m.emitStatus()
	if serr == nil {
		out := make(map[string]AppSetSummary, len(sets))
		for _, s := range sets {
			as := AppSetSummary{Key: c.cfg.ID + "|" + s.Metadata.Namespace + "/" + s.Metadata.Name, Ctx: c.cfg.ID, Name: s.Metadata.Name, Namespace: s.Metadata.Namespace}
			for _, cd := range s.Status.Conditions {
				if cd.Type == "ErrorOccurred" && cd.Status == "True" {
					as.Problems = append(as.Problems, Problem{Severity: "error", Source: "appset", Resource: cd.Reason, Message: cd.Message})
				}
			}
			out[as.Key] = as
		}
		c.mu.Lock()
		c.appsets = out
		c.mu.Unlock()
		c.m.emit("appsets", c.m.AppSets())
	}
	if changed {
		c.m.emit("clusters", c.m.Clusters())
		if rebuild {
			c.rebuildAll()
		}
	}
}

func (c *conn) rebuildAll() {
	c.mu.Lock()
	sums := make(map[string]AppSummary, len(c.apps))
	list := make([]AppSummary, 0, len(c.apps))
	for k, a := range c.apps {
		s := summarize(c.cfg.ID, a, c.clusters, c.enrichFor(k, a))
		sums[k] = s
		list = append(list, s)
	}
	c.sums = sums
	apps := make(map[string]*argocd.Application, len(c.apps))
	for k, a := range c.apps {
		apps[k] = a
	}
	c.mu.Unlock()
	c.m.emitSnapshot(c.cfg.ID, list)
	for k, a := range apps {
		c.maybeEnrich(k, a)
	}
}

func enrichStamp(a *argocd.Application) string {
	return a.Status.ReconciledAt + "|" + a.Status.Health.Status
}

// enrichFor returns cached resource-tree problems if still valid. Caller holds c.mu.
func (c *conn) enrichFor(key string, a *argocd.Application) []Problem {
	e, ok := c.enrich[key]
	if !ok {
		return nil
	}
	if e.stamp != enrichStamp(a) && a.Status.Health.Status != "Degraded" {
		return nil
	}
	return e.problems
}

func needsTree(a *argocd.Application) bool {
	if a.Status.Health.Status != "Degraded" {
		return false
	}
	for _, r := range a.Status.Resources {
		if r.Health != nil && r.Health.Status == "Degraded" {
			return false // controller still persists resource health (Argo CD < 3)
		}
	}
	return true
}

func (c *conn) maybeEnrich(key string, a *argocd.Application) {
	if !needsTree(a) {
		return
	}
	c.mu.RLock()
	e, ok := c.enrich[key]
	c.mu.RUnlock()
	if ok && e.stamp == enrichStamp(a) {
		return
	}
	if _, dup := c.queued.LoadOrStore(key, true); dup {
		return
	}
	select {
	case c.enrichQ <- key:
	default:
		c.queued.Delete(key)
	}
}

func (c *conn) enrichWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-c.enrichQ:
			c.queued.Delete(key)
			c.mu.RLock()
			a := c.apps[key]
			c.mu.RUnlock()
			if a == nil || !needsTree(a) {
				continue
			}
			rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			tree, err := c.client.ResourceTree(rctx, a.Metadata.Name, a.Metadata.Namespace)
			cancel()
			if err != nil {
				continue
			}
			c.mu.Lock()
			c.enrich[key] = enrichEntry{stamp: enrichStamp(a), problems: treeProblems(tree)}
			c.dirty.Store(true)
			cur := c.apps[key]
			var s AppSummary
			if cur != nil {
				s = summarize(c.cfg.ID, cur, c.clusters, c.enrichFor(key, cur))
				c.sums[key] = s
			}
			c.mu.Unlock()
			if cur != nil {
				c.m.queueUp(s)
			}
		}
	}
}

func (c *conn) onEvent(ev argocd.ApplicationWatchEvent) {
	a := ev.Application
	key := appKey(c.cfg.ID, a.Metadata.Namespace, a.Metadata.Name)
	if ev.Type == "DELETED" {
		c.mu.Lock()
		delete(c.apps, key)
		delete(c.sums, key)
		delete(c.enrich, key)
		c.mu.Unlock()
		c.m.queueDel(key)
		return
	}
	c.dirty.Store(true)
	c.mu.Lock()
	prev, had := c.sums[key]
	c.apps[key] = &a
	s := summarize(c.cfg.ID, &a, c.clusters, c.enrichFor(key, &a))
	c.sums[key] = s
	c.mu.Unlock()
	c.m.queueUp(s)
	if had && c.m.OnTransition != nil {
		c.m.OnTransition(prev, s)
	}
	c.maybeEnrich(key, &a)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
