package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"syncscope/internal/argocd"
	"syncscope/internal/config"
)

// MOCKKUBE_KUBECONFIG=... go test ./internal/store -run IntegrationCore
func TestIntegrationCore(t *testing.T) {
	kc := os.Getenv("MOCKKUBE_KUBECONFIG")
	if kc == "" {
		t.Skip("MOCKKUBE_KUBECONFIG not set")
	}
	t.Setenv("SYNCSCOPE_KUBECONFIG", kc)
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, _ := config.Open()
	m := NewManager(cfg, func(string, any) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	info, err := m.Test(config.Context{AuthType: config.AuthCore, KubeContext: "mock-prod", Namespace: "argocd"})
	if err != nil || !strings.Contains(info, "40 Applications") {
		t.Fatalf("test: %q %v", info, err)
	}
	c, err := m.SaveContext(config.Context{Name: "core", AuthType: config.AuthCore, KubeContext: "mock-prod"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "core connected", 15*time.Second, func() bool { return status(m, c.ID).State == "ok" })
	apps := m.Apps()
	var target, degraded string
	sets := 0
	for _, a := range apps {
		if a.Sync == "OutOfSync" && target == "" {
			target = a.Key
		}
		if a.Health == "Degraded" && degraded == "" {
			degraded = a.Key
		}
		if a.AppSet == "guestbook-set" {
			sets++
		}
	}
	if len(apps) != 40 || sets != 20 || target == "" {
		t.Fatalf("apps=%d sets=%d target=%q", len(apps), sets, target)
	}
	// problems come from status.resources
	for _, a := range apps {
		if a.Key == degraded && (len(a.Problems) == 0 || !strings.Contains(a.Problems[0].Message, "progress deadline")) {
			t.Fatalf("degraded app not explained: %+v", a.Problems)
		}
	}
	if rep := m.Sync([]string{target}, argocd.SyncOptions{Prune: true}); rep.Failed != 0 {
		t.Fatalf("sync: %+v", rep)
	}
	waitFor(t, "core sync finished", 10*time.Second, func() bool {
		for _, a := range m.Apps() {
			if a.Key == target {
				return a.Sync == "Synced" && a.OpPhase == "Succeeded"
			}
		}
		return false
	})
	if rep := m.Refresh([]string{target}, true); rep.Failed != 0 {
		t.Fatalf("refresh: %+v", rep)
	}
	if d, err := m.Detail(target); err != nil || len(d.Tree) == 0 {
		t.Fatalf("detail: %v", err)
	}
	if _, err := m.Diff(target); err == nil || !strings.Contains(err.Error(), "core mode") {
		t.Fatalf("diff should be unsupported in core mode, got %v", err)
	}
	key, err := m.CreateApp(c.ID, strings.Replace(m.NewAppTemplate(c.ID), "name: my-app", "name: core-created", 1), false)
	if err != nil || !strings.HasSuffix(key, "/core-created") {
		t.Fatalf("create: %v %v", key, err)
	}
	if rep := m.Delete([]string{target}, DeleteOptions{Cascade: true}); rep.Failed != 0 {
		t.Fatalf("delete: %+v", rep)
	}
}
