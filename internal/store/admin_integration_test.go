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

func TestIntegrationAdmin(t *testing.T) {
	url := os.Getenv("MOCKARGO_URL")
	if url == "" {
		t.Skip("MOCKARGO_URL not set")
	}
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, _ := config.Open()
	m := NewManager(cfg, func(string, any) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	c, _ := m.SaveContext(config.Context{Name: "adm", Server: url, AuthType: config.AuthPassword})
	if err := m.LoginPassword(c.ID, "admin", "admin", false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "connected", 30*time.Second, func() bool { return status(m, c.ID).State == "ok" })

	// create application from the template
	tpl := m.NewAppTemplate(c.ID)
	key, err := m.CreateApp(c.ID, strings.Replace(tpl, "name: my-app", "name: created-by-test", 1), false)
	if err != nil || !strings.HasSuffix(key, "/created-by-test") {
		t.Fatalf("create: %v %v", key, err)
	}
	waitFor(t, "created app streamed", 5*time.Second, func() bool { _, _, err := m.resolve(key); return err == nil })
	if _, err := m.CreateApp(c.ID, strings.Replace(tpl, "name: my-app", "name: created-by-test", 1), false); err == nil {
		t.Fatal("expected conflict without upsert")
	}

	// sync windows + selective sync + image updater
	var billing, other string
	for _, a := range m.Apps() {
		if a.Project == "billing" && billing == "" {
			billing = a.Key
		}
		if a.Project != "billing" && a.Severity == 0 && other == "" {
			other = a.Key
		}
	}
	sw, err := m.AppSyncWindows(billing)
	if err != nil || sw.CanSync || len(sw.ActiveWindows) != 1 {
		t.Fatalf("sync windows: %+v %v", sw, err)
	}
	rep := m.Sync([]string{other}, argocd.SyncOptions{Resources: []argocd.SyncResource{{Group: "apps", Kind: "Deployment", Name: "app", Namespace: "x"}}})
	if rep.Failed != 0 {
		t.Fatalf("selective sync: %+v", rep)
	}
	if err := m.SetImageUpdater(other, map[string]string{"image-list": "app=ghcr.io/acme/app", "app.update-strategy": "semver"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "image updater annotations", 5*time.Second, func() bool {
		iu, _ := m.ImageUpdater(other)
		return iu != nil && iu.Enabled && iu.Annotations["app.update-strategy"] == "semver"
	})

	// configuration
	if err := m.SaveRepository(c.ID, argocd.RepoInput{Repo: "ftp://nope", Type: "git"}, false); err == nil {
		t.Fatal("expected invalid repo URL error")
	}
	if err := m.SaveRepository(c.ID, argocd.RepoInput{Repo: "https://github.com/acme/new.git", Type: "git"}, false); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteRepository(c.ID, "https://github.com/acme/new.git"); err != nil {
		t.Fatal(err)
	}
	py, err := m.ProjectYAML(c.ID, "billing")
	if err != nil || !strings.Contains(py, "syncWindows") {
		t.Fatalf("project yaml: %v %v", py, err)
	}
	if err := m.SaveProjectYAML(c.ID, py, false); err != nil {
		t.Fatal(err)
	}
	if tok, err := m.CreateToken(c.ID, "ci-bot", "t2", 3600); err != nil || tok == "" {
		t.Fatalf("token: %q %v", tok, err)
	}
	if err := m.UpdateClusterMeta(c.ID, "https://dev.k8s.example.com", "dev-renamed", map[string]string{"env": "dev"}); err != nil {
		t.Fatal(err)
	}
}
