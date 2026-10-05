package store

import (
	"testing"

	"argodeck/internal/argocd"
)

func TestTreeProblemsPrefersPods(t *testing.T) {
	tree := &argocd.ResourceTree{Nodes: []argocd.ResourceNode{
		{Kind: "Deployment", Name: "app", Health: &argocd.HealthStatus{Status: "Degraded"}},
		{Kind: "Deployment", Name: "other", Health: &argocd.HealthStatus{Status: "Degraded", Message: "exceeded its progress deadline"}},
		{Kind: "Pod", Name: "app-x", Health: &argocd.HealthStatus{Status: "Degraded", Message: "back-off restarting"},
			Info: []argocd.InfoItem{{Name: "Status Reason", Value: "CrashLoopBackOff"}}},
	}}
	got := treeProblems(tree)
	if len(got) != 2 || got[0].Resource != "Pod app-x" || got[0].Message != "CrashLoopBackOff: back-off restarting" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestSummarizeSyncFailure(t *testing.T) {
	a := &argocd.Application{
		Metadata: argocd.ObjectMeta{Name: "x", Namespace: "argocd", OwnerReferences: []argocd.OwnerReference{{Kind: "ApplicationSet", Name: "set"}}},
		Status: argocd.AppStatus{
			Sync: argocd.SyncStatus{Status: "OutOfSync"}, Health: argocd.HealthStatus{Status: "Healthy"},
			OperationState: &argocd.OperationState{Phase: "Failed", Message: "one or more objects failed to apply",
				SyncResult: &argocd.SyncResult{Resources: []argocd.ResourceResult{{Kind: "Deployment", Namespace: "ns", Name: "api", Status: "SyncFailed", Message: "image: Required value"}}}},
			Conditions: []argocd.Condition{{Type: "OrphanedResourceWarning", Message: "2 orphaned"}},
		},
	}
	s := summarize("c", a, clusterInfo{}, nil)
	if s.AppSet != "set" || s.Severity != 2 || len(s.Problems) != 3 || s.Problems[1].Resource != "Deployment ns/api" || s.Problems[2].Severity != "warning" {
		t.Fatalf("unexpected: %+v", s)
	}
}
