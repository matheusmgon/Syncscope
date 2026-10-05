package kubestore

// MOCKKUBE_KUBECONFIG=/tmp/k.yaml go test ./internal/kubestore -run Integration -v
// (start: go run ./cmd/mockkube -port 8097 -name prod -kubeconfig /tmp/k.yaml)

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"syncscope/internal/config"
)

func wait(t *testing.T, what string, d time.Duration, f func() bool, dbg ...func()) {
	t.Helper()
	defer func() {
		if t.Failed() {
			for _, x := range dbg {
				x()
			}
		}
	}()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", what)
}

func find(m *Manager, kind, nameContains string, pred func(Obj) bool) *Obj {
	for _, o := range m.Objects() {
		if o.Kind == kind && strings.Contains(o.Name, nameContains) && (pred == nil || pred(o)) {
			return &o
		}
	}
	return nil
}

func TestIntegration(t *testing.T) {
	kc := os.Getenv("MOCKKUBE_KUBECONFIG")
	if kc == "" {
		t.Skip("MOCKKUBE_KUBECONFIG not set")
	}
	t.Setenv("SYNCSCOPE_KUBECONFIG", kc)
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, _ := config.Open()
	var mu sync.Mutex
	var logLines int
	m := NewManager(cfg, func(ev string, d any) {
		if ev == "logs" {
			if l, ok := d.(map[string]any)["lines"]; ok {
				mu.Lock()
				logLines += lenAny(l)
				mu.Unlock()
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	ctxs, err := m.Contexts()
	if err != nil || len(ctxs) == 0 {
		t.Fatalf("contexts: %v %v", ctxs, err)
	}
	if err := m.SetEnabled([]string{ctxs[0].Name}); err != nil {
		t.Fatal(err)
	}
	wait(t, "all kinds mirrored", 15*time.Second, func() bool {
		st := m.Statuses()
		if len(st) != 1 || st[0].State != "ok" {
			return false
		}
		for _, k := range []string{"Rollout", "Workflow", "WorkflowTemplate", "CronWorkflow", "EventSource", "Sensor", "EventBus"} {
			if st[0].Kinds[k].State != "ok" || st[0].Kinds[k].Count == 0 {
				return false
			}
		}
		return true
	}, func() { t.Logf("statuses: %+v", m.Statuses()) })
	t.Logf("status: %+v", m.Statuses()[0].Kinds)

	// problems computed per kind
	deg := find(m, "Rollout", "", func(o Obj) bool { return o.Phase == "Degraded" })
	failed := find(m, "Workflow", "", func(o Obj) bool { return o.Phase == "Failed" })
	es := find(m, "EventSource", "kafka", nil)
	cronBad := find(m, "CronWorkflow", "sync-crm", nil)
	if deg == nil || deg.Severity != 2 || failed == nil || failed.Severity != 2 || es == nil || es.Severity != 2 || cronBad == nil || cronBad.Severity != 2 {
		t.Fatalf("problems not detected: %+v %+v %+v %+v", deg, failed, es, cronBad)
	}
	var stepMsg bool
	for _, p := range failed.Problems {
		if p.Source == "step" && strings.Contains(p.Message, "Connection refused") {
			stepMsg = true
		}
	}
	if !stepMsg {
		t.Fatalf("failed step not explained: %+v", failed.Problems)
	}

	// rollout actions
	paused := find(m, "Rollout", "", func(o Obj) bool { return o.Phase == "Paused" && o.Fields["strategy"] == "canary" })
	if err := m.RolloutAction(paused.Key, "promote"); err != nil {
		t.Fatal(err)
	}
	wait(t, "rollout promoted", 5*time.Second, func() bool {
		o := find(m, "Rollout", paused.Name, nil)
		return o != nil && o.Phase == "Progressing" && o.Fields["step"] == int64(2)
	})
	if err := m.RolloutAction(paused.Key, "abort"); err != nil {
		t.Fatal(err)
	}
	wait(t, "rollout aborted", 5*time.Second, func() bool { o := find(m, "Rollout", paused.Name, nil); return o != nil && o.Phase == "Degraded" })

	// workflow actions
	running := find(m, "Workflow", "", func(o Obj) bool { return o.Phase == "Running" })
	if _, err := m.WorkflowAction(running.Key, "stop"); err != nil {
		t.Fatal(err)
	}
	wait(t, "workflow stopped", 5*time.Second, func() bool { o := find(m, "Workflow", running.Name, nil); return o != nil && o.Phase == "Failed" })
	newKey, err := m.WorkflowAction(failed.Key, "resubmit")
	if err != nil || !strings.Contains(newKey, "|Workflow|") {
		t.Fatalf("resubmit: %v %v", newKey, err)
	}
	tmpl := find(m, "WorkflowTemplate", "etl-daily", nil)
	subKey, err := m.SubmitTemplate(tmpl.Key, map[string]string{"env": "staging"})
	if err != nil {
		t.Fatal(err)
	}
	wait(t, "submitted workflow visible", 5*time.Second, func() bool {
		for _, o := range m.Objects() {
			if o.Key == subKey {
				return true
			}
		}
		return false
	})
	if _, err := m.CronAction(cronBad.Key, "suspend"); err != nil {
		t.Fatal(err)
	}
	wait(t, "cron suspended", 5*time.Second, func() bool { o := find(m, "CronWorkflow", "sync-crm", nil); return o.Phase == "Suspended" })

	// pods, logs, events, yaml
	pods, err := m.Pods(failed.Key)
	if err != nil || len(pods) == 0 {
		t.Fatalf("pods: %v %v", pods, err)
	}
	if _, err := m.StartLogs(LogRequest{Ctx: ctxs[0].Name, Namespace: pods[0].Namespace, Pod: pods[0].Name, Container: "main", TailLines: 50}); err != nil {
		t.Fatal(err)
	}
	wait(t, "logs", 5*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return logLines > 0 })
	evs, err := m.Events(deg.Key)
	if err != nil || len(evs) == 0 {
		t.Fatalf("events: %v %v", evs, err)
	}
	y, err := m.YAML(tmpl.Key)
	if err != nil || !strings.Contains(y, "kind: WorkflowTemplate") || strings.Contains(y, "managedFields") {
		t.Fatalf("yaml: %v", err)
	}
	if err := m.SaveYAML(tmpl.Key, strings.Replace(y, "entrypoint: main", "entrypoint: extract", 1)); err != nil {
		t.Fatal(err)
	}
	if n, err := m.RestartPods(es.Key); err != nil || n == 0 {
		t.Fatalf("restart pods: %d %v", n, err)
	}
}

func lenAny(v any) int {
	switch x := v.(type) {
	case []any:
		return len(x)
	default:
		// typed slices from StartLogs
		return 1
	}
}
