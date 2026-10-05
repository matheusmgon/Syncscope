package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

var testAssets = []ghAsset{
	{Name: "SHA256SUMS", BrowserDownloadURL: "https://dl/SHA256SUMS"},
	{Name: "Syncscope_v1.3.0_darwin_universal.zip", BrowserDownloadURL: "https://dl/mac.zip"},
	{Name: "Syncscope_v1.3.0_windows_amd64.exe", BrowserDownloadURL: "https://dl/win.exe"},
	{Name: "Syncscope_v1.3.0_windows_amd64_installer.exe", BrowserDownloadURL: "https://dl/win-installer.exe"},
	{Name: "Syncscope_v1.3.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://dl/linux-amd64.tar.gz"},
	{Name: "Syncscope_v1.3.0_linux_arm64.tar.gz", BrowserDownloadURL: "https://dl/linux-arm64.tar.gz"},
}

func server(t *testing.T, status int, rel *ghRelease, hits *atomic.Int32) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		if r.URL.Path != "/repos/acme/syncscope/releases/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		w.WriteHeader(status)
		if rel != nil {
			_ = json.NewEncoder(w).Encode(rel)
		}
	}))
	t.Cleanup(srv.Close)
	old := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = old })
}

func latest(tag string) *ghRelease {
	return &ghRelease{
		TagName:     tag,
		HTMLURL:     "https://github.com/acme/syncscope/releases/tag/" + tag,
		PublishedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Body:        "## Changes\n- stuff",
		Assets:      testAssets,
	}
}

func TestCheckNewer(t *testing.T) {
	server(t, 200, latest("v1.3.0"), nil)
	r, err := check(context.Background(), "acme/syncscope", "v1.2.9", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected an update")
	}
	if r.Tag != "v1.3.0" || r.Version != "1.3.0" {
		t.Errorf("tag = %q version = %q", r.Tag, r.Version)
	}
	if r.URL != "https://github.com/acme/syncscope/releases/tag/v1.3.0" {
		t.Errorf("url = %q", r.URL)
	}
	if r.Notes == "" || r.PublishedAt.IsZero() {
		t.Errorf("notes/publishedAt not filled: %+v", r)
	}
	if r.AssetURL != "https://dl/linux-amd64.tar.gz" {
		t.Errorf("asset = %q", r.AssetURL)
	}
}

func TestCheckUpToDate(t *testing.T) {
	for _, cur := range []string{"v1.3.0", "1.3.0", "v1.4.0", "v2.0.0-rc.1"} {
		server(t, 200, latest("v1.3.0"), nil)
		r, err := check(context.Background(), "acme/syncscope", cur, "linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		if r != nil {
			t.Errorf("current %s: unexpected update %+v", cur, r)
		}
	}
}

func TestCheckDevNeverNotifiesAndSkipsNetwork(t *testing.T) {
	var hits atomic.Int32
	server(t, 200, latest("v9.9.9"), &hits)
	for _, cur := range []string{"dev", "", "abc123"} {
		r, err := Check(context.Background(), "acme/syncscope", cur)
		if err != nil || r != nil {
			t.Errorf("current %q: got %+v, %v; want nil, nil", cur, r, err)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("dev build hit the API %d times", hits.Load())
	}
}

func TestCheckNoReleases(t *testing.T) {
	server(t, 404, nil, nil)
	r, err := Check(context.Background(), "acme/syncscope", "v1.0.0")
	if err != nil || r != nil {
		t.Fatalf("got %+v, %v; want nil, nil", r, err)
	}
}

func TestCheckHTTPError(t *testing.T) {
	server(t, 403, nil, nil)
	if _, err := Check(context.Background(), "acme/syncscope", "v1.0.0"); err == nil {
		t.Fatal("expected error on rate limit / 403")
	}
}

func TestCheckPrereleaseIgnored(t *testing.T) {
	rel := latest("v2.0.0")
	rel.Prerelease = true
	server(t, 200, rel, nil)
	r, err := Check(context.Background(), "acme/syncscope", "v1.0.0")
	if err != nil || r != nil {
		t.Fatalf("got %+v, %v; want nil, nil", r, err)
	}
}

func TestCheckInvalidRepo(t *testing.T) {
	if _, err := Check(context.Background(), "nope", "v1.0.0"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPickAsset(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "https://dl/mac.zip"},
		{"darwin", "amd64", "https://dl/mac.zip"},
		{"windows", "amd64", "https://dl/win-installer.exe"},
		{"linux", "amd64", "https://dl/linux-amd64.tar.gz"},
		{"linux", "arm64", "https://dl/linux-arm64.tar.gz"},
		{"windows", "arm64", ""},
		{"freebsd", "amd64", ""},
	}
	for _, c := range cases {
		got := ""
		if a := pickAsset(testAssets, c.goos, c.goarch); a != nil {
			got = a.BrowserDownloadURL
		}
		if got != c.want {
			t.Errorf("%s/%s: got %q want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.4", "1.2.3", 1},
		{"1.10.0", "1.9.9", 1},
		{"2.0.0", "10.0.0", -1},
		{"1.2", "1.2.0", 0},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0+build.5", "1.0.0", 0},
	}
	for _, c := range cases {
		a, ok1 := parseSemver(c.a)
		b, ok2 := parseSemver(c.b)
		if !ok1 || !ok2 {
			t.Fatalf("parse failed: %q %q", c.a, c.b)
		}
		if got := compare(a, b); got != c.want {
			t.Errorf("compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	for _, bad := range []string{"dev", "", "v", "1.x", "1.2.3.4", "-1.0.0"} {
		if _, ok := parseSemver(bad); ok {
			t.Errorf("parseSemver(%q) accepted", bad)
		}
	}
}
