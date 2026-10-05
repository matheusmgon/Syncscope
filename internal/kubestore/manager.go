// Package kubestore mirrors Argo Workflows, Events and Rollouts objects from the
// enabled kubeconfig contexts and exposes them (plus actions, logs, events) to
// the UI — the Kubernetes counterpart of internal/store.
package kubestore

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"syncscope/internal/config"
	"syncscope/internal/kube"
)

type Emitter func(event string, data any)

type KindState struct {
	State   string `json:"state"` // ok | connecting | error | missing | forbidden
	Message string `json:"message,omitempty"`
	Count   int    `json:"count"`
}

type ContextStatus struct {
	Name    string               `json:"name"`
	Server  string               `json:"server"`
	Version string               `json:"version,omitempty"`
	State   string               `json:"state"` // connecting | ok | error
	Message string               `json:"message,omitempty"`
	Kinds   map[string]KindState `json:"kinds"`
}

type Manager struct {
	cfg  *config.Store
	emit Emitter

	mu    sync.RWMutex
	conns map[string]*kconn

	pendMu  sync.Mutex
	pendUp  map[string]Obj
	pendDel map[string]struct{}

	logs struct {
		mu     sync.Mutex
		cancel map[string]context.CancelFunc
	}
}

type kconn struct {
	m      *Manager
	name   string
	client *kube.Client
	cancel context.CancelFunc

	mu     sync.RWMutex
	raw    map[string]*unstructured.Unstructured
	sums   map[string]Obj
	status ContextStatus
}

func NewManager(cfg *config.Store, emit Emitter) *Manager {
	return &Manager{cfg: cfg, emit: emit, conns: map[string]*kconn{}, pendUp: map[string]Obj{}, pendDel: map[string]struct{}{}}
}

func (m *Manager) Start(ctx context.Context) {
	for _, n := range m.cfg.Prefs().KubeContexts {
		m.startConn(n)
	}
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
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
	up := make([]Obj, 0, len(m.pendUp))
	for _, o := range m.pendUp {
		up = append(up, o)
	}
	del := make([]string, 0, len(m.pendDel))
	for k := range m.pendDel {
		del = append(del, k)
	}
	m.pendUp, m.pendDel = map[string]Obj{}, map[string]struct{}{}
	m.pendMu.Unlock()
	m.emit("k:delta", map[string]any{"upserts": up, "deletes": del})
}

func (m *Manager) emitStatus() { m.emit("k:status", m.Statuses()) }

// ---- contexts -----------------------------------------------------------------------

type ContextView struct {
	kube.ContextInfo
	Enabled bool `json:"enabled"`
}

func (m *Manager) Contexts() ([]ContextView, error) {
	list, err := kube.ListContexts()
	if err != nil {
		return nil, err
	}
	en := map[string]bool{}
	for _, n := range m.cfg.Prefs().KubeContexts {
		en[n] = true
	}
	out := make([]ContextView, 0, len(list))
	for _, c := range list {
		out = append(out, ContextView{ContextInfo: c, Enabled: en[c.Name]})
	}
	return out, nil
}

func (m *Manager) SetEnabled(names []string) error {
	p := m.cfg.Prefs()
	old := map[string]bool{}
	for _, n := range p.KubeContexts {
		old[n] = true
	}
	sort.Strings(names)
	p.KubeContexts = names
	if err := m.cfg.SetPrefs(p); err != nil {
		return err
	}
	now := map[string]bool{}
	for _, n := range names {
		now[n] = true
		if !old[n] {
			m.startConn(n)
		}
	}
	for n := range old {
		if !now[n] {
			m.stopConn(n)
		}
	}
	m.emitStatus()
	return nil
}

func (m *Manager) Reconnect(name string) {
	m.stopConn(name)
	m.startConn(name)
}

func (m *Manager) Statuses() []ContextStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ContextStatus, 0, len(m.conns))
	for _, c := range m.conns {
		c.mu.RLock()
		st := c.status
		st.Kinds = map[string]KindState{}
		for k, v := range c.status.Kinds {
			st.Kinds[k] = v
		}
		c.mu.RUnlock()
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Manager) Objects() []Obj {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Obj
	for _, c := range m.conns {
		c.mu.RLock()
		for _, o := range c.sums {
			out = append(out, o)
		}
		c.mu.RUnlock()
	}
	return out
}

func (m *Manager) startConn(name string) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &kconn{m: m, name: name, cancel: cancel, raw: map[string]*unstructured.Unstructured{}, sums: map[string]Obj{},
		status: ContextStatus{Name: name, State: "connecting", Kinds: map[string]KindState{}}}
	m.mu.Lock()
	if old := m.conns[name]; old != nil {
		old.cancel()
	}
	m.conns[name] = c
	m.mu.Unlock()
	m.emitStatus()
	go c.run(ctx)
}

