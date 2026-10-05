package main

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"syncscope/internal/argocd"
	"syncscope/internal/config"
	"syncscope/internal/kube"
	"syncscope/internal/kubestore"
	"syncscope/internal/store"
	"syncscope/internal/updater"
)

// App is the surface exposed to the frontend through Wails bindings.
type App struct {
	ctx context.Context
	cfg *config.Store
	m   *store.Manager
	k   *kubestore.Manager
}

func NewApp(cfg *config.Store) *App {
	return &App{cfg: cfg}
}

func (a *App) shutdown(ctx context.Context) {
	if a.m != nil {
		a.m.SaveCaches()
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.m = store.NewManager(a.cfg, func(ev string, data any) { runtime.EventsEmit(ctx, ev, data) })
	a.m.OpenBrowser = func(u string) { runtime.BrowserOpenURL(ctx, u) }
	a.m.Start(ctx)
	a.k = kubestore.NewManager(a.cfg, func(ev string, data any) { runtime.EventsEmit(ctx, ev, data) })
	a.k.Start(ctx)
	go a.updateLoop(ctx)
}

// ---- contexts ----

func (a *App) Contexts() []config.Context                   { return a.cfg.Contexts() }
func (a *App) Statuses() []store.ContextStatus              { return a.m.Statuses() }
func (a *App) TestContext(c config.Context) (string, error) { return a.m.Test(c) }
func (a *App) SaveContext(c config.Context) (config.Context, error) {
	return a.m.SaveContext(c)
}
func (a *App) DeleteContext(id string) error { return a.m.DeleteContext(id) }
func (a *App) Reconnect(id string) error     { return a.m.Reconnect(id) }
func (a *App) ImportCLI() (int, error)       { return a.m.ImportCLI() }
func (a *App) CLIConfigPath() string         { return config.CLIConfigPath() }

// ---- auth ----

func (a *App) LoginSSO(id string) error { return a.m.LoginSSO(id) }
func (a *App) LoginPassword(id, user, pass string, remember bool) error {
	return a.m.LoginPassword(id, user, pass, remember)
}
func (a *App) LoginToken(id, token string) error { return a.m.LoginToken(id, token) }
func (a *App) Logout(id string) error            { return a.m.Logout(id) }

// ---- data ----

func (a *App) Apps() []store.AppSummary         { return a.m.Apps() }
func (a *App) AppSets() []store.AppSetSummary   { return a.m.AppSets() }
func (a *App) Clusters() []store.ClusterSummary { return a.m.Clusters() }
func (a *App) Detail(key string) (*store.AppDetail, error) {
	return a.m.Detail(key)
}

// ---- actions ----

func (a *App) Sync(keys []string, o argocd.SyncOptions) store.ActionReport { return a.m.Sync(keys, o) }
func (a *App) Refresh(keys []string, hard bool) store.ActionReport         { return a.m.Refresh(keys, hard) }
func (a *App) Restart(keys []string) store.ActionReport                    { return a.m.Restart(keys) }
func (a *App) Terminate(keys []string) store.ActionReport                  { return a.m.Terminate(keys) }
func (a *App) RestartResource(key string, r argocd.ResourceAction) error {
	return a.m.RestartResource(key, r)
}

func (a *App) Containers(key string, r argocd.ResourceAction) ([]string, error) {
	return a.m.Containers(key, r)
}
func (a *App) StartLogs(key string, req store.LogRequest) (string, error) {
	return a.m.StartLogs(key, req)
}

// StopLogs stops an Argo CD or a Kubernetes log stream.
func (a *App) StopLogs(id string) {
	a.m.StopLogs(id)
	a.k.StopLogs(id)
}

func (a *App) Delete(keys []string, o store.DeleteOptions) store.ActionReport {
	return a.m.Delete(keys, o)
}
func (a *App) History(key string) ([]store.HistoryEntry, error) { return a.m.History(key) }
func (a *App) Rollback(key string, id int64, prune, dryRun bool) error {
	return a.m.Rollback(key, id, prune, dryRun)
}
func (a *App) AppSetDetail(key string) (*store.AppSetDetail, error) { return a.m.AppSetDetail(key) }
func (a *App) DeleteAppSet(key string) error                        { return a.m.DeleteAppSet(key) }
func (a *App) ArgoConfig(ctxID string) (*store.ArgoConfig, error)   { return a.m.ArgoConfig(ctxID) }

// ---- phase 1: operations ----

func (a *App) SetSyncPolicy(keys []string, p store.SyncPolicy) store.ActionReport {
	return a.m.SetSyncPolicy(keys, p)
}
func (a *App) AppSetGuard(key string) (*store.AppSetGuard, error) { return a.m.AppSetGuard(key) }
func (a *App) Diff(key string) ([]store.DiffItem, error)          { return a.m.Diff(key) }
func (a *App) AppEvents(key string) ([]store.EventRow, error)     { return a.m.Events(key, nil, "") }
func (a *App) ResourceEvents(key string, r argocd.ResourceAction, uid string) ([]store.EventRow, error) {
	return a.m.Events(key, &r, uid)
}
func (a *App) ResourceActions(key string, r argocd.ResourceAction) ([]argocd.ActionDef, error) {
	return a.m.ResourceActions(key, r)
}
func (a *App) RunAction(key string, r argocd.ResourceAction, action string) error {
	return a.m.RunAction(key, r, action)
}
func (a *App) ResourceYAML(key string, r argocd.ResourceAction) (string, error) {
	return a.m.ResourceYAML(key, r)
}
func (a *App) PatchResourceYAML(key string, r argocd.ResourceAction, y string) error {
	return a.m.PatchResourceYAML(key, r, y)
}
func (a *App) DeleteResource(key string, r argocd.ResourceAction, force, orphan bool) error {
	return a.m.DeleteResource(key, r, force, orphan)
}
func (a *App) AppYAML(key string) (string, error)               { return a.m.AppYAML(key) }
func (a *App) SaveAppYAML(key, y string) error                  { return a.m.SaveAppYAML(key, y) }
func (a *App) AppSources(key string) (*store.AppSources, error) { return a.m.AppSources(key) }
func (a *App) SaveSource(key string, index int, src map[string]any) error {
	return a.m.SaveSource(key, index, src)
}
func (a *App) StartTerminal(key string, r store.TerminalRequest) (string, error) {
	return a.m.StartTerminal(key, r)
}
func (a *App) TermInput(id, data string) error            { return a.m.TermInput(id, data) }
func (a *App) TermResize(id string, rows, cols int) error { return a.m.TermResize(id, rows, cols) }
func (a *App) TermClose(id string)                        { a.m.TermClose(id) }

func (a *App) OpenInArgo(key string) error {
	u, err := a.m.WebURL(key)
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, u)
	return nil
}

