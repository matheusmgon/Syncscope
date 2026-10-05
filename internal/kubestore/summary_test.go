package kubestore

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRolloutSummary(t *testing.T) {
	bg := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "web", "namespace": "ns"},
		"spec":     map[string]any{"strategy": map[string]any{"blueGreen": map[string]any{"activeService": "a"}}},
		"status":   map[string]any{"phase": "Paused", "pauseConditions": []any{map[string]any{"reason": "BlueGreenPause"}}},
	}}
	s := Summarize("c", "Rollout", bg)
	if len(s.Problems) != 1 || strings.Contains(s.Problems[0].Message, "step") || !strings.Contains(s.Problems[0].Message, "preview") {
		t.Fatalf("blue/green message: %+v", s.Problems)
	}
	ab := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "api", "namespace": "ns"},
		"spec":     map[string]any{"strategy": map[string]any{"canary": map[string]any{"steps": []any{map[string]any{"setWeight": int64(20)}}}}},
		"status":   map[string]any{"phase": "Degraded", "abort": true, "currentStepIndex": int64(0)},
	}}
	s = Summarize("c", "Rollout", ab)
	if s.Fields["weight"] != int64(0) || s.Severity != 2 {
		t.Fatalf("aborted rollout: weight=%v severity=%d", s.Fields["weight"], s.Severity)
	}
}
