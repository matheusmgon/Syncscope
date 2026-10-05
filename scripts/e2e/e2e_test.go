package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"syncscope/internal/argocd"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func client(t *testing.T) *argocd.Client {
	t.Helper()
	url, pass := os.Getenv("ARGOCD_E2E_URL"), os.Getenv("ARGOCD_E2E_PASSWORD")
	if url == "" || pass == "" {
		t.Skip("ARGOCD_E2E_URL / ARGOCD_E2E_PASSWORD not set")
	}
	c, err := argocd.NewClient(argocd.Options{Server: url, Insecure: true}, argocd.Credentials{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.LoginPassword(ctx, env("ARGOCD_E2E_USER", "admin"), pass, true); err != nil {
		t.Fatalf("login: %v", err)
	}
	return c
}

func find(apps []argocd.Application, name string) *argocd.Application {
	for i := range apps {
		if apps[i].Metadata.Name == name {
			return &apps[i]
		}
	}
	return nil
}

func TestE2EListApplications(t *testing.T) {
	c := client(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	list, err := c.ListApplications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if list.Metadata.ResourceVersion == "" {
		t.Error("list resourceVersion is empty (needed to start the watch)")
	}
	// scripts/e2e/manifests: guestbook, helm-guestbook, kustomize-guestbook + 2 from the ApplicationSet
	if len(list.Items) < 5 {
		t.Fatalf("expected at least 5 applications, got %d", len(list.Items))
	}
	for _, n := range []string{env("ARGOCD_E2E_APP", "guestbook"), "helm-guestbook", "kustomize-guestbook"} {
		a := find(list.Items, n)
		if a == nil {
			t.Errorf("application %s not listed", n)
			continue
		}
		if a.Spec.Project == "" || a.Spec.Destination.Namespace == "" {
			t.Errorf("%s: spec not decoded: %+v", n, a.Spec)
		}
		if a.Spec.Source == nil || a.Spec.Source.RepoURL == "" {
			t.Errorf("%s: source missing", n)
		}
	}
}

func TestE2ESyncToHealthy(t *testing.T) {
	c := client(t)
	name := env("ARGOCD_E2E_APP", "guestbook")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := c.Sync(ctx, name, "", argocd.SyncOptions{Prune: true}); err != nil {
		// a sync may already be running (e.g. started by the workflow); that is fine
		if !strings.Contains(strings.ToLower(err.Error()), "another operation is already in progress") {
			t.Fatalf("sync: %v", err)
		}
	}
	var app *argocd.Application
	for {
		var err error
		app, err = c.GetApplication(ctx, name, "", "normal")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		op := app.Status.OperationState
		done := op == nil || (op.Phase != "Running" && op.Phase != "Terminating")
		if done && app.Status.Sync.Status == "Synced" && app.Status.Health.Status == "Healthy" {
			break
		}
		if op != nil && (op.Phase == "Failed" || op.Phase == "Error") {
			t.Fatalf("sync %s: %s", op.Phase, op.Message)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out: sync=%s health=%s op=%+v", app.Status.Sync.Status, app.Status.Health.Status, op)
		case <-time.After(3 * time.Second):
		}
	}
	if app.Status.Sync.Revision == "" {
		t.Error("synced revision is empty")
	}
	if len(app.Status.Resources) == 0 {
		t.Error("status.resources is empty")
	}

	tree, err := c.ResourceTree(ctx, name, "")
	if err != nil {
		t.Fatalf("resource tree: %v", err)
	}
	kinds := map[string]int{}
	for _, n := range tree.Nodes {
		kinds[n.Kind]++
	}
	for _, k := range []string{"Deployment", "ReplicaSet", "Pod", "Service"} {
		if kinds[k] == 0 {
			t.Errorf("resource tree has no %s (kinds: %v)", k, kinds)
		}
	}
	for _, n := range tree.Nodes {
		if n.Kind == "Pod" && len(n.ParentRefs) == 0 {
			t.Errorf("pod %s has no parentRefs", n.Name)
		}
	}

	managed, err := c.ManagedResources(ctx, name, "")
	if err != nil {
		t.Fatalf("managed resources: %v", err)
	}
	var sawDeploy bool
	for _, m := range managed {
		if m.Kind == "Deployment" {
			sawDeploy = true
			if m.LiveState == "" || m.LiveState == "null" {
				t.Errorf("deployment %s has no live state", m.Name)
			}
			if m.TargetState == "" || m.TargetState == "null" {
				t.Errorf("deployment %s has no target state", m.Name)
			}
		}
	}
	if !sawDeploy {
		t.Errorf("managed resources has no Deployment (%d items)", len(managed))
	}
}

func TestE2EApplicationSet(t *testing.T) {
	c := client(t)
	setName := env("ARGOCD_E2E_APPSET", "guestbook-set")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	sets, err := c.ListApplicationSets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range sets {
		if s.Metadata.Name == setName {
			found = true
		}
	}
	if !found {
		t.Fatalf("ApplicationSet %s not listed (%d sets)", setName, len(sets))
	}

	list, err := c.ListApplications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var owned []string
	for _, a := range list.Items {
		for _, o := range a.Metadata.OwnerReferences {
			if o.Kind == "ApplicationSet" && o.Name == setName {
				owned = append(owned, a.Metadata.Name)
			}
		}
	}
	// the manifest's list generator has two elements
	if len(owned) < 2 {
		t.Fatalf("expected >= 2 apps owned by ApplicationSet %s, got %v", setName, owned)
	}
	// the plain Applications must not be attributed to the set
	if a := find(list.Items, env("ARGOCD_E2E_APP", "guestbook")); a != nil {
		for _, o := range a.Metadata.OwnerReferences {
			if o.Kind == "ApplicationSet" {
				t.Errorf("%s unexpectedly owned by %s", a.Metadata.Name, o.Name)
			}
		}
	}
}
