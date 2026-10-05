package store

// Integration test against cmd/mockargo:
//
//	go run ./cmd/mockargo -port 8099 -apps 15000 &
//	MOCKARGO_URL=http://localhost:8099 go test ./internal/store -run Integration -v

import (
	"context"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"syncscope/internal/argocd"
	"syncscope/internal/config"
)

type recorder struct {
	mu     sync.Mutex
	events map[string]int
	snaps  map[string]int
}

func (r *recorder) emit(ev string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[ev]++
	if ev == "apps:snapshot" {
		m := data.(map[string]any)
		apps, _ := m["apps"].([]AppSummary)
		r.snaps[m["ctx"].(string)] = len(apps)
	}
}

func waitFor(t *testing.T, what string, d time.Duration, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestIntegration(t *testing.T) {
	url := os.Getenv("MOCKARGO_URL")
	if url == "" {
		t.Skip("MOCKARGO_URL not set")
	}
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, err := config.Open()
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{events: map[string]int{}, snaps: map[string]int{}}
	m := NewManager(cfg, rec.emit)
	// Simulated browser: open the auth page and follow the login link.
	m.OpenBrowser = func(u string) {
		go func() {
			resp, err := http.Get(u)
			if err != nil {
				t.Errorf("auth page: %v", err)
				return
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			link := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(string(b))
			if link == nil {
				t.Errorf("no login link in %s", b)
				return
			}
			r2, err := http.Get(strings.ReplaceAll(link[1], "&amp;", "&"))
			if err != nil {
				t.Errorf("callback: %v", err)
				return
			}
			r2.Body.Close()
		}()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	info, err := m.Test(config.Context{Server: url})
	if err != nil || !strings.Contains(info, "SSO") {
		t.Fatalf("Test(): %q %v", info, err)
	}
	t.Log("test connection:", info)

	// --- password login ---
	pw, err := m.SaveContext(config.Context{Name: "pw", Server: url, AuthType: config.AuthPassword})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "auth-required state", 5*time.Second, func() bool { return status(m, pw.ID).State == "auth" })
	if err := m.LoginPassword(pw.ID, "admin", "wrong", false); err == nil || !strings.Contains(err.Error(), "Invalid") {
		t.Fatalf("expected invalid password error, got %v", err)
	}
	start := time.Now()
	if err := m.LoginPassword(pw.ID, "admin", "admin", true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pw connected", 30*time.Second, func() bool { return status(m, pw.ID).State == "ok" })
	t.Logf("listed %d apps in %s", len(m.Apps()), time.Since(start).Round(time.Millisecond))
	if n := len(m.Apps()); n < 1000 {
		t.Fatalf("expected many apps, got %d", n)
	}

	// --- problems were computed ---
	var syncFail, comparison, retry, cluster, degradedTree int
	for _, a := range m.Apps() {
		for _, p := range a.Problems {
			switch {
			case p.Source == "sync" && p.Resource != "":
				syncFail++
			case p.Resource == "ComparisonError":
				comparison++
			case p.Source == "retry":
				retry++
			case p.Source == "cluster":
				cluster++
			case p.Source == "health" && strings.Contains(p.Message, "BackOff"):
				degradedTree++
			}
		}
	}
	t.Logf("problems: syncFailedResources=%d comparisonErrors=%d retries=%d clusterDown=%d", syncFail, comparison, retry, cluster)
	if syncFail == 0 || comparison == 0 || retry == 0 || cluster == 0 {
		t.Fatal("expected each kind of problem to be detected")
	}
	waitFor(t, "resource-tree enrichment of degraded apps", 20*time.Second, func() bool {
		n := 0
		for _, a := range m.Apps() {
			for _, p := range a.Problems {
				if strings.Contains(p.Message, "BackOff") {
					n++
				}
			}
		}
		degradedTree = n
		return n > 0
	})
	t.Logf("degraded apps explained via resource-tree: %d", degradedTree)

	sets := m.AppSets()
	var badSet bool
	for _, s := range sets {
		if s.Name == "reporting" && len(s.Problems) > 0 {
			badSet = true
		}
	}
	if !badSet {
		t.Fatal("appset error not detected")
	}

	// --- live stream ---
	rec.mu.Lock()
	before := rec.events["apps:delta"]
	rec.mu.Unlock()
	waitFor(t, "watch deltas", 10*time.Second, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return rec.events["apps:delta"] > before+1
	})

	// --- actions ---
	var okKey, legacyKey, badRepoKey string
	for _, a := range m.Apps() {
		switch {
		case a.Cluster == "legacy-dc" && legacyKey == "":
			legacyKey = a.Key
		case a.Cluster != "legacy-dc" && a.Severity == 0 && okKey == "":
			okKey = a.Key
		}
		for _, p := range a.Problems {
			if p.Resource == "ComparisonError" && badRepoKey == "" {
				badRepoKey = a.Key
			}
		}
	}
	rep := m.Restart([]string{okKey, legacyKey})
	if rep.Failed != 1 || !strings.Contains(rep.Results[1].Error, "credentials") {
		t.Fatalf("restart report: %+v", rep)
	}
	rep = m.Sync([]string{okKey, badRepoKey}, argocd.SyncOptions{})
	if rep.Failed != 1 || !strings.Contains(rep.Results[1].Error, "Repository not found") {
		t.Fatalf("sync report: %+v", rep)
	}
	waitFor(t, "sync to show Running then Succeeded", 10*time.Second, func() bool {
		c, _ := m.conn(pw.ID)
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.sums[okKey].OpPhase == "Succeeded" && c.sums[okKey].Sync == "Synced"
	})
	if rep := m.Refresh([]string{okKey}, true); rep.Failed != 0 {
		t.Fatalf("refresh: %+v", rep)
	}
	d, err := m.Detail(okKey)
	if err != nil || len(d.Resources) == 0 || len(d.Pods) == 0 || d.WebURL == "" {
		t.Fatalf("detail: %+v %v", d, err)
	}

	// --- SSO login with PKCE, then refresh-token renewal ---
	sso, err := m.SaveContext(config.Context{Name: "sso", Server: url, AuthType: config.AuthSSO, SSOPort: 18085})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoginSSO(sso.ID); err != nil {
		t.Fatalf("sso: %v", err)
	}
	waitFor(t, "sso connected", 30*time.Second, func() bool { return status(m, sso.ID).State == "ok" })
	if st := status(m, sso.ID); st.User != "dev@acme.io" {
		t.Fatalf("sso user = %q", st.User)
	}
	cr := cfg.Credentials(sso.ID)
	if cr.RefreshToken == "" {
		t.Fatal("refresh token not persisted")
	}
	// Invalidate the token: the client must renew it transparently.
	cfg.SetCredentials(sso.ID, argocd.Credentials{Token: "bogus", RefreshToken: cr.RefreshToken})
	if err := m.Reconnect(sso.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "sso reconnect via refresh token", 30*time.Second, func() bool { return status(m, sso.ID).State == "ok" })
	if got := cfg.Credentials(sso.ID).Token; got == "bogus" {
		t.Fatal("token was not renewed")
	}
	t.Logf("apps across both contexts: %d", len(m.Apps()))
}

func status(m *Manager, id string) ContextStatus {
	for _, s := range m.Statuses() {
		if s.ID == id {
			return s
		}
	}
	return ContextStatus{}
}
