package store

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"argodeck/internal/argocd"
	"argodeck/internal/config"
)

func TestIntegrationOps(t *testing.T) {
	url := os.Getenv("MOCKARGO_URL")
	if url == "" {
		t.Skip("MOCKARGO_URL not set")
	}
	t.Setenv("ARGODECK_CONFIG_DIR", t.TempDir())
	t.Setenv("ARGODECK_NO_KEYRING", "1")
	cfg, _ := config.Open()
	var mu sync.Mutex
	var termOut strings.Builder
	termClosed := false
	m := NewManager(cfg, func(ev string, data any) {
		if ev != "term" {
			return
		}
		d := data.(map[string]any)
		mu.Lock()
		defer mu.Unlock()
		if s, ok := d["data"].(string); ok {
			termOut.WriteString(s)
		}
		if d["closed"] == true {
			termClosed = true
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	c, _ := m.SaveContext(config.Context{Name: "ops", Server: url, AuthType: config.AuthPassword})
	if err := m.LoginPassword(c.ID, "admin", "admin", false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "connected", 30*time.Second, func() bool { return status(m, c.ID).State == "ok" })

	var key, helmKey string
	for _, a := range m.Apps() {
		if a.Sync == "OutOfSync" && a.Severity == 0 && key == "" && !strings.Contains(a.Name, "platform-addons") {
			key = a.Key
		}
		if strings.HasPrefix(a.Name, "platform-addons") && helmKey == "" {
			helmKey = a.Key
		}
	}

	// sync policy
	rep := m.SetSyncPolicy([]string{key}, SyncPolicy{Automated: true, Prune: true, SelfHeal: false})
	if rep.Failed != 0 {
		t.Fatalf("sync policy: %+v", rep)
	}
	waitFor(t, "self-heal off", 5*time.Second, func() bool {
		cn, _ := m.conn(c.ID)
		cn.mu.RLock()
		defer cn.mu.RUnlock()
		sp := cn.apps[key].Spec.SyncPolicy
		return sp != nil && sp.Automated != nil && !sp.Automated.SelfHeal && sp.Automated.Prune
	})
	if g, err := m.AppSetGuard(key); err != nil || g.AppSet == "" {
		t.Fatalf("guard: %+v %v", g, err)
	}

	// diff
	diff, err := m.Diff(key)
	var depDiff *DiffItem
	for i := range diff {
		if diff[i].Kind == "Deployment" {
			depDiff = &diff[i]
		}
	}
	if err != nil || depDiff == nil || !depDiff.Modified || !strings.Contains(depDiff.Live, "v2.2.0") ||
		strings.Contains(depDiff.Live, "managedFields") || strings.Contains(depDiff.Live, "status:") || !diff[0].Modified {
		t.Fatalf("diff: %+v %v", diff, err)
	}

	// events
	if evs, err := m.Events(key, nil, ""); err != nil || len(evs) == 0 {
		t.Fatalf("events: %v %v", evs, err)
	}
	dep := argocd.ResourceAction{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: diff[0].Namespace, Name: "app"}
	acts, err := m.ResourceActions(key, dep)
	if err != nil || len(acts) == 0 || acts[0].Name != "restart" {
		t.Fatalf("actions: %v %v", acts, err)
	}
	if err := m.RunAction(key, dep, "pause"); err != nil {
		t.Fatal(err)
	}

	// app YAML round trip + helm source edit
	y, err := m.AppYAML(key)
	if err != nil || !strings.Contains(y, "kind: Application") {
		t.Fatalf("yaml: %v %v", y, err)
	}
	if err := m.SaveAppYAML(key, strings.Replace(y, "targetRevision: main", "targetRevision: release-1.2", 1)); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveAppYAML(key, strings.Replace(y, "targetRevision: main", "targetRevision: does-not-exist", 1)); err == nil || !strings.Contains(err.Error(), "InvalidSpecError") {
		t.Fatalf("expected validation error, got %v", err)
	}
	srcs, err := m.AppSources(helmKey)
	if err != nil || len(srcs.Sources) != 1 || srcs.Sources[0]["helm"] == nil {
		t.Fatalf("sources: %+v %v", srcs, err)
	}
	s := srcs.Sources[0]
	s["helm"].(map[string]any)["values"] = "replicaCount: 3\n"
	if err := m.SaveSource(helmKey, 0, s); err != nil {
		t.Fatal(err)
	}
	srcs, _ = m.AppSources(helmKey)
	if srcs.Sources[0]["helm"].(map[string]any)["values"] != "replicaCount: 3\n" {
		t.Fatalf("helm values not saved: %+v", srcs.Sources[0])
	}

	// terminal
	if _, err := m.StartTerminal(key, TerminalRequest{Namespace: diff[0].Namespace, Pod: "app-x", Container: "istio-proxy"}); err == nil {
		t.Fatal("expected error for container without shell")
	}
	id, err := m.StartTerminal(key, TerminalRequest{Namespace: diff[0].Namespace, Pod: "app-x", Container: "app"})
	if err != nil {
		t.Fatal(err)
	}
	_ = m.TermResize(id, 40, 120)
	_ = m.TermInput(id, "whoami\r")
	waitFor(t, "terminal output", 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(termOut.String(), "\r\napp\r\n")
	})
	_ = m.TermInput(id, "exit\r")
	waitFor(t, "terminal closed", 5*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return termClosed })
}