func (m *Manager) stopConn(name string) {
	m.mu.Lock()
	c := m.conns[name]
	delete(m.conns, name)
	m.mu.Unlock()
	if c != nil {
		c.cancel()
		m.emit("k:reset", map[string]any{"ctx": name})
	}
	m.emitStatus()
}

func (c *kconn) setStatus(f func(s *ContextStatus)) {
	c.mu.Lock()
	f(&c.status)
	c.mu.Unlock()
	c.m.emitStatus()
}

func (c *kconn) run(ctx context.Context) {
	cl, err := kube.NewClient(c.name)
	if err != nil {
		c.setStatus(func(s *ContextStatus) { s.State, s.Message = "error", err.Error() })
		return
	}
	c.client = cl
	c.setStatus(func(s *ContextStatus) { s.Server = cl.Server })
	vctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	v, err := cl.Version(vctx)
	cancel()
	if err != nil {
		c.setStatus(func(s *ContextStatus) { s.State, s.Message = "error", kube.ShortErr(err) })
		// keep trying in the background
		for ctx.Err() == nil {
			if !sleepCtx(ctx, 30*time.Second) {
				return
			}
			vctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			v, err = cl.Version(vctx)
			cancel()
			if err == nil {
				break
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
	c.setStatus(func(s *ContextStatus) { s.State, s.Message, s.Version = "ok", "", v })
	for kind, r := range kube.Kinds {
		kind, r := kind, r
		go cl.Mirror(ctx, r, kube.MirrorHandler{
			Reset: func(items []unstructured.Unstructured) { c.reset(kind, items) },
			Upsert: func(u *unstructured.Unstructured) {
				c.upsert(kind, u)
			},
			Delete: func(ns, name string) { c.delete(kind, ns, name) },
			State: func(state, msg string) {
				c.setStatus(func(s *ContextStatus) {
					ks := s.Kinds[kind]
					ks.State, ks.Message = state, msg
					s.Kinds[kind] = ks
				})
			},
		})
	}
	<-ctx.Done()
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func (c *kconn) reset(kind string, items []unstructured.Unstructured) {
	prefix := c.name + "|" + kind + "|"
	c.mu.Lock()
	for k := range c.raw {
		if strings.HasPrefix(k, prefix) {
			delete(c.raw, k)
			delete(c.sums, k)
		}
	}
	list := make([]Obj, 0, len(items))
	for i := range items {
		u := &items[i]
		s := Summarize(c.name, kind, u)
		c.raw[s.Key], c.sums[s.Key] = u, s
		list = append(list, s)
	}
	ks := c.status.Kinds[kind]
	ks.Count = len(items)
	c.status.Kinds[kind] = ks
	c.mu.Unlock()
	c.m.pendMu.Lock()
	for k := range c.m.pendUp {
		if strings.HasPrefix(k, prefix) {
			delete(c.m.pendUp, k)
		}
	}
	c.m.pendMu.Unlock()
	c.m.emit("k:snapshot", map[string]any{"ctx": c.name, "kind": kind, "items": list})
	c.m.emitStatus()
}

func (c *kconn) upsert(kind string, u *unstructured.Unstructured) {
	s := Summarize(c.name, kind, u)
	c.mu.Lock()
	_, existed := c.raw[s.Key]
	c.raw[s.Key], c.sums[s.Key] = u, s
	if !existed {
		ks := c.status.Kinds[kind]
		ks.Count++
		c.status.Kinds[kind] = ks
	}
	c.mu.Unlock()
	c.m.pendMu.Lock()
	delete(c.m.pendDel, s.Key)
	c.m.pendUp[s.Key] = s
	c.m.pendMu.Unlock()
}

func (c *kconn) delete(kind, ns, name string) {
	key := objKey(c.name, kind, ns, name)
	c.mu.Lock()
	if _, ok := c.raw[key]; ok {
		ks := c.status.Kinds[kind]
		ks.Count--
		c.status.Kinds[kind] = ks
	}
	delete(c.raw, key)
	delete(c.sums, key)
	c.mu.Unlock()
	c.m.pendMu.Lock()
	delete(c.m.pendUp, key)
	c.m.pendDel[key] = struct{}{}
	c.m.pendMu.Unlock()
}

// ---- lookups ------------------------------------------------------------------------

func splitKey(key string) (ctxName, kind, ns, name string, err error) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return "", "", "", "", errors.New("invalid key")
	}
	nn := strings.SplitN(parts[2], "/", 2)
	if len(nn) != 2 {
		return "", "", "", "", errors.New("invalid key")
	}
	return parts[0], parts[1], nn[0], nn[1], nil
}

func (m *Manager) resolve(key string) (*kconn, string, string, string, error) {
	ctxName, kind, ns, name, err := splitKey(key)
	if err != nil {
		return nil, "", "", "", err
	}
	m.mu.RLock()
	c := m.conns[ctxName]
	m.mu.RUnlock()
	if c == nil || c.client == nil {
		return nil, "", "", "", errors.New("cluster " + ctxName + " is not connected")
	}
	return c, kind, ns, name, nil
}

func opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 45*time.Second)
}
