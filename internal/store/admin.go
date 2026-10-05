package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"syncscope/internal/argocd"
)

func (m *Manager) client(ctxID string) (*argocd.Client, error) {
	c, err := m.conn(ctxID)
	if err != nil {
		return nil, err
	}
	return c.client, nil
}

// CreateApp creates an Application from YAML on an instance; returns its key.
func (m *Manager) CreateApp(ctxID, y string, upsert bool) (string, error) {
	cl, err := m.client(ctxID)
	if err != nil {
		return "", err
	}
	var app map[string]any
	if err := yaml.Unmarshal([]byte(y), &app); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}
	if app == nil {
		return "", errors.New("empty manifest")
	}
	if app["kind"] == nil {
		app["kind"] = "Application"
	}
	if app["apiVersion"] == nil {
		app["apiVersion"] = "argoproj.io/v1alpha1"
	}
	if app["kind"] != "Application" {
		return "", errors.New("kind must be Application")
	}
	ctx, cancel := opCtx()
	defer cancel()
	out, err := cl.CreateApplication(ctx, app, upsert)
	if err != nil {
		return "", err
	}
	md, _ := out["metadata"].(map[string]any)
	ns, _ := md["namespace"].(string)
	name, _ := md["name"].(string)
	return appKey(ctxID, ns, name), nil
}

func (m *Manager) AppSyncWindows(key string) (*argocd.AppSyncWindows, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.AppSyncWindows(ctx, a.Metadata.Name, a.Metadata.Namespace)
}

// ---- Argo CD Image Updater ------------------------------------------------------------

const iuPrefix = "argocd-image-updater.argoproj.io/"

type ImageUpdater struct {
	Enabled     bool              `json:"enabled"`
	Annotations map[string]string `json:"annotations"` // without prefix
}

func (m *Manager) ImageUpdater(key string) (*ImageUpdater, error) {
	_, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	out := &ImageUpdater{Annotations: map[string]string{}}
	for k, v := range a.Metadata.Annotations {
		if strings.HasPrefix(k, iuPrefix) {
			out.Annotations[strings.TrimPrefix(k, iuPrefix)] = v
		}
	}
	out.Enabled = out.Annotations["image-list"] != ""
	return out, nil
}

// SetImageUpdater replaces all image updater annotations with the given ones.
func (m *Manager) SetImageUpdater(key string, ann map[string]string) error {
	c, a, err := m.resolve(key)
	if err != nil {
		return err
	}
	patch := map[string]any{}
	for k := range a.Metadata.Annotations {
		if strings.HasPrefix(k, iuPrefix) {
			patch[k] = nil
		}
	}
	for k, v := range ann {
		if strings.TrimSpace(v) != "" {
			patch[iuPrefix+k] = v
		}
	}
	ctx, cancel := opCtx()
	defer cancel()
	return c.client.PatchApplication(ctx, a.Metadata.Name, a.Metadata.Namespace, map[string]any{"metadata": map[string]any{"annotations": patch}})
}

// ---- configuration management ------------------------------------------------------

func (m *Manager) SaveRepository(ctxID string, r argocd.RepoInput, upsert bool) error {
	cl, err := m.client(ctxID)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateRepository(ctx, r, upsert)
}

func (m *Manager) DeleteRepository(ctxID, repo string) error {
	cl, err := m.client(ctxID)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteRepository(ctx, repo)
}

func (m *Manager) ProjectYAML(ctxID, name string) (string, error) {
	cl, err := m.client(ctxID)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	p, err := cl.GetProject(ctx, name)
	if err != nil {
		return "", err
	}
	clean(p, true)
	if md, ok := p["metadata"].(map[string]any); ok {
		// keep resourceVersion for optimistic updates
		_ = md
	}
	return toYAML(p), nil
}

func (m *Manager) SaveProjectYAML(ctxID, y string, create bool) error {
	cl, err := m.client(ctxID)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := yaml.Unmarshal([]byte(y), &p); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	if p == nil || p["metadata"] == nil {
		return errors.New("the project needs metadata.name")
	}
	delete(p, "status")
	ctx, cancel := opCtx()
	defer cancel()
	if create {
		return cl.CreateProject(ctx, p, false)
	}
	return cl.UpdateProject(ctx, p)
}

func (m *Manager) DeleteProject(ctxID, name string) error {
	cl, err := m.client(ctxID)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteProject(ctx, name)
}

func (m *Manager) DeleteCluster(ctxID, server string) error {
	cl, err := m.client(ctxID)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteCluster(ctx, server)
}

func (m *Manager) UpdateClusterMeta(ctxID, server, name string, labels map[string]string) error {
	cl, err := m.client(ctxID)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.UpdateClusterMeta(ctx, server, name, labels); err != nil {
		return err
	}
	c, _ := m.conn(ctxID)
	go c.refreshAux(context.Background(), true)
	return nil
}

func (m *Manager) CreateToken(ctxID, account, id string, expiresInSeconds int64) (string, error) {
	cl, err := m.client(ctxID)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateToken(ctx, account, id, expiresInSeconds)
}

func (m *Manager) DeleteToken(ctxID, account, id string) error {
	cl, err := m.client(ctxID)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteToken(ctx, account, id)
}

// NewAppTemplate returns a starter Application YAML for the create dialog,
// prefilled with the instance's first project, repository and cluster.
func (m *Manager) NewAppTemplate(ctxID string) string {
	project, repo, server := "default", "https://github.com/argoproj/argocd-example-apps.git", "https://kubernetes.default.svc"
	if c, err := m.conn(ctxID); err == nil {
		c.mu.RLock()
		servers := make([]string, 0, len(c.clusters.byServer))
		for s := range c.clusters.byServer {
			servers = append(servers, s)
		}
		c.mu.RUnlock()
		sort.Strings(servers)
		for _, s := range servers {
			if s == "https://kubernetes.default.svc" {
				server = s
			}
		}
	}
	return fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  # namespace: argocd
spec:
  project: %s
  source:
    repoURL: %s
    path: guestbook
    targetRevision: HEAD
    # helm:
    #   valueFiles: [values-prod.yaml]
  destination:
    server: %s
    namespace: my-app
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
`, project, repo, server)
}
