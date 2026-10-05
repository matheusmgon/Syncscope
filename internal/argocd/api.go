// Code generated from the methods of Client by scripts/gen-argocd-api.py; DO NOT EDIT.

package argocd

import (
	"context"
	"errors"
)

// ErrUnsupported is returned by back-ends that cannot perform an operation
// (core mode talks to Kubernetes directly, without an Argo CD API server).
var ErrUnsupported = errors.New("not available in core mode (needs an Argo CD API server)")

// API is everything Syncscope needs from an Argo CD back-end.
type API interface {
	CreateApplication(ctx context.Context, app map[string]any, upsert bool) (map[string]any, error)
	AppSyncWindows(ctx context.Context, name, appNs string) (*AppSyncWindows, error)
	CreateRepository(ctx context.Context, r RepoInput, upsert bool) error
	DeleteRepository(ctx context.Context, repo string) error
	GetProject(ctx context.Context, name string) (map[string]any, error)
	CreateProject(ctx context.Context, p map[string]any, upsert bool) error
	UpdateProject(ctx context.Context, p map[string]any) error
	DeleteProject(ctx context.Context, name string) error
	DeleteCluster(ctx context.Context, server string) error
	UpdateClusterMeta(ctx context.Context, server, name string, labels map[string]string) error
	CreateToken(ctx context.Context, account, id string, expiresInSeconds int64) (string, error)
	DeleteToken(ctx context.Context, account, id string) error
	BaseURL() string
	Credentials() Credentials
	Renew(ctx context.Context) bool
	LoginPassword(ctx context.Context, user, pass string, remember bool) error
	LoginToken(tok string)
	Logout()
	Version(ctx context.Context) (string, error)
	Settings(ctx context.Context) (*Settings, error)
	UserInfo(ctx context.Context) (*UserInfo, error)
	ListApplications(ctx context.Context) (*ApplicationList, error)
	GetApplication(ctx context.Context, name, appNs string, refresh string) (*Application, error)
	ResourceTree(ctx context.Context, name, appNs string) (*ResourceTree, error)
	ListApplicationSets(ctx context.Context) ([]ApplicationSet, error)
	ListClusters(ctx context.Context) ([]Cluster, error)
	WatchApplications(ctx context.Context, resourceVersion string, fn func(ApplicationWatchEvent)) error
	Sync(ctx context.Context, name, appNs string, o SyncOptions) error
	TerminateOperation(ctx context.Context, name, appNs string) error
	RunResourceAction(ctx context.Context, app, appNs, project string, r ResourceAction, action string) error
	Logs(ctx context.Context, app, appNs, project string, q LogQuery, fn func(LogEntry)) error
	ResourceManifest(ctx context.Context, app, appNs, project string, r ResourceAction) (map[string]any, error)
	DeleteApplication(ctx context.Context, name, appNs string, cascade bool, policy string) error
	Rollback(ctx context.Context, name, appNs string, id int64, prune, dryRun bool) error
	RevisionMetadata(ctx context.Context, name, appNs, revision string, sourceIndex int) (*RevisionMetadata, error)
	GetApplicationSet(ctx context.Context, name, ns string) (map[string]any, error)
	DeleteApplicationSet(ctx context.Context, name, ns string) error
	ListRepositories(ctx context.Context) ([]map[string]any, error)
	ListProjects(ctx context.Context) ([]map[string]any, error)
	ListAccounts(ctx context.Context) ([]map[string]any, error)
	ListClustersRaw(ctx context.Context) ([]map[string]any, error)
	RawSettings(ctx context.Context) (map[string]any, error)
	GetApplicationRaw(ctx context.Context, name, appNs string) (map[string]any, error)
	UpdateApplicationRaw(ctx context.Context, app map[string]any, validate bool) error
	PatchApplication(ctx context.Context, name, appNs string, patch any) error
	ManagedResources(ctx context.Context, name, appNs string) ([]ManagedResource, error)
	Events(ctx context.Context, name, appNs, project string, r *ResourceAction, uid string) ([]Event, error)
	ListResourceActions(ctx context.Context, app, appNs, project string, r ResourceAction) ([]ActionDef, error)
	DeleteResource(ctx context.Context, app, appNs, project string, r ResourceAction, force, orphan bool) error
	PatchResource(ctx context.Context, app, appNs, project string, r ResourceAction, patch string) error
	SSOProvider(ctx context.Context) (string, error)
	LoginSSO(ctx context.Context, port int, offlineAccess bool, openBrowser func(string)) error
	OpenTerminal(ctx context.Context, r TerminalRequest) (*Terminal, error)
}

var _ API = (*Client)(nil)

// Unsupported implements API with ErrUnsupported; embed it and override what works.
type Unsupported struct{}

