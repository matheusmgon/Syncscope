package argocd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

func resQuery(appNs, project string, r ResourceAction) url.Values {
	q := url.Values{"namespace": {r.Namespace}, "resourceName": {r.Name}, "version": {r.Version}, "group": {r.Group}, "kind": {r.Kind}}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	if project != "" {
		q.Set("project", project)
	}
	return q
}

// ---- raw application (full object round-trip) ------------------------------

// GetApplicationRaw returns the full Application as a generic map so it can be
// edited and written back without losing fields ArgoDeck does not model.
func (c *Client) GetApplicationRaw(ctx context.Context, name, appNs string) (map[string]any, error) {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	var out map[string]any
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(name), q, &out)
	return out, err
}

// UpdateApplicationRaw writes a full Application (optimistic concurrency via
// metadata.resourceVersion). validate=true asks Argo CD to validate the spec.
func (c *Client) UpdateApplicationRaw(ctx context.Context, app map[string]any, validate bool) error {
	meta, _ := app["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	q := url.Values{"validate": {strconv.FormatBool(validate)}}
	return c.sendJSON(ctx, http.MethodPut, "/api/v1/applications/"+url.PathEscape(name), q, app, nil)
}

// PatchApplication applies a JSON merge patch to an Application.
func (c *Client) PatchApplication(ctx context.Context, name, appNs string, patch any) error {
	b, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	body := map[string]any{"name": name, "patch": string(b), "patchType": "merge"}
	if appNs != "" {
		body["appNamespace"] = appNs
	}
	return c.sendJSON(ctx, http.MethodPatch, "/api/v1/applications/"+url.PathEscape(name), nil, body, nil)
}

// ---- diff ---------------------------------------------------------------------

type ManagedResource struct {
	Group               string `json:"group"`
	Kind                string `json:"kind"`
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	LiveState           string `json:"liveState"`
	TargetState         string `json:"targetState"`
	NormalizedLiveState string `json:"normalizedLiveState"`
	PredictedLiveState  string `json:"predictedLiveState"`
	Hook                bool   `json:"hook"`
	Modified            bool   `json:"modified"`
}

func (c *Client) ManagedResources(ctx context.Context, name, appNs string) ([]ManagedResource, error) {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	var out struct {
		Items []ManagedResource `json:"items"`
	}
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(name)+"/managed-resources", q, &out)
	return out.Items, err
}

// ---- events -------------------------------------------------------------------

type Event struct {
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Count          int    `json:"count"`
	FirstTimestamp string `json:"firstTimestamp"`
	LastTimestamp  string `json:"lastTimestamp"`
	EventTime      string `json:"eventTime"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"involvedObject"`
	Source struct {
		Component string `json:"component"`
		Host      string `json:"host"`
	} `json:"source"`
	Metadata struct {
		CreationTimestamp string `json:"creationTimestamp"`
	} `json:"metadata"`
}

// Events returns the events of the app (r == nil) or of one resource.
func (c *Client) Events(ctx context.Context, name, appNs, project string, r *ResourceAction, uid string) ([]Event, error) {
	q := url.Values{}
	if appNs != "" {
		q.Set("appNamespace", appNs)
	}
	if project != "" {
		q.Set("project", project)
	}
	if r != nil {
		q.Set("resourceNamespace", r.Namespace)
		q.Set("resourceName", r.Name)
		if uid != "" {
			q.Set("resourceUID", uid)
		}
	}
	var out struct {
		Items []Event `json:"items"`
	}
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(name)+"/events", q, &out)
	return out.Items, err
}

// ---- resource actions / manifest / delete -----------------------------------

type ActionDef struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Disabled    bool   `json:"disabled"`
	IconClass   string `json:"iconClass,omitempty"`
}

func (c *Client) ListResourceActions(ctx context.Context, app, appNs, project string, r ResourceAction) ([]ActionDef, error) {
	var out struct {
		Actions []ActionDef `json:"actions"`
	}
	err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(app)+"/resource/actions", resQuery(appNs, project, r), &out)
	return out.Actions, err
}

func (c *Client) DeleteResource(ctx context.Context, app, appNs, project string, r ResourceAction, force, orphan bool) error {
	q := resQuery(appNs, project, r)
	q.Set("force", strconv.FormatBool(force))
	q.Set("orphan", strconv.FormatBool(orphan))
	return c.sendJSON(ctx, http.MethodDelete, "/api/v1/applications/"+url.PathEscape(app)+"/resource", q, nil, nil)
}

// PatchResource patches a live resource (merge patch), like "Edit" in the Argo CD UI.
func (c *Client) PatchResource(ctx context.Context, app, appNs, project string, r ResourceAction, patch string) error {
	q := resQuery(appNs, project, r)
	q.Set("patchType", "application/merge-patch+json")
	return c.sendJSON(ctx, http.MethodPost, "/api/v1/applications/"+url.PathEscape(app)+"/resource", q, patch, nil)
}
