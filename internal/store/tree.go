package store

import (
	"strings"

	"argodeck/internal/argocd"
)

// TreeNode is one node of the application resource graph, as drawn by the
// Argo CD "tree" view. Parents are given by node ID; managed resources that do
// not exist in the cluster (Missing) are included too.
type TreeNode struct {
	ID          string            `json:"id"`
	Group       string            `json:"group"`
	Version     string            `json:"version"`
	Kind        string            `json:"kind"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Parents     []string          `json:"parents"`
	Health      string            `json:"health"`
	HealthMsg   string            `json:"healthMsg,omitempty"`
	Sync        string            `json:"sync,omitempty"`
	Managed     bool              `json:"managed"`
	Hook        bool              `json:"hook"`
	Prune       bool              `json:"prune"`
	Restartable bool              `json:"restartable"`
	HasLogs     bool              `json:"hasLogs"`
	Info        map[string]string `json:"info,omitempty"`
	Images      []string          `json:"images,omitempty"`
	CreatedAt   string            `json:"createdAt,omitempty"`
}

func refKey(group, kind, ns, name string) string {
	return group + "/" + kind + "/" + ns + "/" + name
}

func isRestartable(group, kind string) bool {
	return (group == "apps" && (kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet")) ||
		(group == "argoproj.io" && kind == "Rollout")
}

func buildTree(a *argocd.Application, t *argocd.ResourceTree) []TreeNode {
	managed := map[string]argocd.ResourceStatus{}
	for _, r := range a.Status.Resources {
		managed[refKey(r.Group, r.Kind, r.Namespace, r.Name)] = r
	}
	var out []TreeNode
	seen := map[string]bool{}
	byUID := map[string]string{}
	if t != nil {
		for _, n := range t.Nodes {
			if n.UID != "" {
				byUID[n.UID] = refKey(n.Group, n.Kind, n.Namespace, n.Name)
			}
		}
		for _, n := range t.Nodes {
			id := refKey(n.Group, n.Kind, n.Namespace, n.Name)
			if seen[id] {
				continue
			}
			seen[id] = true
			tn := TreeNode{ID: id, Group: n.Group, Version: n.Version, Kind: n.Kind, Namespace: n.Namespace, Name: n.Name,
				Images: n.Images, CreatedAt: n.CreatedAt, Restartable: isRestartable(n.Group, n.Kind), HasLogs: HasLogs(n.Kind)}
			if n.Health != nil {
				tn.Health, tn.HealthMsg = n.Health.Status, n.Health.Message
			}
			for _, p := range n.ParentRefs {
				pid := ""
				if p.UID != "" {
					pid = byUID[p.UID]
				}
				if pid == "" {
					pid = refKey(p.Group, p.Kind, p.Namespace, p.Name)
				}
				tn.Parents = append(tn.Parents, pid)
			}
			if len(n.Info) > 0 {
				tn.Info = map[string]string{}
				for _, in := range n.Info {
					tn.Info[in.Name] = in.Value
				}
				if r := tn.Info["Status Reason"]; r != "" && tn.HealthMsg != "" && !strings.Contains(tn.HealthMsg, r) {
					tn.HealthMsg = r + ": " + tn.HealthMsg
				} else if r != "" && tn.HealthMsg == "" && tn.Health != "Healthy" {
					tn.HealthMsg = r
				}
			}
			if r, ok := managed[id]; ok {
				tn.Managed, tn.Sync, tn.Hook, tn.Prune = true, r.Status, r.Hook, r.RequiresPruning
				if tn.Health == "" && r.Health != nil {
					tn.Health, tn.HealthMsg = r.Health.Status, r.Health.Message
				}
			}
			out = append(out, tn)
		}
	}
	// Managed resources missing from the live tree (not created yet, or pruned).
	for id, r := range managed {
		if seen[id] {
			continue
		}
		tn := TreeNode{ID: id, Group: r.Group, Version: r.Version, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name,
			Managed: true, Sync: r.Status, Hook: r.Hook, Prune: r.RequiresPruning, Health: "Missing", Restartable: isRestartable(r.Group, r.Kind)}
		if r.Health != nil {
			tn.Health, tn.HealthMsg = r.Health.Status, r.Health.Message
		}
		out = append(out, tn)
	}
	// Drop parent links to nodes that are not in the tree (cluster-scoped owners etc.).
	ids := map[string]bool{}
	for _, n := range out {
		ids[n.ID] = true
	}
	for i := range out {
		ps := out[i].Parents[:0]
		for _, p := range out[i].Parents {
			if ids[p] {
				ps = append(ps, p)
			}
		}
		out[i].Parents = ps
	}
	return out
}