func (Unsupported) CreateApplication(ctx context.Context, app map[string]any, upsert bool) (map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) AppSyncWindows(ctx context.Context, name, appNs string) (*AppSyncWindows, error) {
	return nil, ErrUnsupported
}
func (Unsupported) CreateRepository(ctx context.Context, r RepoInput, upsert bool) error {
	return ErrUnsupported
}
func (Unsupported) DeleteRepository(ctx context.Context, repo string) error { return ErrUnsupported }
func (Unsupported) GetProject(ctx context.Context, name string) (map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) CreateProject(ctx context.Context, p map[string]any, upsert bool) error {
	return ErrUnsupported
}
func (Unsupported) UpdateProject(ctx context.Context, p map[string]any) error { return ErrUnsupported }
func (Unsupported) DeleteProject(ctx context.Context, name string) error      { return ErrUnsupported }
func (Unsupported) DeleteCluster(ctx context.Context, server string) error    { return ErrUnsupported }
func (Unsupported) UpdateClusterMeta(ctx context.Context, server, name string, labels map[string]string) error {
	return ErrUnsupported
}
func (Unsupported) CreateToken(ctx context.Context, account, id string, expiresInSeconds int64) (string, error) {
	return "", ErrUnsupported
}
func (Unsupported) DeleteToken(ctx context.Context, account, id string) error { return ErrUnsupported }
func (Unsupported) BaseURL() string                                           { return "" }
func (Unsupported) Credentials() Credentials                                  { return Credentials{} }
func (Unsupported) Renew(ctx context.Context) bool                            { return false }
func (Unsupported) LoginPassword(ctx context.Context, user, pass string, remember bool) error {
	return ErrUnsupported
}
func (Unsupported) LoginToken(tok string)                           {}
func (Unsupported) Logout()                                         {}
func (Unsupported) Version(ctx context.Context) (string, error)     { return "", ErrUnsupported }
func (Unsupported) Settings(ctx context.Context) (*Settings, error) { return nil, ErrUnsupported }
func (Unsupported) UserInfo(ctx context.Context) (*UserInfo, error) { return nil, ErrUnsupported }
func (Unsupported) ListApplications(ctx context.Context) (*ApplicationList, error) {
	return nil, ErrUnsupported
}
func (Unsupported) GetApplication(ctx context.Context, name, appNs string, refresh string) (*Application, error) {
	return nil, ErrUnsupported
}
func (Unsupported) ResourceTree(ctx context.Context, name, appNs string) (*ResourceTree, error) {
	return nil, ErrUnsupported
}
func (Unsupported) ListApplicationSets(ctx context.Context) ([]ApplicationSet, error) {
	return nil, ErrUnsupported
}
func (Unsupported) ListClusters(ctx context.Context) ([]Cluster, error) { return nil, ErrUnsupported }
func (Unsupported) WatchApplications(ctx context.Context, resourceVersion string, fn func(ApplicationWatchEvent)) error {
	return ErrUnsupported
}
func (Unsupported) Sync(ctx context.Context, name, appNs string, o SyncOptions) error {
	return ErrUnsupported
}
func (Unsupported) TerminateOperation(ctx context.Context, name, appNs string) error {
	return ErrUnsupported
}
func (Unsupported) RunResourceAction(ctx context.Context, app, appNs, project string, r ResourceAction, action string) error {
	return ErrUnsupported
}
func (Unsupported) Logs(ctx context.Context, app, appNs, project string, q LogQuery, fn func(LogEntry)) error {
	return ErrUnsupported
}
func (Unsupported) ResourceManifest(ctx context.Context, app, appNs, project string, r ResourceAction) (map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) DeleteApplication(ctx context.Context, name, appNs string, cascade bool, policy string) error {
	return ErrUnsupported
}
func (Unsupported) Rollback(ctx context.Context, name, appNs string, id int64, prune, dryRun bool) error {
	return ErrUnsupported
}
func (Unsupported) RevisionMetadata(ctx context.Context, name, appNs, revision string, sourceIndex int) (*RevisionMetadata, error) {
	return nil, ErrUnsupported
}
func (Unsupported) GetApplicationSet(ctx context.Context, name, ns string) (map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) DeleteApplicationSet(ctx context.Context, name, ns string) error {
	return ErrUnsupported
}
func (Unsupported) ListRepositories(ctx context.Context) ([]map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) ListProjects(ctx context.Context) ([]map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) ListAccounts(ctx context.Context) ([]map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) ListClustersRaw(ctx context.Context) ([]map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) RawSettings(ctx context.Context) (map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) GetApplicationRaw(ctx context.Context, name, appNs string) (map[string]any, error) {
	return nil, ErrUnsupported
}
func (Unsupported) UpdateApplicationRaw(ctx context.Context, app map[string]any, validate bool) error {
	return ErrUnsupported
}
func (Unsupported) PatchApplication(ctx context.Context, name, appNs string, patch any) error {
	return ErrUnsupported
}
func (Unsupported) ManagedResources(ctx context.Context, name, appNs string) ([]ManagedResource, error) {
	return nil, ErrUnsupported
}
func (Unsupported) Events(ctx context.Context, name, appNs, project string, r *ResourceAction, uid string) ([]Event, error) {
	return nil, ErrUnsupported
}
func (Unsupported) ListResourceActions(ctx context.Context, app, appNs, project string, r ResourceAction) ([]ActionDef, error) {
	return nil, ErrUnsupported
}
func (Unsupported) DeleteResource(ctx context.Context, app, appNs, project string, r ResourceAction, force, orphan bool) error {
	return ErrUnsupported
}
func (Unsupported) PatchResource(ctx context.Context, app, appNs, project string, r ResourceAction, patch string) error {
	return ErrUnsupported
}
func (Unsupported) SSOProvider(ctx context.Context) (string, error) { return "", ErrUnsupported }
func (Unsupported) LoginSSO(ctx context.Context, port int, offlineAccess bool, openBrowser func(string)) error {
	return ErrUnsupported
}
func (Unsupported) OpenTerminal(ctx context.Context, r TerminalRequest) (*Terminal, error) {
	return nil, ErrUnsupported
}
