// Package updater checks GitHub Releases for a newer version of the app.
//
// It only reports what is available; downloading and installing is left to the
// user (the release page / asset URL is opened in the browser).
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Release describes a newer release than the running version.
type Release struct {
	Tag         string    `json:"tag"`
	Version     string    `json:"version"` // tag without the leading "v"
	URL         string    `json:"url"`     // html_url of the release page
	PublishedAt time.Time `json:"publishedAt"`
	Notes       string    `json:"notes"`    // release body (Markdown)
	AssetURL    string    `json:"assetUrl"` // download URL for this OS/arch, empty if none matches
	AssetName   string    `json:"assetName"`
}

// APIBase and HTTPClient can be overridden (tests, GitHub Enterprise).
var (
	APIBase    = "https://api.github.com"
	HTTPClient = &http.Client{Timeout: 15 * time.Second}
)

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []ghAsset `json:"assets"`
}

// Check queries the latest release of repo ("owner/name") and returns it when it
// is newer than current. It returns (nil, nil) when the app is up to date, when
// current is a development build ("dev", "" or not a semver), or when the repo
// has no releases yet.
func Check(ctx context.Context, repo, current string) (*Release, error) {
	return check(ctx, repo, current, runtime.GOOS, runtime.GOARCH)
}

func check(ctx context.Context, repo, current, goos, goarch string) (*Release, error) {
	cur, ok := parseSemver(current)
	if !ok {
		// dev builds are always "outdated" but must not nag the developer
		return nil, nil
	}
	if strings.Count(repo, "/") != 1 {
		return nil, fmt.Errorf("invalid repo %q, want owner/name", repo)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(APIBase, "/")+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "syncscope-updater")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no published release yet
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var gr ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("github: decoding release: %w", err)
	}
	if gr.Draft || gr.Prerelease {
		return nil, nil
	}
	latest, ok := parseSemver(gr.TagName)
	if !ok {
		return nil, errors.New("github: latest release tag is not a version: " + gr.TagName)
	}
	if compare(latest, cur) <= 0 {
		return nil, nil
	}
	r := &Release{
		Tag:         gr.TagName,
		Version:     strings.TrimPrefix(gr.TagName, "v"),
		URL:         gr.HTMLURL,
		PublishedAt: gr.PublishedAt,
		Notes:       gr.Body,
	}
	if a := pickAsset(gr.Assets, goos, goarch); a != nil {
		r.AssetURL, r.AssetName = a.BrowserDownloadURL, a.Name
	}
	return r, nil
}

// pickAsset selects the download for goos/goarch. Names are expected to contain
// the OS (darwin/windows/linux, with common aliases); an arch match is preferred,
// and "universal" counts as a match for any arch. Checksums and signatures are
// ignored. On Windows the installer is preferred over a bare executable.
func pickAsset(assets []ghAsset, goos, goarch string) *ghAsset {
	osNames := map[string][]string{
		"darwin":  {"darwin", "macos"},
		"windows": {"windows"}, // not "win": "darwin" contains it
		"linux":   {"linux"},
	}[goos]
	if osNames == nil {
		osNames = []string{goos}
	}
	archNames := map[string][]string{
		"amd64": {"amd64", "x86_64", "x64"},
		"arm64": {"arm64", "aarch64"},
	}[goarch]
	if archNames == nil {
		archNames = []string{goarch}
	}
	containsAny := func(s string, subs []string) bool {
		for _, x := range subs {
			if strings.Contains(s, x) {
				return true
			}
		}
		return false
	}
	var best *ghAsset
	bestScore := 0
	for i := range assets {
		n := strings.ToLower(assets[i].Name)
		if strings.HasSuffix(n, ".sha256") || strings.Contains(n, "sha256sums") || strings.HasSuffix(n, ".sig") || strings.HasSuffix(n, ".asc") {
			continue
		}
		if !containsAny(n, osNames) {
			continue
		}
		score := 1
		switch {
		case containsAny(n, archNames):
			score += 4
		case strings.Contains(n, "universal"):
			score += 3
		case containsAny(n, []string{"amd64", "x86_64", "x64", "arm64", "aarch64", "386"}):
			continue // built for another arch
		}
		if goos == "windows" && (strings.Contains(n, "installer") || strings.Contains(n, "setup")) {
			score++
		}
		if score > bestScore {
			best, bestScore = &assets[i], score
		}
	}
	return best
}

type semver struct {
	major, minor, patch int
	pre                 string
}

// parseSemver accepts "v1.2.3", "1.2", "1.2.3-rc.1+build". Anything else
// (including "dev") is rejected.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v semver
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.pre, s = s[i+1:], s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 || parts[0] == "" {
		return semver{}, false
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		nums[i] = n
	}
	v.major, v.minor, v.patch = nums[0], nums[1], nums[2]
	return v, true
}

func compare(a, b semver) int {
	for _, d := range []int{a.major - b.major, a.minor - b.minor, a.patch - b.patch} {
		if d != 0 {
			if d > 0 {
				return 1
			}
			return -1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "": // release > pre-release
		return 1
	case b.pre == "":
		return -1
	}
	return comparePre(a.pre, b.pre)
}

// comparePre orders pre-release identifiers per semver 2.0 §11.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				if an > bn {
					return 1
				}
				return -1
			}
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(as) > len(bs):
		return 1
	case len(as) < len(bs):
		return -1
	}
	return 0
}
