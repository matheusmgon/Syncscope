package store

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"argodeck/internal/argocd"
	"argodeck/internal/config"
)

// Emitter pushes events to the UI.
type Emitter func(event string, data any)

type ContextStatus struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Server   string `json:"server"`
	AuthType string `json:"authType"`
	Color    string `json:"color"`
	Disabled bool   `json:"disabled"`
	State    string `json:"state"` // idle | connecting | ok | error | auth
	Message  string `json:"message,omitempty"`
	Version  string `json:"version,omitempty"`
	User     string `json:"user,omitempty"`
	Synced   string `json:"synced,omitempty"`   // last successful list
	CachedAt string `json:"cachedAt,omitempty"` // set while showing data loaded from disk
	// Errors listing secondary objects (usually RBAC); shown in the UI.
	AppSetsError  string `json:"appSetsError,omitempty"`
	ClustersError string `json:"clustersError,omitempty"`
}

type AppSetSummary struct {
	Key       string    `json:"key"`
	Ctx       string    `json:"ctx"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Problems  []Problem `json:"problems,omitempty"`
}

type ClusterSummary struct {
	Ctx     string `json:"ctx"`
	Name    string `json:"name"`
	Server  string `json:"server"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`
}

type Manager struct {
	cfg  *config.Store
	emit Emitter
	// OpenBrowser is used by the SSO flow.
	OpenBrowser func(string)

	mu    sync.RWMutex
	conns map[string]*conn

	logs  logStreams
	terms termSessions

	pendMu  sync.Mutex
	pendUp  map[string]AppSummary
	pendDel map[string]struct{}
}

func NewManager(cfg *config.Store, emit Emitter) *Manager {
	return &Manager{cfg: cfg, emit: emit, conns: map[string]*conn{},
		pendUp: map[string]AppSummary{}, pendDel: map[string]struct{}{}}
}

// Start connects every enabled context and begins flushing UI deltas.
func (m *Manager) Start(ctx context.Context) {
	for _, c := range m.cfg.Contexts() {
		m.startConn(c)
	}
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.flush()
			}
		}
	}()
}

func (m *Manager) flush() {
	m.pendMu.Lock()
	if len(m.pendUp) == 0 && len(m.pendDel) == 0 {
		m.pendMu.Unlock()
		return
	}
	up := make([]AppSummary, 0, len(m.pendUp))
	for _, s := range m.pendUp {
		up = append(up, s)
	}
	del := make([]string, 0, len(m.pendDel))
	for k := range m.pendDel {
		del = append(del, k)
	}
	m.pendUp = map[string]AppSummary{}
	m.pendDel = map[string]struct{}{}
	m.pendMu.Unlock()
	m.emit("apps:delta", map[string]any{"upserts": up, "deletes": del})
}

func (m *Manager) queueUp(s AppSummary) {
	m.pendMu.Lock()
	delete(m.pendDel, s.Key)
	m.pendUp[s.Key] = s
	m.pendMu.Unlock()
}

func (m *Manager) queueDel(key string) {
	m.pendMu.Lock()
	delete(m.pendUp, key)
	m.pendDel[key] = struct{}{}
	m.pendMu.Unlock()
}

func (m *Manager) emitSnapshot(ctxID string, apps []AppSummary) {
	m.pendMu.Lock()
	prefix := ctxID + "|"
	for k := range m.pendUp {
		if strings.HasPrefix(k, prefix) {
			delete(m.pendUp, k)
		}
	}
	for k := range m.pendDel {
		if strings.HasPrefix(k, prefix) {
			delete(m.pendDel, k)
		}
	}
	m.pendMu.Unlock()
	m.emit("apps:snapshot", map[string]any{"ctx": ctxID, "apps": apps})
}

func (m *Manager) emitStatus() { m.emit("ctx:status", m.Statuses()) }

func (m *Manager) startConn(c config.Context) {
	m.mu.Lock()
	if old := m.conns[c.ID]; old != nil {
		old.stop()
	}
	cn := newConn(m, c)
	m.conns[c.ID] = cn
	m.mu.Unlock()
	if !c.Disabled {
		cn.start()
	}
	m.emitStatus()
}

func (m *Manager) conn(id string) (*conn, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c := m.conns[id]
	if c == nil {
		return nil, errors.New("context not found")
	}
	return c, nil
}

// ---- contexts --------------------------------------------------------------

func (m *Manager) Statuses() []ContextStatus {
	m.mu.RLock()
	out := make([]ContextStatus, 0, len(m.conns))
	for _, c := range m.conns {
		out = append(out, c.getStatus())
	}
	m.mu.RUnlock()
	order := map[string]int{}
	for i, c := range m.cfg.Contexts() {
		order[c.ID] = i
	}
	sort.Slice(out, func(i, j int) bool { return order[out[i].ID] < order[out[j].ID] })
	return out
}

func (m *Manager) SaveContext(c config.Context) (config.Context, error) {
	saved, err := m.cfg.Upsert(c)
	if err != nil {
		return saved, err
	}
	m.startConn(saved)
	return saved, nil
}

func (m *Manager) DeleteContext(id string) error {
	m.mu.Lock()
	if c := m.conns[id]; c != nil {
		c.stop()
		delete(m.conns, id)
	}
	m.mu.Unlock()
	err := m.cfg.Delete(id)
	_ = os.Remove(m.cachePath(id))
	m.emitSnapshot(id, nil)
	m.emit("appsets", m.AppSets())
	m.emitStatus()
	return err
}

func (m *Manager) ImportCLI() (int, error) {
	ctxs, err := m.cfg.ImportCLI()
	for _, c := range ctxs {
		m.startConn(c)
	}
	return len(ctxs), err
}

func (m *Manager) Reconnect(id string) error {
	c, ok := m.cfg.Get(id)
	if !ok {
		return errors.New("context not found")
	}
	m.startConn(c)
	return nil
}

func (m *Manager) Test(c config.Context) (string, error) {
	cl, err := argocd.NewClient(c.ClientOptions(), argocd.Credentials{}, nil)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	v, err := cl.Version(ctx)
	if err != nil {
		return "", err
	}
	info := "Argo CD " + v
	if p, err := cl.SSOProvider(ctx); err == nil {
		info += " · SSO: " + p
	} else {
		info += " · no SSO"
	}
	return info, nil
}

func (m *Manager) LoginSSO(id string) error {
	c, err := m.conn(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	open := m.OpenBrowser
	if open == nil {
		open = func(string) {}
	}
	if err := c.client.LoginSSO(ctx, c.cfg.SSOPort, !c.cfg.SSONoOffline, open); err != nil {
		return err
	}
	return m.Reconnect(id)
}

func (m *Manager) LoginPassword(id, user, pass string, remember bool) error {
	c, err := m.conn(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.client.LoginPassword(ctx, user, pass, remember); err != nil {
		return err
	}
	cfg := c.cfg
	cfg.Username = user
	_, _ = m.cfg.Upsert(cfg)
	return m.Reconnect(id)
}

func (m *Manager) LoginToken(id, token string) error {
	c, err := m.conn(id)
	if err != nil {
		return err
	}
	c.client.LoginToken(token)
	return m.Reconnect(id)
}

func (m *Manager) Logout(id string) error {
	c, err := m.conn(id)
	if err != nil {
		return err
	}
	c.client.Logout()
	return m.Reconnect(id)
}

// ---- reads -----------------------------------------------------------------

func (m *Manager) Apps() []AppSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []AppSummary
	for _, c := range m.conns {
		c.mu.RLock()
		for _, s := range c.sums {
			out = append(out, s)
		}
		c.mu.RUnlock()
	}
	return out
}

func (m *Manager) AppSets() []AppSetSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []AppSetSummary{}
	for _, c := range m.conns {
		c.mu.RLock()
		for _, s := range c.appsets {
			out = append(out, s)
		}
		c.mu.RUnlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (m *Manager) Clusters() []ClusterSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []ClusterSummary{}
	for _, c := range m.conns {
		c.mu.RLock()
		for _, cl := range c.clusters.byServer {
			st, msg := c.clusters.state(cl)
			out = append(out, ClusterSummary{Ctx: c.cfg.ID, Name: cl.Name, Server: cl.Server, State: st, Message: msg, Version: cl.Info.ServerVersion})
		}
		c.mu.RUnlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ctx+out[i].Name < out[j].Ctx+out[j].Name })
	return out
}

func splitKey(key string) (ctxID, ns, name string, ok bool) {
	ctxID, rest, ok := strings.Cut(key, "|")
	if !ok {
		return
	}
	ns, name, ok = strings.Cut(rest, "/")
	return
}

func (m *Manager) resolve(key string) (*conn, *argocd.Application, error) {
	ctxID, _, _, ok := splitKey(key)
	if !ok {
		return nil, nil, errors.New("invalid key")
	}
	c, err := m.conn(ctxID)
	if err != nil {
		return nil, nil, err
	}
	c.mu.RLock()
	a := c.apps[key]
	c.mu.RUnlock()
	if a == nil {
		return nil, nil, errors.New("application not found (deleted?)")
	}
	return c, a, nil
}

func (m *Manager) WebURL(key string) (string, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(c.client.BaseURL(), "/") + "/applications/" + a.Metadata.Namespace + "/" + a.Metadata.Name, nil
}
