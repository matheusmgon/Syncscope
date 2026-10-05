// Package kube gives Syncscope direct access to Kubernetes clusters through the
// user's kubeconfig (exec auth plugins such as gke-gcloud-auth-plugin included),
// the way Lens does. It backs Argo Workflows, Argo Events and Argo Rollouts,
// which are plain CRDs without (or besides) a dedicated API server.
package kube

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func gvr(g, v, r string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: g, Version: v, Resource: r}
}

// Argo CRDs.
var (
	Rollouts          = gvr("argoproj.io", "v1alpha1", "rollouts")
	Workflows         = gvr("argoproj.io", "v1alpha1", "workflows")
	WorkflowTemplates = gvr("argoproj.io", "v1alpha1", "workflowtemplates")
	CronWorkflows     = gvr("argoproj.io", "v1alpha1", "cronworkflows")
	EventSources      = gvr("argoproj.io", "v1alpha1", "eventsources")
	Sensors           = gvr("argoproj.io", "v1alpha1", "sensors")
	EventBus          = gvr("argoproj.io", "v1alpha1", "eventbus")
	Applications      = gvr("argoproj.io", "v1alpha1", "applications")
	ApplicationSets   = gvr("argoproj.io", "v1alpha1", "applicationsets")
)

// Kinds maps the short kind names used across Syncscope to their resources.
var Kinds = map[string]schema.GroupVersionResource{
	"Rollout": Rollouts, "Workflow": Workflows, "WorkflowTemplate": WorkflowTemplates, "CronWorkflow": CronWorkflows,
	"EventSource": EventSources, "Sensor": Sensors, "EventBus": EventBus,
}

// ---- kubeconfig ------------------------------------------------------------------

type ContextInfo struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	User      string `json:"user"`
	Current   bool   `json:"current"`
}

func loadingRules() *clientcmd.ClientConfigLoadingRules {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	// dev/test override; KUBECONFIG is honoured by the default rules already
	if p := os.Getenv("SYNCSCOPE_KUBECONFIG"); p != "" {
		rules.Precedence = filepath.SplitList(p)
		rules.ExplicitPath = ""
	}
	return rules
}

// KubeconfigPaths returns the files the contexts are read from.
func KubeconfigPaths() []string { return loadingRules().GetLoadingPrecedence() }

func ListContexts() ([]ContextInfo, error) {
	cfg, err := loadingRules().Load()
	if err != nil {
		return nil, err
	}
	out := make([]ContextInfo, 0, len(cfg.Contexts))
	for name, c := range cfg.Contexts {
		ci := ContextInfo{Name: name, Cluster: c.Cluster, Namespace: c.Namespace, User: c.AuthInfo, Current: name == cfg.CurrentContext}
		if cl, ok := cfg.Clusters[c.Cluster]; ok {
			ci.Server = cl.Server
		}
		out = append(out, ci)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ---- client ----------------------------------------------------------------------

type Client struct {
	Context   string
	Namespace string // context default namespace
	Server    string
	Dyn       dynamic.Interface
	Core      kubernetes.Interface
}

func NewClient(contextName string) (*Client, error) {
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName})
	rc, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig context %q: %w", contextName, err)
	}
	ns, _, _ := cc.Namespace()
	return newFromRest(contextName, ns, rc)
}

func newFromRest(name, ns string, rc *rest.Config) (*Client, error) {
	rc.QPS, rc.Burst = 50, 100
	rc.UserAgent = "Syncscope"
	dyn, err := dynamic.NewForConfig(rc)
	if err != nil {
		return nil, err
	}
	core, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, err
	}
	if ns == "" {
		ns = "default"
	}
	return &Client{Context: name, Namespace: ns, Server: rc.Host, Dyn: dyn, Core: core}, nil
}

// Version checks connectivity (and triggers exec auth plugins).
func (c *Client) Version(ctx context.Context) (string, error) {
	v, err := c.Core.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return v.GitVersion, nil
}

// ErrNotInstalled means the CRD is not present in the cluster.
var ErrNotInstalled = errors.New("not installed in this cluster")

func classify(err error) error {
	if apierrors.IsNotFound(err) {
		return ErrNotInstalled
	}
	return err
}

