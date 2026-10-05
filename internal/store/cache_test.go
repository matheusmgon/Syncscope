package store

import (
	"testing"

	"argodeck/internal/argocd"
	"argodeck/internal/config"
)

func TestCacheRoundTrip(t *testing.T) {
	t.Setenv("ARGODECK_CONFIG_DIR", t.TempDir())
	t.Setenv("ARGODECK_NO_KEYRING", "1")
	cfg, _ := config.Open()
	ctx, _ := cfg.Upsert(config.Context{Name: "c", Server: "https://argocd.example.com"})
	var snaps []int
	m := NewManager(cfg, func(ev string, d any) {
		if ev == "apps:snapshot" {
			snaps = append(snaps, len(d.(map[string]any)["apps"].([]AppSummary)))
		}
	})

	c := newConn(m, ctx)
	for _, n := range []string{"a", "b", "c"} {
		a := &argocd.Application{Metadata: argocd.ObjectMeta{Name: n, Namespace: "argocd",
			OwnerReferences: []argocd.OwnerReference{{Kind: "ApplicationSet", Name: "set"}}}}
		a.Status.Health.Status = "Healthy"
		c.apps[appKey(ctx.ID, "argocd", n)] = a
	}
	c.appsets["k"] = AppSetSummary{Key: "k", Name: "set"}
	c.apps[appKey(ctx.ID, "argocd", "a")].Status.Health.Status = "Degraded"
	c.enrich[appKey(ctx.ID, "argocd", "a")] = enrichEntry{stamp: "x", problems: []Problem{{Severity: "error", Source: "health", Resource: "Pod p", Message: "CrashLoopBackOff"}}}
	if err := c.saveCache(); err != nil {
		t.Fatal(err)
	}

	// "restart": a fresh conn shows the cached apps before any network call
	c2 := newConn(m, ctx)
	if !c2.loadCache() {
		t.Fatal("cache not loaded")
	}
	if len(c2.sums) != 3 || c2.sums[appKey(ctx.ID, "argocd", "a")].AppSet != "set" || c2.getStatus().CachedAt == "" {
		t.Fatalf("unexpected: %d sums, status %+v", len(c2.sums), c2.getStatus())
	}
	if p := c2.sums[appKey(ctx.ID, "argocd", "a")].Problems; len(p) == 0 || p[0].Message != "CrashLoopBackOff" {
		t.Fatalf("enrichment not restored: %+v", p)
	}
	if len(snaps) != 1 || snaps[0] != 3 {
		t.Fatalf("snapshot not emitted: %v", snaps)
	}

	// a cache for another server URL is ignored
	ctx.Server = "https://other.example.com"
	if newConn(m, ctx).loadCache() {
		t.Fatal("cache of a different server must not be used")
	}
}
