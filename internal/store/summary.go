package store

import (
	"fmt"
	"sort"
	"strings"

	"argodeck/internal/argocd"
)

// Problem is one human-readable reason an application is unhealthy or failed.
type Problem struct {
	Severity string `json:"severity"` // error | warning
	Source   string `json:"source"`   // sync | condition | health | cluster | appset | retry
	Resource string `json:"resource,omitempty"`
	Message  string `json:"message"`
}

// AppSummary is the compact, UI-ready view of an application.
type AppSummary struct {
	Key           string            `json:"key"`
	Ctx           string            `json:"ctx"`
	Name          string            `json:"name"`
	AppNamespace  string            `json:"appNamespace"`
	Project       string            `json:"project"`
	AppSet        string            `json:"appSet"`
	Cluster       string            `json:"cluster"`
	ClusterServer string            `json:"clusterServer"`
	DestNamespace string            `json:"destNamespace"`
	Repo          string            `json:"repo"`
	Path          string            `json:"path"`
	TargetRev     string            `json:"targetRev"`
	SyncRev       string            `json:"syncRev"`
	Sync          string            `json:"sync"`
	Health        string            `json:"health"`
	HealthMsg     string            `json:"healthMsg,omitempty"`
	OpPhase       string            `json:"opPhase,omitempty"`
	OpMessage     string            `json:"opMessage,omitempty"`
	OpFinishedAt  string            `json:"opFinishedAt,omitempty"`
	OpStartedAt   string            `json:"opStartedAt,omitempty"`
	AutoSync      bool              `json:"autoSync"`
	Deleting      bool              `json:"deleting"`
	Labels        map[string]string `json:"labels,omitempty"`
	Problems      []Problem         `json:"problems,omitempty"`
	Severity      int               `json:"severity"` // 0 ok, 1 warning, 2 error
	Workloads     int               `json:"workloads"`
	ReconciledAt  string            `json:"reconciledAt,omitempty"`
	CreatedAt     string            `json:"createdAt,omitempty"`
}

func appKey(ctxID, ns, name string) string { return ctxID + "|" + ns + "/" + name }

func resName(kind, ns, name string) string {
	if ns != "" {
		return kind + " " + ns + "/" + name
	}
	return kind + " " + name
}

func shortRev(r string) string {
	if len(r) == 40 && !strings.ContainsAny(r, "./-_") {
		return r[:7]
	}
	return r
}

// restartable lists workloads that support the built-in "restart" action.
func restartable(a *argocd.Application) []argocd.ResourceAction {
	var out []argocd.ResourceAction
	for _, r := range a.Status.Resources {
		ok := (r.Group == "apps" && (r.Kind == "Deployment" || r.Kind == "StatefulSet" || r.Kind == "DaemonSet")) ||
			(r.Group == "argoproj.io" && r.Kind == "Rollout")
		if ok && !r.RequiresPruning {
			out = append(out, argocd.ResourceAction{Group: r.Group, Version: r.Version, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name})
		}
	}
	return out
}

type clusterInfo struct {
	byServer map[string]argocd.Cluster
	byName   map[string]argocd.Cluster
}

func (ci clusterInfo) state(c argocd.Cluster) (string, string) {
	st, msg := c.ConnectionState.Status, c.ConnectionState.Message
	if st == "" {
		st, msg = c.Info.ConnectionState.Status, c.Info.ConnectionState.Message
	}
	return st, msg
}

