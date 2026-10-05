package argocd

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ---- create application ---------------------------------------------------------

// CreateApplication creates an Application from a full object (apiVersion/kind/
// metadata/spec). upsert=false fails if it already exists.
func (c *Client) CreateApplication(ctx context.Context, app map[string]any, upsert bool) (map[string]any, error) {
	q := url.Values{"upsert": {strconv.FormatBool(upsert)}, "validate": {"true"}}
	var out map[string]any
	err := c.sendJSON(ctx, http.MethodPost, "/api/v1/applications", q, app, &out)
	return out, err
}

// ---- sync windows -----------------------------------------------------------------

type SyncWindow struct {
	Kind         string   `json:"kind"`
	Schedule     string   `json:"schedule"`
	Duration     string   `json:"duration"`
	Applications []string `json:"applications,omitempty"`
	Namespaces   []string `json:"namespaces,omitempty"`
	Clusters     []string `json:"clusters,omitempty"`
	ManualSync   bool     `json:"manualSync,omitempty"`
	TimeZone     string   `json:"timeZone,omitempty"`
}

type AppSyncWindows struct {
	AssignedWindows []SyncWindow `json:"assignedWindows"`
	ActiveWindows   []SyncWindow `json:"activeWindows"`
	CanSync         bool         `json:"canSync"`
}

func (c *Client) AppSyncWindows(ctx context.Context, name, appNs string) (*AppSyncWindows, error) {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	var out AppSyncWindows
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(name)+"/syncwindows", q, &out)
	return &out, err
}

// ---- repositories ------------------------------------------------------------------

type RepoInput struct {
	Repo          string `json:"repo"`
	Type          string `json:"type"` // git | helm | oci
	Name          string `json:"name,omitempty"`
	Project       string `json:"project,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	SSHPrivateKey string `json:"sshPrivateKey,omitempty"`
	Insecure      bool   `json:"insecure,omitempty"`
	EnableOCI     bool   `json:"enableOCI,omitempty"`
}

func (c *Client) CreateRepository(ctx context.Context, r RepoInput, upsert bool) error {
	q := url.Values{"upsert": {strconv.FormatBool(upsert)}}
	return c.sendJSON(ctx, http.MethodPost, "/api/v1/repositories", q, r, nil)
}

func (c *Client) DeleteRepository(ctx context.Context, repo string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/repositories/"+url.PathEscape(repo), nil, nil, nil)
}

// ---- projects ---------------------------------------------------------------------------

func (c *Client) GetProject(ctx context.Context, name string) (map[string]any, error) {
	var out map[string]any
	err := c.getJSON(ctx, "/api/v1/projects/"+url.PathEscape(name), nil, &out)
	return out, err
}

func (c *Client) CreateProject(ctx context.Context, p map[string]any, upsert bool) error {
	return c.sendJSON(ctx, http.MethodPost, "/api/v1/projects", nil, map[string]any{"project": p, "upsert": upsert}, nil)
}

func (c *Client) UpdateProject(ctx context.Context, p map[string]any) error {
	md, _ := p["metadata"].(map[string]any)
	name, _ := md["name"].(string)
	return c.sendJSON(ctx, http.MethodPut, "/api/v1/projects/"+url.PathEscape(name), nil, map[string]any{"project": p}, nil)
}

func (c *Client) DeleteProject(ctx context.Context, name string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/projects/"+url.PathEscape(name), nil, nil, nil)
}

// ---- clusters -----------------------------------------------------------------------------

func (c *Client) DeleteCluster(ctx context.Context, server string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/clusters/"+url.PathEscape(server), nil, nil, nil)
}

// UpdateClusterMeta renames a cluster and/or replaces its labels.
func (c *Client) UpdateClusterMeta(ctx context.Context, server, name string, labels map[string]string) error {
	body := map[string]any{"server": server, "name": name, "labels": labels}
	q := url.Values{"updatedFields": {"name", "labels"}}
	return c.sendJSON(ctx, http.MethodPut, "/api/v1/clusters/"+url.PathEscape(server), q, body, nil)
}

// ---- account tokens -------------------------------------------------------------------------

func (c *Client) CreateToken(ctx context.Context, account, id string, expiresInSeconds int64) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	body := map[string]any{"name": account, "id": id, "expiresIn": expiresInSeconds}
	err := c.sendJSON(ctx, http.MethodPost, "/api/v1/account/"+url.PathEscape(account)+"/token", nil, body, &out)
	return out.Token, err
}

func (c *Client) DeleteToken(ctx context.Context, account, id string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/account/"+url.PathEscape(account)+"/token/"+url.PathEscape(id), nil, nil, nil)
}
