package argocd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// oidcInfo describes the identity provider Argo CD delegates to: either the
// bundled Dex (issuer <server>/api/dex, client "argo-cd-cli") or an external
// OIDC provider configured in argocd-cm.
type oidcInfo struct {
	Issuer        string
	ClientID      string
	Scopes        []string
	ProviderName  string
	AuthEndpoint  string
	TokenEndpoint string
	PKCEMethods   []string
}

// SSOProvider returns a human readable name of the configured SSO provider, or
// an error if the server has no SSO configured.
func (c *Client) SSOProvider(ctx context.Context) (string, error) {
	info, err := c.oidcInfo(ctx)
	if err != nil {
		return "", err
	}
	return info.ProviderName, nil
}

func (c *Client) oidcInfo(ctx context.Context) (*oidcInfo, error) {
	c.oidcMu.Lock()
	defer c.oidcMu.Unlock()
	if c.oidc != nil {
		return c.oidc, nil
	}
	s, err := c.Settings(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading /api/v1/settings: %w", err)
	}
	info := &oidcInfo{}
	switch {
	case s.OIDCConfig != nil && s.OIDCConfig.Issuer != "":
		info.Issuer = s.OIDCConfig.Issuer
		info.ClientID = s.OIDCConfig.CLIClientID
		if info.ClientID == "" {
			info.ClientID = s.OIDCConfig.ClientID
		}
		info.Scopes = s.OIDCConfig.Scopes
		info.ProviderName = s.OIDCConfig.Name
	case s.DexConfig != nil && len(s.DexConfig.Connectors) > 0:
		info.Issuer = strings.TrimRight(c.base.String(), "/") + "/api/dex"
		info.ClientID = "argo-cd-cli"
		names := make([]string, 0, len(s.DexConfig.Connectors))
		for _, cn := range s.DexConfig.Connectors {
			names = append(names, cn.Name)
		}
		info.ProviderName = "Dex (" + strings.Join(names, ", ") + ")"
	default:
		return nil, errors.New("this Argo CD server has no SSO configured")
	}
	if len(info.Scopes) == 0 {
		info.Scopes = []string{"openid", "profile", "email", "groups"}
	}
	if !slices.Contains(info.Scopes, "openid") {
		info.Scopes = append([]string{"openid"}, info.Scopes...)
	}

	var disc struct {
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(info.Issuer, "/")+"/.well-known/openid-configuration", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery (%s): %w", info.Issuer, err)
	}
	defer drain(resp)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OIDC discovery (%s): %s", info.Issuer, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	info.AuthEndpoint = disc.AuthorizationEndpoint
	info.TokenEndpoint = disc.TokenEndpoint
	info.PKCEMethods = disc.CodeChallengeMethods
	c.oidc = info
	return info, nil
}

func randString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// LoginSSO runs the OIDC authorization-code flow with PKCE, exactly like
// `argocd login --sso`: a local callback server on localhost:<port> and the
// system browser for the actual login.
func (c *Client) LoginSSO(ctx context.Context, port int, offlineAccess bool, openBrowser func(string)) error {
	if port == 0 {
		port = 8085
	}
	info, err := c.oidcInfo(ctx)
	if err != nil {
		return err
	}
	scopes := slices.Clone(info.Scopes)
	if offlineAccess && !slices.Contains(scopes, "offline_access") {
		scopes = append(scopes, "offline_access")
	}
	redirect := fmt.Sprintf("http://localhost:%d/auth/callback", port)
	verifier := randString(48)
	state := randString(24)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {info.ClientID},
		"redirect_uri":          {redirect},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	authURL := info.AuthEndpoint
	if strings.Contains(authURL, "?") {
		authURL += "&" + q.Encode()
	} else {
		authURL += "?" + q.Encode()
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("could not listen on localhost:%d for the SSO callback (port in use? close any `argocd login --sso`): %w", port, err)
	}
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		var res result
		switch {
		case r.URL.Query().Get("error") != "":
			res.err = fmt.Errorf("%s: %s", r.URL.Query().Get("error"), r.URL.Query().Get("error_description"))
		case r.URL.Query().Get("state") != state:
			res.err = errors.New("invalid state in SSO callback")
		default:
			res.code = r.URL.Query().Get("code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if res.err != nil {
			fmt.Fprintf(w, callbackPage, "#e96d76", "Login failed", html.EscapeString(res.err.Error()))
		} else {
			fmt.Fprintf(w, callbackPage, "#18be94", "Login complete", "You can close this tab and return to ArgoDeck.")
		}
		select {
		case done <- res:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	openBrowser(authURL)

	var res result
	select {
	case res = <-done:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		return errors.New("timed out waiting for the browser login")
	}
	if res.err != nil {
		return res.err
	}
	cr, err := c.tokenRequest(ctx, info, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {res.code},
		"redirect_uri":  {redirect},
		"client_id":     {info.ClientID},
		"code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	c.setCredentials(cr)
	return nil
}

func (c *Client) refreshOIDC(ctx context.Context, refresh string) (Credentials, error) {
	info, err := c.oidcInfo(ctx)
	if err != nil {
		return Credentials{}, err
	}
	cr, err := c.tokenRequest(ctx, info, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {info.ClientID},
	})
	if err != nil {
		return Credentials{}, err
	}
	if cr.RefreshToken == "" {
		cr.RefreshToken = refresh
	}
	return cr, nil
}

func (c *Client) tokenRequest(ctx context.Context, info *oidcInfo, form url.Values) (Credentials, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, info.TokenEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Credentials{}, err
	}
	defer drain(resp)
	var tok struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tok)
	if resp.StatusCode >= 300 || tok.Error != "" {
		msg := tok.ErrorDesc
		if msg == "" {
			msg = tok.Error
		}
		if msg == "" {
			msg = resp.Status
		}
		return Credentials{}, fmt.Errorf("OIDC token exchange failed: %s", msg)
	}
	// Argo CD authenticates with the ID token, like the CLI does.
	t := tok.IDToken
	if t == "" {
		t = tok.AccessToken
	}
	if t == "" {
		return Credentials{}, errors.New("the OIDC provider returned no id_token")
	}
	return Credentials{Token: t, RefreshToken: tok.RefreshToken}, nil
}

const callbackPage = `<!doctype html><html><head><meta charset="utf-8"><title>ArgoDeck</title></head>
<body style="font-family:system-ui,sans-serif;background:#0f2733;color:#fff;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">
<div style="text-align:center"><div style="width:14px;height:14px;border-radius:50%%;background:%s;margin:0 auto 16px"></div>
<h2 style="margin:0 0 8px">%s</h2><p style="opacity:.8">%s</p></div></body></html>`
