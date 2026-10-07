package store

import (
	"context"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"syncscope/internal/config"
)

// Re-login via SSO: cancelling frees the callback port, a retry works, and the
// callback is accepted on the IPv6 loopback too (browsers may resolve
// "localhost" to ::1).
func TestIntegrationSSORetry(t *testing.T) {
	url := os.Getenv("MOCKARGO_URL")
	if url == "" {
		t.Skip("MOCKARGO_URL not set")
	}
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, _ := config.Open()
	urls := make(chan string, 4)
	m := NewManager(cfg, func(ev string, d any) {
		if ev == "sso:url" {
			urls <- d.(map[string]any)["url"].(string)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	c, err := m.SaveContext(config.Context{Name: "sso", Server: url, AuthType: config.AuthSSO, SSOPort: 18086})
	if err != nil {
		t.Fatal(err)
	}

	// 1) start a login, never complete it in the browser, cancel it
	errc := make(chan error, 1)
	go func() { errc <- m.LoginSSO(c.ID) }()
	select {
	case <-urls:
	case <-time.After(10 * time.Second):
		t.Fatal("no login URL published")
	}
	m.CancelSSO(c.ID)
	select {
	case err := <-errc:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not stop the login")
	}

	// 2) retry immediately (same port) and complete it through [::1]
	go func() { errc <- m.LoginSSO(c.ID) }()
	var authURL string
	select {
	case authURL = <-urls:
	case <-time.After(10 * time.Second):
		t.Fatal("retry did not start (port still busy?)")
	}
	resp, err := http.Get(authURL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	link := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(string(b))
	if link == nil {
		t.Fatalf("no login link: %s", b)
	}
	cb := strings.ReplaceAll(strings.Replace(link[1], "localhost", "[::1]", 1), "&amp;", "&")
	r2, err := http.Get(cb)
	if err != nil {
		t.Fatalf("IPv6 callback: %v", err)
	}
	r2.Body.Close()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("login after retry: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("login did not finish")
	}
	waitFor(t, "connected after SSO", 15*time.Second, func() bool { return status(m, c.ID).State == "ok" })
}