func (c *Client) res(r schema.GroupVersionResource, ns string) dynamic.ResourceInterface {
	if ns == "" {
		return c.Dyn.Resource(r)
	}
	return c.Dyn.Resource(r).Namespace(ns)
}

func (c *Client) Get(ctx context.Context, r schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
	return c.res(r, ns).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) Create(ctx context.Context, r schema.GroupVersionResource, ns string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.res(r, ns).Create(ctx, obj, metav1.CreateOptions{})
}

func (c *Client) Delete(ctx context.Context, r schema.GroupVersionResource, ns, name string) error {
	return c.res(r, ns).Delete(ctx, name, metav1.DeleteOptions{})
}

// MergePatch applies a JSON merge patch, optionally to a subresource ("status").
func (c *Client) MergePatch(ctx context.Context, r schema.GroupVersionResource, ns, name string, patch []byte, subresource string) error {
	var sub []string
	if subresource != "" {
		sub = []string{subresource}
	}
	_, err := c.res(r, ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}, sub...)
	return err
}

// ---- mirror (list + watch) ---------------------------------------------------------

// MirrorHandler receives the state of one resource type in one cluster.
type MirrorHandler struct {
	Reset  func(items []unstructured.Unstructured) // full list (also after reconnects)
	Upsert func(obj *unstructured.Unstructured)
	Delete func(ns, name string)
	State  func(state, msg string) // ok | connecting | error | missing | forbidden
}

// Mirror keeps a resource type in sync until ctx is cancelled: list all
// namespaces (falling back to the context namespace when cluster-wide list is
// forbidden), then watch from the list's resourceVersion; relist on errors.
func (c *Client) Mirror(ctx context.Context, r schema.GroupVersionResource, h MirrorHandler) {
	backoff := time.Second
	ns := ""
	for ctx.Err() == nil {
		h.State("connecting", "")
		list, err := c.res(r, ns).List(ctx, metav1.ListOptions{})
		if err != nil && apierrors.IsForbidden(err) && ns == "" {
			ns = c.Namespace // retry namespaced
			continue
		}
		if err != nil {
			switch {
			case apierrors.IsNotFound(err):
				h.State("missing", "CRD not installed")
				backoff = 5 * time.Minute // check again later (CRD may be installed meanwhile)
			case apierrors.IsForbidden(err):
				h.State("forbidden", err.Error())
				backoff = 5 * time.Minute
			default:
				h.State("error", err.Error())
			}
			if !sleep(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		h.Reset(list.Items)
		msg := ""
		if ns != "" {
			msg = "namespace " + ns + " only (no cluster-wide permission)"
		}
		h.State("ok", msg)
		w, err := c.res(r, ns).Watch(ctx, metav1.ListOptions{ResourceVersion: list.GetResourceVersion(), AllowWatchBookmarks: true})
		if err != nil {
			h.State("error", err.Error())
			if !sleep(ctx, backoff) {
				return
			}
			continue
		}
		for ev := range w.ResultChan() {
			obj, ok := ev.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			switch ev.Type {
			case watch.Added, watch.Modified:
				h.Upsert(obj)
			case watch.Deleted:
				h.Delete(obj.GetNamespace(), obj.GetName())
			case watch.Error:
				// expired resourceVersion etc. → relist
			}
		}
		w.Stop()
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// StripNoise removes managedFields and last-applied annotations from an object.
func StripNoise(o map[string]any) {
	md, _ := o["metadata"].(map[string]any)
	if md == nil {
		return
	}
	delete(md, "managedFields")
	if an, ok := md["annotations"].(map[string]any); ok {
		delete(an, "kubectl.kubernetes.io/last-applied-configuration")
	}
}

// IsForbidden / IsNotFound helpers for callers.
func IsForbidden(err error) bool     { return apierrors.IsForbidden(err) }
func IsNotFound(err error) bool      { return apierrors.IsNotFound(err) || errors.Is(err, ErrNotInstalled) }
func IsAlreadyExists(err error) bool { return apierrors.IsAlreadyExists(err) }

// ShortErr trims verbose Kubernetes errors for the UI.
func ShortErr(err error) string {
	if err == nil {
		return ""
	}
	s := classify(err).Error()
	if i := strings.Index(s, ": Get \""); i > 0 && len(s) > 300 {
		s = s[:i]
	}
	return s
}
