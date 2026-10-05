package argocd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Options configures how to reach an Argo CD API server.
type Options struct {
	Server         string // base URL including scheme, optionally a root path (https://host/argocd)
	Insecure       bool
	CAFile         string
	ClientCertFile string
	ClientKeyFile  string
	Headers        map[string]string
}

// Credentials are the secrets used to authenticate. Which fields are set
// depends on the login method.
type Credentials struct {
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
}

// ErrUnauthorized is returned when the server rejects the credentials and they
// could not be renewed automatically.
var ErrUnauthorized = errors.New("not authenticated: please log in again")

// APIError is a non-2xx answer from the Argo CD API.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return e.Message
}

type Client struct {
	base    *url.URL
	opts    Options
	http    *http.Client // regular calls
	stream  *http.Client // long-lived streams (no timeout)
	mu      sync.Mutex
	creds   Credentials
	onCreds func(Credentials)

	reauthMu sync.Mutex
	oidcMu   sync.Mutex
	oidc     *oidcInfo
}

func NewClient(opts Options, creds Credentials, onCreds func(Credentials)) (*Client, error) {
	server := strings.TrimRight(strings.TrimSpace(opts.Server), "/")
	if server == "" {
		return nil, errors.New("server URL is empty")
	}
	if !strings.Contains(server, "://") {
		server = "https://" + server
	}
	u, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	tlsCfg := &tls.Config{InsecureSkipVerify: opts.Insecure} //nolint:gosec // user opt-in, same as argocd --insecure
	if opts.CAFile != "" {
		pem, err := os.ReadFile(expandHome(opts.CAFile))
		if err != nil {
			return nil, fmt.Errorf("reading CA file: %w", err)
		}
		pool, _ := x509.SystemCertPool()
		if pool == nil {
			pool = x509.NewCertPool()
		}
		pool.AppendCertsFromPEM(pem)
		tlsCfg.RootCAs = pool
	}
	if opts.ClientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(expandHome(opts.ClientCertFile), expandHome(opts.ClientKeyFile))
		if err != nil {
			return nil, fmt.Errorf("reading client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsCfg,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &Client{
		base:    u,
		opts:    opts,
		http:    &http.Client{Transport: tr, Timeout: 60 * time.Second},
		stream:  &http.Client{Transport: tr},
		creds:   creds,
		onCreds: onCreds,
	}, nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return h + p[1:]
		}
	}
	return p
}

func (c *Client) BaseURL() string { return c.base.String() }

func (c *Client) Credentials() Credentials {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.creds
}

func (c *Client) setCredentials(cr Credentials) {
	c.mu.Lock()
	c.creds = cr
	c.mu.Unlock()
	if c.onCreds != nil {
		c.onCreds(cr)
	}
}

func (c *Client) url(path string, q url.Values) string {
	u := *c.base
	u.Path = strings.TrimRight(c.base.Path, "/") + path
	if q != nil {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (c *Client) newRequest(ctx context.Context, method, path string, q url.Values, body any) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path, q), rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range c.opts.Headers {
		req.Header.Set(k, v)
	}
	if tok := c.Credentials().Token; tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return req, nil
}

// do performs a request, renewing credentials once on 401.
func (c *Client) do(ctx context.Context, hc *http.Client, method, path string, q url.Values, body any) (*http.Response, error) {
	c.refreshIfExpiring(ctx)
	for attempt := 0; ; attempt++ {
		req, err := c.newRequest(ctx, method, path, q, body)
		if err != nil {
			return nil, err
		}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			drain(resp)
			if c.reauth(ctx, req.Header.Get("Authorization")) {
				continue
			}
			return nil, ErrUnauthorized
		}
		if resp.StatusCode == http.StatusUnauthorized {
			drain(resp)
			return nil, ErrUnauthorized
		}
		if resp.StatusCode >= 300 {
			defer drain(resp)
			return nil, readAPIError(resp)
		}
		return resp, nil
	}
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
}

