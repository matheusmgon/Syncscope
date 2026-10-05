// Package core implements argocd.API on top of the Kubernetes API, like
// `argocd --core`: Applications and ApplicationSets are read and written as
// CRDs in the Argo CD namespace, without an Argo CD API server. Operations that
// need the API server's logic (resource tree, diff, rollback, resource actions,
// repositories, projects…) return argocd.ErrUnsupported.
package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"

	"syncscope/internal/argocd"
	"syncscope/internal/kube"
)

type Client struct {
	argocd.Unsupported
	kc   *kube.Client
	name string
	ns   string
}

var _ argocd.API = (*Client)(nil)

func New(kubeContext, namespace string) (*Client, error) {
	kc, err := kube.NewClient(kubeContext)
	if err != nil {
		return nil, err
	}
	if namespace == "" {
		namespace = "argocd"
	}
	return &Client{kc: kc, name: kubeContext, ns: namespace}, nil
}

func ns(appNs, def string) string {
	if appNs != "" {
		return appNs
	}
	return def
}

func convert(u map[string]any, out any) error {
	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// ---- identity -------------------------------------------------------------------------

func (c *Client) BaseURL() string                 { return "kube://" + c.name + "/" + c.ns }
func (c *Client) Credentials() argocd.Credentials { return argocd.Credentials{Token: "core"} }
func (c *Client) Renew(ctx context.Context) bool  { return false }
func (c *Client) SSOProvider(context.Context) (string, error) {
	return "", fmt.Errorf("core mode uses your kubeconfig credentials")
}

func (c *Client) Version(ctx context.Context) (string, error) {
	v, err := c.kc.Version(ctx)
	if err != nil {
		return "", err
	}
	return "core · Kubernetes " + v, nil
}

func (c *Client) UserInfo(ctx context.Context) (*argocd.UserInfo, error) {
	return &argocd.UserInfo{LoggedIn: true, Username: "kubeconfig:" + c.name}, nil
}

// ---- applications -----------------------------------------------------------------

func (c *Client) ListApplications(ctx context.Context) (*argocd.ApplicationList, error) {
	list, err := c.kc.Dyn.Resource(kube.Applications).Namespace(c.ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := &argocd.ApplicationList{Metadata: argocd.ListMeta{ResourceVersion: list.GetResourceVersion()}}
	for _, it := range list.Items {
		var a argocd.Application
		if convert(it.Object, &a) == nil {
			out.Items = append(out.Items, a)
		}
	}
	return out, nil
}

func (c *Client) WatchApplications(ctx context.Context, rv string, fn func(argocd.ApplicationWatchEvent)) error {
	w, err := c.kc.Dyn.Resource(kube.Applications).Namespace(c.ns).Watch(ctx, metav1.ListOptions{ResourceVersion: rv})
	if err != nil {
		return err
	}
	defer w.Stop()
	for ev := range w.ResultChan() {
		u, ok := ev.Object.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		var a argocd.Application
		if convert(u.Object, &a) != nil {
			continue
		}
		t := map[watch.EventType]string{watch.Added: "ADDED", watch.Modified: "MODIFIED", watch.Deleted: "DELETED"}[ev.Type]
		if t != "" {
			fn(argocd.ApplicationWatchEvent{Type: t, Application: a})
		}
	}
	return fmt.Errorf("watch closed")
}

func (c *Client) GetApplicationRaw(ctx context.Context, name, appNs string) (map[string]any, error) {
	u, err := c.kc.Get(ctx, kube.Applications, ns(appNs, c.ns), name)
	if err != nil {
		return nil, err
	}
	kube.StripNoise(u.Object)
	return u.Object, nil
}

func (c *Client) GetApplication(ctx context.Context, name, appNs, refresh string) (*argocd.Application, error) {
	if refresh != "" {
		p := fmt.Sprintf(`{"metadata":{"annotations":{"argocd.argoproj.io/refresh":%q}}}`, refresh)
		if err := c.kc.MergePatch(ctx, kube.Applications, ns(appNs, c.ns), name, []byte(p), ""); err != nil {
			return nil, err
		}
	}
	raw, err := c.GetApplicationRaw(ctx, name, appNs)
	if err != nil {
		return nil, err
	}
	var a argocd.Application
	return &a, convert(raw, &a)
}

func (c *Client) UpdateApplicationRaw(ctx context.Context, app map[string]any, validate bool) error {
	u := &unstructured.Unstructured{Object: app}
	_, err := c.kc.Dyn.Resource(kube.Applications).Namespace(ns(u.GetNamespace(), c.ns)).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func (c *Client) PatchApplication(ctx context.Context, name, appNs string, patch any) error {
	b, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	return c.kc.MergePatch(ctx, kube.Applications, ns(appNs, c.ns), name, b, "")
}

// Sync sets the operation field, which the application controller picks up
// (this is exactly what the API server does).
func (c *Client) Sync(ctx context.Context, name, appNs string, o argocd.SyncOptions) error {
	sync := map[string]any{"prune": o.Prune, "dryRun": o.DryRun}
	if o.Revision != "" {
		sync["revision"] = o.Revision
	}
	if o.Force {
		sync["syncStrategy"] = map[string]any{"hook": map[string]any{"force": true}}
	}
	if o.ApplyOutOfSyncOnly {
		sync["syncOptions"] = []string{"ApplyOutOfSyncOnly=true"}
	}
	if len(o.Resources) > 0 {
		sync["resources"] = o.Resources
	}
	raw, err := c.GetApplicationRaw(ctx, name, appNs)
	if err != nil {
		return err
	}
	if op, ok := raw["operation"]; ok && op != nil {
		return fmt.Errorf("another operation is already in progress")
	}
	patch := map[string]any{"operation": map[string]any{
		"initiatedBy": map[string]any{"username": "syncscope (core)"},
		"sync":        sync,
		"retry":       map[string]any{},
	}}
	return c.PatchApplication(ctx, name, appNs, patch)
}

func (c *Client) TerminateOperation(ctx context.Context, name, appNs string) error {
	return c.PatchApplication(ctx, name, appNs, map[string]any{"status": map[string]any{"operationState": map[string]any{"phase": "Terminating"}}})
}

func (c *Client) DeleteApplication(ctx context.Context, name, appNs string, cascade bool, policy string) error {
	fin := []string{}
	if cascade {
		f := "resources-finalizer.argocd.argoproj.io"
		if policy == "background" {
			f += "/background"
		}
		fin = []string{f}
	}
	if err := c.PatchApplication(ctx, name, appNs, map[string]any{"metadata": map[string]any{"finalizers": fin}}); err != nil {
		return err
	}
	return c.kc.Delete(ctx, kube.Applications, ns(appNs, c.ns), name)
}

func (c *Client) CreateApplication(ctx context.Context, app map[string]any, upsert bool) (map[string]any, error) {
	u := &unstructured.Unstructured{Object: app}
	if u.GetNamespace() == "" {
		u.SetNamespace(c.ns)
	}
	out, err := c.kc.Create(ctx, kube.Applications, u.GetNamespace(), u)
	if err != nil && upsert && kube.IsAlreadyExists(err) {
		cur, gerr := c.kc.Get(ctx, kube.Applications, u.GetNamespace(), u.GetName())
		if gerr != nil {
			return nil, gerr
		}
		u.SetResourceVersion(cur.GetResourceVersion())
		out, err = c.kc.Dyn.Resource(kube.Applications).Namespace(u.GetNamespace()).Update(ctx, u, metav1.UpdateOptions{})
	}
	if err != nil {
		return nil, err
	}
	return out.Object, nil
}

// ResourceTree is approximated from status.resources (no live children such as
// ReplicaSets or Pods, which only the controller's cache knows about).
func (c *Client) ResourceTree(ctx context.Context, name, appNs string) (*argocd.ResourceTree, error) {
	a, err := c.GetApplication(ctx, name, appNs, "")
	if err != nil {
		return nil, err
	}
	t := &argocd.ResourceTree{}
	for _, r := range a.Status.Resources {
		t.Nodes = append(t.Nodes, argocd.ResourceNode{Group: r.Group, Version: r.Version, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name, Health: r.Health})
	}
	return t, nil
}

// ---- application sets / clusters ---------------------------------------------------

func (c *Client) ListApplicationSets(ctx context.Context) ([]argocd.ApplicationSet, error) {
	list, err := c.kc.Dyn.Resource(kube.ApplicationSets).Namespace(c.ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if kube.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []argocd.ApplicationSet
	for _, it := range list.Items {
		var s argocd.ApplicationSet
		if convert(it.Object, &s) == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func (c *Client) GetApplicationSet(ctx context.Context, name, nsName string) (map[string]any, error) {
	u, err := c.kc.Get(ctx, kube.ApplicationSets, ns(nsName, c.ns), name)
	if err != nil {
		return nil, err
	}
	kube.StripNoise(u.Object)
	return u.Object, nil
}

func (c *Client) DeleteApplicationSet(ctx context.Context, name, nsName string) error {
	return c.kc.Delete(ctx, kube.ApplicationSets, ns(nsName, c.ns), name)
}

// ListClusters reads the cluster secrets (when allowed) plus the in-cluster target.
func (c *Client) ListClusters(ctx context.Context) ([]argocd.Cluster, error) {
	out := []argocd.Cluster{{Server: "https://kubernetes.default.svc", Name: "in-cluster"}}
	secrets, err := c.kc.Core.CoreV1().Secrets(c.ns).List(ctx, metav1.ListOptions{LabelSelector: "argocd.argoproj.io/secret-type=cluster"})
	if err != nil {
		return out, nil // no permission: in-cluster only
	}
	for _, s := range secrets.Items {
		cl := argocd.Cluster{Server: str(s, "server"), Name: str(s, "name")}
		if cl.Server != "" && cl.Server != "https://kubernetes.default.svc" {
			out = append(out, cl)
		}
	}
	return out, nil
}

func str(s corev1.Secret, k string) string {
	if v, ok := s.Data[k]; ok {
		return string(v)
	}
	if v, ok := s.StringData[k]; ok {
		return v
	}
	if v, ok := s.Annotations[k]; ok {
		if b, err := base64.StdEncoding.DecodeString(v); err == nil {
			return string(b)
		}
	}
	return ""
}

// ---- logs & events (only for workloads in the same cluster) ------------------------

func (c *Client) Logs(ctx context.Context, app, appNs, project string, q argocd.LogQuery, fn func(argocd.LogEntry)) error {
	if q.PodName == "" {
		return fmt.Errorf("in core mode, open logs from a single pod")
	}
	return c.kc.Logs(ctx, q.Namespace, q.PodName, kube.LogOptions{Container: q.Container, TailLines: q.TailLines, Follow: q.Follow, Previous: q.Previous},
		func(ts, line string) { fn(argocd.LogEntry{Content: line, TimeStamp: ts, PodName: q.PodName}) })
}

func (c *Client) Events(ctx context.Context, name, appNs, project string, r *argocd.ResourceAction, uid string) ([]argocd.Event, error) {
	target, kind, n := ns(appNs, c.ns), "Application", name
	if r != nil {
		target, kind, n = r.Namespace, r.Kind, r.Name
	}
	evs, err := c.kc.Events(ctx, target, kind, n)
	if err != nil {
		return nil, err
	}
	out := make([]argocd.Event, 0, len(evs))
	for _, e := range evs {
		var ev argocd.Event
		ev.Type, ev.Reason, ev.Message, ev.Count = e.Type, e.Reason, e.Message, int(e.Count)
		ev.FirstTimestamp, ev.LastTimestamp = e.First, e.Last
		parts := strings.SplitN(e.Object, "/", 2)
		if len(parts) == 2 {
			ev.InvolvedObject.Kind, ev.InvolvedObject.Name = parts[0], parts[1]
		}
		ev.Source.Component = e.Component
		out = append(out, ev)
	}
	return out, nil
}