func (a *App) OpenURL(u string) { runtime.BrowserOpenURL(a.ctx, u) }

func (a *App) Prefs() config.Prefs           { return a.cfg.Prefs() }
func (a *App) SetPrefs(p config.Prefs) error { return a.cfg.SetPrefs(p) }

// ---- Kubernetes-backed Argo tools (Workflows, Events, Rollouts) ----

func (a *App) KubeContexts() ([]kubestore.ContextView, error) { return a.k.Contexts() }
func (a *App) KubeconfigPaths() []string                      { return kube.KubeconfigPaths() }
func (a *App) SetKubeContexts(names []string) error           { return a.k.SetEnabled(names) }
func (a *App) KubeReconnect(name string)                      { a.k.Reconnect(name) }
func (a *App) KubeStatuses() []kubestore.ContextStatus        { return a.k.Statuses() }
func (a *App) KObjects() []kubestore.Obj                      { return a.k.Objects() }
func (a *App) KObject(key string) (map[string]any, error)     { return a.k.Object(key) }
func (a *App) KYAML(key string) (string, error)               { return a.k.YAML(key) }
func (a *App) KSaveYAML(key, y string) error                  { return a.k.SaveYAML(key, y) }
func (a *App) KDelete(key string) error                       { return a.k.DeleteObject(key) }
func (a *App) KPods(key string) ([]kube.PodInfo, error)       { return a.k.Pods(key) }
func (a *App) KEvents(key string) ([]kube.Event, error)       { return a.k.Events(key) }
func (a *App) KStartLogs(r kubestore.LogRequest) (string, error) {
	return a.k.StartLogs(r)
}
func (a *App) RolloutAction(key, action string) error { return a.k.RolloutAction(key, action) }
func (a *App) WorkflowAction(key, action string) (string, error) {
	return a.k.WorkflowAction(key, action)
}
func (a *App) SubmitTemplate(key string, params map[string]string) (string, error) {
	return a.k.SubmitTemplate(key, params)
}
func (a *App) CronAction(key, action string) (string, error) { return a.k.CronAction(key, action) }
func (a *App) RestartPods(key string) (int, error)           { return a.k.RestartPods(key) }