func readAPIError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	msg := ""
	if json.Unmarshal(b, &e) == nil {
		msg = e.Message
		if msg == "" {
			msg = e.Error
		}
	}
	if msg == "" {
		msg = strings.TrimSpace(string(b))
		if strings.HasPrefix(msg, "<") { // HTML page from a proxy or the UI fallback
			msg = ""
		}
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
	}
	if msg == "" {
		msg = resp.Status
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	resp, err := c.do(ctx, c.http, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "text/html") {
		return &APIError{Status: resp.StatusCode, Message: "server answered with HTML instead of JSON — check the URL (root path?) or proxy"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) sendJSON(ctx context.Context, method, path string, q url.Values, body, out any) error {
	resp, err := c.do(ctx, c.http, method, path, q, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---- token lifecycle -------------------------------------------------------

func jwtExpiry(tok string) (time.Time, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(b, &claims) != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

func (c *Client) refreshIfExpiring(ctx context.Context) {
	cr := c.Credentials()
	if cr.Token == "" {
		return
	}
	if exp, ok := jwtExpiry(cr.Token); ok && time.Until(exp) < time.Minute {
		c.reauth(ctx, "Bearer "+cr.Token)
	}
}

// reauth renews credentials with a refresh token (SSO) or by logging in again
// with the stored username/password. usedAuth is the header that failed; if
// another goroutine already renewed it, we just retry.
func (c *Client) reauth(ctx context.Context, usedAuth string) bool {
	c.reauthMu.Lock()
	defer c.reauthMu.Unlock()
	cr := c.Credentials()
	if cr.Token != "" && "Bearer "+cr.Token != usedAuth {
		return true
	}
	switch {
	case cr.RefreshToken != "":
		nc, err := c.refreshOIDC(ctx, cr.RefreshToken)
		if err != nil {
			return false
		}
		c.setCredentials(nc)
		return true
	case cr.Username != "" && cr.Password != "":
		tok, err := c.passwordLogin(ctx, cr.Username, cr.Password)
		if err != nil {
			return false
		}
		cr.Token = tok
		c.setCredentials(cr)
		return true
	}
	return false
}

// Renew forces a credential renewal (refresh token or stored password). Used
// when the server reports the session as logged out without returning 401.
func (c *Client) Renew(ctx context.Context) bool {
	return c.reauth(ctx, "Bearer "+c.Credentials().Token)
}

// ---- auth endpoints --------------------------------------------------------

func (c *Client) passwordLogin(ctx context.Context, user, pass string) (string, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/session", nil, map[string]string{"username": user, "password": pass})
	if err != nil {
		return "", err
	}
	req.Header.Del("Authorization")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp)
	if resp.StatusCode >= 300 {
		return "", readAPIError(resp)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Token, nil
}

// LoginPassword authenticates a local Argo CD account. When remember is true the
// password is kept so the session can be renewed automatically.
func (c *Client) LoginPassword(ctx context.Context, user, pass string, remember bool) error {
	tok, err := c.passwordLogin(ctx, user, pass)
	if err != nil {
		return err
	}
	cr := Credentials{Token: tok, Username: user}
	if remember {
		cr.Password = pass
	}
	c.setCredentials(cr)
	return nil
}

func (c *Client) LoginToken(tok string) {
	c.setCredentials(Credentials{Token: strings.TrimSpace(tok)})
}

func (c *Client) Logout() { c.setCredentials(Credentials{}) }

// ---- read endpoints --------------------------------------------------------

func (c *Client) Version(ctx context.Context) (string, error) {
	var v Version
	if err := c.getJSON(ctx, "/api/version", nil, &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

func (c *Client) Settings(ctx context.Context) (*Settings, error) {
	var s Settings
	if err := c.getJSON(ctx, "/api/v1/settings", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) UserInfo(ctx context.Context) (*UserInfo, error) {
	var u UserInfo
	if err := c.getJSON(ctx, "/api/v1/session/userinfo", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Fields requested on list/watch. Mirrors what the Argo CD UI asks for, plus
// conditions and sync results so failures can be explained.
var appFields = []string{
	// whole metadata: ownerReferences (ApplicationSet link) must survive field filtering
	"metadata",
	"spec", "operation.sync",
	"status.sync.status", "status.sync.revision", "status.sync.revisions",
	"status.health", "status.operationState", "status.conditions",
	"status.resources", "status.reconciledAt",
}

func (c *Client) ListApplications(ctx context.Context) (*ApplicationList, error) {
	f := make([]string, 0, len(appFields)+1)
	f = append(f, "metadata.resourceVersion")
	for _, x := range appFields {
		f = append(f, "items."+x)
	}
	var out ApplicationList
	err := c.getJSON(ctx, "/api/v1/applications", url.Values{"fields": {strings.Join(f, ",")}}, &out)
	return &out, err
}

func (c *Client) GetApplication(ctx context.Context, name, appNs string, refresh string) (*Application, error) {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	if refresh != "" {
		q.Set("refresh", refresh)
	}
	var out Application
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(name), q, &out)
	return &out, err
}

func (c *Client) ResourceTree(ctx context.Context, name, appNs string) (*ResourceTree, error) {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	var out ResourceTree
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(name)+"/resource-tree", q, &out)
	return &out, err
}

func (c *Client) ListApplicationSets(ctx context.Context) ([]ApplicationSet, error) {
	var out ApplicationSetList
	err := c.getJSON(ctx, "/api/v1/applicationsets", nil, &out)
	return out.Items, err
}

func (c *Client) ListClusters(ctx context.Context) ([]Cluster, error) {
	var out ClusterList
	err := c.getJSON(ctx, "/api/v1/clusters", nil, &out)
	return out.Items, err
}

// WatchApplications streams application events until ctx is cancelled or the
// connection drops. The handler is called for each event.
func (c *Client) WatchApplications(ctx context.Context, resourceVersion string, fn func(ApplicationWatchEvent)) error {
	f := []string{"result.type"}
	for _, x := range appFields {
		f = append(f, "result.application."+x)
	}
	q := url.Values{"fields": {strings.Join(f, ",")}}
	if resourceVersion != "" {
		q.Set("resourceVersion", resourceVersion)
	}
	resp, err := c.do(ctx, c.stream, http.MethodGet, "/api/v1/stream/applications", q, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		line = bytes.TrimPrefix(line, []byte("data:"))
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var msg struct {
			Result *ApplicationWatchEvent `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Error != nil {
			return errors.New(msg.Error.Message)
		}
		if msg.Result != nil {
			fn(*msg.Result)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// ---- write endpoints -------------------------------------------------------

type SyncOptions struct {
	Prune              bool `json:"prune"`
	DryRun             bool `json:"dryRun"`
	Force              bool `json:"force"`
	ApplyOutOfSyncOnly bool `json:"applyOutOfSyncOnly"`
	// Resources limits the sync to these resources (empty = whole app).
	Resources []SyncResource `json:"resources,omitempty"`
	// Revision overrides the target revision for this sync only.
	Revision string `json:"revision,omitempty"`
}

type SyncResource struct {
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (c *Client) Sync(ctx context.Context, name, appNs string, o SyncOptions) error {
	body := map[string]any{"name": name, "prune": o.Prune, "dryRun": o.DryRun}
	if appNs != "" {
		body["appNamespace"] = appNs
	}
	if o.Force {
		body["strategy"] = map[string]any{"hook": map[string]any{"force": true}}
	}
	if o.ApplyOutOfSyncOnly {
		body["syncOptions"] = map[string]any{"items": []string{"ApplyOutOfSyncOnly=true"}}
	}
	if len(o.Resources) > 0 {
		body["resources"] = o.Resources
	}
	if o.Revision != "" {
		body["revision"] = o.Revision
	}
	return c.sendJSON(ctx, http.MethodPost, "/api/v1/applications/"+url.PathEscape(name)+"/sync", nil, body, nil)
}

func (c *Client) TerminateOperation(ctx context.Context, name, appNs string) error {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/applications/"+url.PathEscape(name)+"/operation", q, nil, nil)
}

type ResourceAction struct {
	Group, Version, Kind, Namespace, Name string
}

// RunResourceAction runs an action (e.g. "restart") on a managed resource.
// Tries the v2 endpoint (Argo CD 3.x) and falls back to the legacy one.
func (c *Client) RunResourceAction(ctx context.Context, app, appNs, project string, r ResourceAction, action string) error {
	body := map[string]any{
		"name": app, "namespace": r.Namespace, "resourceName": r.Name,
		"version": r.Version, "group": r.Group, "kind": r.Kind,
		"action": action, "project": project,
	}
	if appNs != "" {
		body["appNamespace"] = appNs
	}
	err := c.sendJSON(ctx, http.MethodPost, "/api/v1/applications/"+url.PathEscape(app)+"/resource/actions/v2", nil, body, nil)
	var ae *APIError
	if err == nil || !errors.As(err, &ae) || (ae.Status != 404 && ae.Status != 405 && ae.Status != 501) {
		return err
	}
	q := url.Values{
		"namespace": {r.Namespace}, "resourceName": {r.Name}, "version": {r.Version},
		"group": {r.Group}, "kind": {r.Kind}, "project": {project},
	}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	return c.sendJSON(ctx, http.MethodPost, "/api/v1/applications/"+url.PathEscape(app)+"/resource/actions", q, action, nil)
}
