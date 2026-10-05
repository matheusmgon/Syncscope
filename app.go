package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"argodeck/internal/argocd"
	"argodeck/internal/config"
	"argodeck/internal/store"
)

// App is the surface exposed to the frontend through Wails bindings.
type App struct {
	ctx context.Context
	cfg *config.Store
	m   *store.Manager
}

func NewApp(cfg *config.Store) *App {
	return &App{cfg: cfg}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.m = store.NewManager(a.cfg, func(ev string, data any) { runtime.EventsEmit(ctx, ev, data) })
	a.m.OpenBrowser = func(u string) { runtime.BrowserOpenURL(ctx, u) }
	a.m.Start(ctx)
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
