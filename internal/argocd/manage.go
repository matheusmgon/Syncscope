package argocd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// DeleteApplication deletes an app. cascade=false keeps the managed resources;
// policy is "foreground" or "background" (ignored without cascade).
func (c *Client) DeleteApplication(ctx context.Context, name, appNs string, cascade bool, policy string) error {
	q := url.Values{"cascade": {strconv.FormatBool(cascade)}}
	if cascade && policy != "" {
		q.Set("propagationPolicy", policy)
	}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/applications/"+url.PathEscape(name), q, nil, nil)
}

func (c *Client) Rollback(ctx context.Context, name, appNs string, id int64, prune, dryRun bool) error {
	body := map[string]any{"name": name, "id": id, "prune": prune, "dryRun": dryRun}
	if appNs != "" {
		body["appNamespace"] = appNs
	}
	return c.sendJSON(ctx, http.MethodPost, "/api/v1/applications/"+url.PathEscape(name)+"/rollback", nil, body, nil)
}

type RevisionMetadata struct {
	Author  string   `json:"author,omitempty"`
	Date    string   `json:"date,omitempty"`
	Message string   `json:"message,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

func (c *Client) RevisionMetadata(ctx context.Context, name, appNs, revision string, sourceIndex int) (*RevisionMetadata, error) {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	if sourceIndex >= 0 {
		q.Set("sourceIndex", strconv.Itoa(sourceIndex))
	}
	var out RevisionMetadata
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(name)+"/revisions/"+url.PathEscape(revision)+"/metadata", q, &out)
	return &out, err
}

// GetApplicationSet returns the raw ApplicationSet object.
func (c *Client) GetApplicationSet(ctx context.Context, name, ns string) (map[string]any, error) {
	q := url.Values{}
	if ns != "" {
		q.Set("appsetNamespace", ns)
	}
	var out map[string]any
	err := c.getJSON(ctx, "/api/v1/applicationsets/"+url.PathEscape(name), q, &out)
	return out, err
}

func (c *Client) DeleteApplicationSet(ctx context.Context, name, ns string) error {
	q := url.Values{}
	if ns != "" {
		q.Set("appsetNamespace", ns)
	}
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/applicationsets/"+url.PathEscape(name), q, nil, nil)
}

// getItems fetches a list endpoint and returns its raw items.
func (c *Client) getItems(ctx context.Context, path string) ([]map[string]any, error) {
	var out struct {
		Items []map[string]any `json:"items"`
	}
	err := c.getJSON(ctx, path, nil, &out)
	return out.Items, err
}

func (c *Client) ListRepositories(ctx context.Context) ([]map[string]any, error) {
	return c.getItems(ctx, "/api/v1/repositories")
}

func (c *Client) ListProjects(ctx context.Context) ([]map[string]any, error) {
	return c.getItems(ctx, "/api/v1/projects")
}

func (c *Client) ListAccounts(ctx context.Context) ([]map[string]any, error) {
	return c.getItems(ctx, "/api/v1/account")
}

func (c *Client) ListClustersRaw(ctx context.Context) ([]map[string]any, error) {
	return c.getItems(ctx, "/api/v1/clusters")
}

// RawSettings returns /api/v1/settings as-is (UI-safe subset of argocd-cm).
func (c *Client) RawSettings(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.getJSON(ctx, "/api/v1/settings", nil, &out)
	return out, err
}

func pretty(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// Pretty is exported for the store package.
func Pretty(v any) string { return pretty(v) }
