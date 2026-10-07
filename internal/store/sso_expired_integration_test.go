package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"syncscope/internal/argocd"
	"syncscope/internal/config"
)

// An SSO session whose ID token has expired (laptop closed overnight) must be
// renewed with the refresh token when the app starts — without deadlocking the
// connection (stuck in "connecting…") or a later SSO login.
func TestIntegrationExpiredSSOTokenOnStartup(t *testing.T) {
	url := os.Getenv("MOCKARGO_URL")
	if url == "" {
		t.Skip("MOCKARGO_URL not set")
	}
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, _ := config.Open()
	c, err := cfg.Upsert(config.Context{Name: "sso", Server: url, AuthType: config.AuthSSO, SSOPort: 18087})
	if err != nil {
		t.Fatal(err)
	}
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"sub":"dev","exp":%d}`, time.Now().Add(-20*time.Hour).Unix())))
	cfg.SetCredentials(c.ID, argocd.Credentials{Token: hdr + "." + body + ".sig", RefreshToken: "mock-refresh"})

	m := NewManager(cfg, func(string, any) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx) // fresh start: OIDC settings not cached yet
	waitFor(t, "renewed with the refresh token", 15*time.Second, func() bool { return status(m, c.ID).State == "ok" })
}

// When the refresh token is dead too, the instance must quickly show "login
// required" and a fresh SSO login must work.
func TestIntegrationExpiredRefreshTokenThenSSOLogin(t *testing.T) {
	url := os.Getenv("MOCKARGO_URL")
	if url == "" {
		t.Skip("MOCKARGO_URL not set")
	}
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, _ := config.Open()
	c, _ := cfg.Upsert(config.Context{Name: "sso", Server: url, AuthType: config.AuthSSO, SSOPort: 18088})
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"sub":"dev","exp":%d}`, time.Now().Add(-20*time.Hour).Unix())))
	cfg.SetCredentials(c.ID, argocd.Credentials{Token: hdr + "." + body + ".sig", RefreshToken: "revoked"})

	urls := make(chan string, 2)
	m := NewManager(cfg, func(ev string, d any) {
		if ev == "sso:url" {
			urls <- d.(map[string]any)["url"].(string)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	waitFor(t, "login required", 15*time.Second, func() bool { return status(m, c.ID).State == "auth" })

	errc := make(chan error, 1)
	go func() { errc <- m.LoginSSO(c.ID) }()
	var authURL string
	select {
	case authURL = <-urls:
	case <-time.After(10 * time.Second):
		t.Fatal("SSO login never opened the browser")
	}
	resp, err := http.Get(authURL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	link := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(string(b))
	r2, err := http.Get(strings.ReplaceAll(link[1], "&amp;", "&"))
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if err := <-errc; err != nil {
		t.Fatalf("login: %v", err)
	}
	waitFor(t, "connected after re-login", 15*time.Second, func() bool { return status(m, c.ID).State == "ok" })
}