func summarize(ctxID string, a *argocd.Application, ci clusterInfo, extra []Problem) AppSummary {
	s := AppSummary{
		Key:           appKey(ctxID, a.Metadata.Namespace, a.Metadata.Name),
		Ctx:           ctxID,
		Name:          a.Metadata.Name,
		AppNamespace:  a.Metadata.Namespace,
		Project:       a.Spec.Project,
		DestNamespace: a.Spec.Destination.Namespace,
		Sync:          a.Status.Sync.Status,
		Health:        a.Status.Health.Status,
		HealthMsg:     a.Status.Health.Message,
		Labels:        a.Metadata.Labels,
		Deleting:      a.Metadata.DeletionTimestamp != nil,
		ReconciledAt:  a.Status.ReconciledAt,
		CreatedAt:     a.Metadata.CreationTimestamp,
	}
	if s.Sync == "" {
		s.Sync = "Unknown"
	}
	if s.Health == "" {
		s.Health = "Unknown"
	}
	for _, o := range a.Metadata.OwnerReferences {
		if o.Kind == "ApplicationSet" {
			s.AppSet = o.Name
		}
	}
	// cluster
	d := a.Spec.Destination
	var cl argocd.Cluster
	var hasCl bool
	if d.Name != "" {
		s.Cluster = d.Name
		cl, hasCl = ci.byName[d.Name]
		s.ClusterServer = cl.Server
	} else {
		s.ClusterServer = d.Server
		cl, hasCl = ci.byServer[d.Server]
		if hasCl && cl.Name != "" {
			s.Cluster = cl.Name
		} else {
			s.Cluster = d.Server
		}
	}
	// source
	src := a.Spec.Source
	if src == nil && len(a.Spec.Sources) > 0 {
		src = &a.Spec.Sources[0]
	}
	if src != nil {
		s.Repo = src.RepoURL
		s.Path = src.Path
		if src.Chart != "" {
			s.Path = src.Chart
		}
		s.TargetRev = src.TargetRevision
		if len(a.Spec.Sources) > 1 {
			s.Repo += fmt.Sprintf(" (+%d)", len(a.Spec.Sources)-1)
		}
	}
	s.SyncRev = shortRev(a.Status.Sync.Revision)
	if s.SyncRev == "" && len(a.Status.Sync.Revisions) > 0 {
		s.SyncRev = shortRev(a.Status.Sync.Revisions[0])
	}
	if p := a.Spec.SyncPolicy; p != nil && p.Automated != nil {
		s.AutoSync = p.Automated.Enabled == nil || *p.Automated.Enabled
	}
	s.Workloads = len(restartable(a))

	// ---- problems ---------------------------------------------------------
	var probs []Problem
	if op := a.Status.OperationState; op != nil {
		s.OpPhase, s.OpMessage, s.OpFinishedAt, s.OpStartedAt = op.Phase, op.Message, op.FinishedAt, op.StartedAt
		switch op.Phase {
		case "Failed", "Error":
			msg := op.Message
			if msg == "" {
				msg = "sync " + strings.ToLower(op.Phase)
			}
			probs = append(probs, Problem{Severity: "error", Source: "sync", Message: msg})
			if op.SyncResult != nil {
				for _, r := range op.SyncResult.Resources {
					if r.Status == "SyncFailed" || r.HookPhase == "Failed" || r.HookPhase == "Error" {
						probs = append(probs, Problem{Severity: "error", Source: "sync", Resource: resName(r.Kind, r.Namespace, r.Name), Message: r.Message})
					}
				}
			}
		case "Running":
			if op.RetryCount > 0 {
				probs = append(probs, Problem{Severity: "warning", Source: "retry",
					Message: fmt.Sprintf("sync retrying (attempt %d): %s", op.RetryCount, op.Message)})
			}
		}
	}
	for _, c := range a.Status.Conditions {
		sev := "warning"
		if strings.HasSuffix(c.Type, "Error") {
			sev = "error"
		}
		probs = append(probs, Problem{Severity: sev, Source: "condition", Resource: c.Type, Message: c.Message})
	}
	if s.Health == "Degraded" || s.Health == "Missing" {
		sev := "error"
		if s.Health == "Missing" {
			sev = "warning"
		}
		found := false
		for _, r := range a.Status.Resources {
			if r.Health != nil && (r.Health.Status == "Degraded" || r.Health.Status == "Missing") {
				found = true
				msg := r.Health.Message
				if msg == "" {
					msg = r.Health.Status
				}
				probs = append(probs, Problem{Severity: sev, Source: "health", Resource: resName(r.Kind, r.Namespace, r.Name), Message: msg})
			}
		}
		if !found && len(extra) > 0 {
			found = true
			probs = append(probs, extra...)
		}
		if !found {
			msg := s.HealthMsg
			if msg == "" {
				msg = "application is " + s.Health
			}
			probs = append(probs, Problem{Severity: sev, Source: "health", Message: msg})
		}
	}
	if hasCl {
		if st, msg := ci.state(cl); st == "Failed" {
			probs = append(probs, Problem{Severity: "error", Source: "cluster", Resource: s.Cluster, Message: "cluster unreachable: " + msg})
		}
	}
	if s.Deleting {
		probs = append(probs, Problem{Severity: "warning", Source: "condition", Message: "being deleted (since " + *a.Metadata.DeletionTimestamp + ") — if stuck, check finalizers"})
	}
	sort.SliceStable(probs, func(i, j int) bool { return probs[i].Severity == "error" && probs[j].Severity != "error" })
	if len(probs) > 25 {
		probs = append(probs[:25], Problem{Severity: "warning", Source: "condition", Message: fmt.Sprintf("… and %d more problems", len(probs)-25)})
	}
	s.Problems = probs
	for _, p := range probs {
		if p.Severity == "error" {
			s.Severity = 2
			break
		}
		s.Severity = 1
	}
	return s
}

// treeProblems extracts unhealthy nodes from a resource tree. Pods come first
// because their reason (CrashLoopBackOff, ImagePullBackOff, OOMKilled…) is
// usually the real cause; nodes without any message are dropped when others
// explain the failure.
func treeProblems(t *argocd.ResourceTree) []Problem {
	var pods, others, bare []Problem
	for _, n := range t.Nodes {
		if n.Health == nil || (n.Health.Status != "Degraded" && n.Health.Status != "Missing") {
			continue
		}
		msg := n.Health.Message
		for _, in := range n.Info {
			if in.Name == "Status Reason" && in.Value != "" && !strings.Contains(msg, in.Value) {
				if msg != "" {
					msg = in.Value + ": " + msg
				} else {
					msg = in.Value
				}
			}
		}
		sev := "error"
		if n.Health.Status == "Missing" {
			sev = "warning"
		}
		p := Problem{Severity: sev, Source: "health", Resource: resName(n.Kind, n.Namespace, n.Name), Message: msg}
		switch {
		case msg == "":
			p.Message = n.Health.Status
			bare = append(bare, p)
		case n.Kind == "Pod":
			pods = append(pods, p)
		default:
			others = append(others, p)
		}
	}
	out := append(pods, others...)
	if len(out) == 0 {
		out = bare
	}
	if len(out) > 15 {
		out = out[:15]
	}
	return out
}