// ---- updates ----

func (a *App) Version() string { return version }

// CheckForUpdate asks GitHub Releases for a newer version (nil when up to date,
// for dev builds, or when no repository is configured).
func (a *App) CheckForUpdate() (*updater.Release, error) {
	if updateRepo == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return updater.Check(ctx, updateRepo, version)
}

func (a *App) updateLoop(ctx context.Context) {
	if updateRepo == "" || a.cfg.Prefs().NoUpdateCheck {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
	}
	for {
		if rel, err := a.CheckForUpdate(); err == nil && rel != nil {
			runtime.EventsEmit(ctx, "update:available", rel)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}

// ---- Argo CD: creation, sync windows, image updater, configuration ----

func (a *App) NewAppTemplate(ctxID string) string { return a.m.NewAppTemplate(ctxID) }
func (a *App) CreateApp(ctxID, y string, upsert bool) (string, error) {
	return a.m.CreateApp(ctxID, y, upsert)
}
func (a *App) AppSyncWindows(key string) (*argocd.AppSyncWindows, error) {
	return a.m.AppSyncWindows(key)
}
func (a *App) ImageUpdater(key string) (*store.ImageUpdater, error) { return a.m.ImageUpdater(key) }
func (a *App) SetImageUpdater(key string, ann map[string]string) error {
	return a.m.SetImageUpdater(key, ann)
}
func (a *App) SaveRepository(ctxID string, r argocd.RepoInput, upsert bool) error {
	return a.m.SaveRepository(ctxID, r, upsert)
}
func (a *App) DeleteRepository(ctxID, repo string) error      { return a.m.DeleteRepository(ctxID, repo) }
func (a *App) ProjectYAML(ctxID, name string) (string, error) { return a.m.ProjectYAML(ctxID, name) }
func (a *App) SaveProjectYAML(ctxID, y string, create bool) error {
	return a.m.SaveProjectYAML(ctxID, y, create)
}
func (a *App) DeleteProject(ctxID, name string) error   { return a.m.DeleteProject(ctxID, name) }
func (a *App) DeleteCluster(ctxID, server string) error { return a.m.DeleteCluster(ctxID, server) }
func (a *App) UpdateClusterMeta(ctxID, server, name string, labels map[string]string) error {
	return a.m.UpdateClusterMeta(ctxID, server, name, labels)
}
func (a *App) CreateToken(ctxID, account, id string, expiresInSeconds int64) (string, error) {
	return a.m.CreateToken(ctxID, account, id, expiresInSeconds)
}
func (a *App) DeleteToken(ctxID, account, id string) error {
	return a.m.DeleteToken(ctxID, account, id)
}
