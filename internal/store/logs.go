package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"syncscope/internal/argocd"
)

type LogLine struct {
	Pod     string `json:"pod"`
	Time    string `json:"time"`
	Content string `json:"content"`
}

type LogRequest struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Container string `json:"container"`
	TailLines int64  `json:"tailLines"`
	Follow    bool   `json:"follow"`
	Previous  bool   `json:"previous"`
}

var logKinds = map[string]bool{"Pod": true, "Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true, "Job": true, "Rollout": true}

// HasLogs tells whether logs can be shown for a resource kind.
func HasLogs(kind string) bool { return logKinds[kind] }

type logStreams struct {
	mu     sync.Mutex
	cancel map[string]context.CancelFunc
}

// Containers lists container names (init containers first, prefixed with
// "init:" in the label only) from the live manifest of a pod or workload.
func (m *Manager) Containers(key string, r argocd.ResourceAction) ([]string, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	man, err := c.client.ResourceManifest(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, r)
	if err != nil {
		return nil, err
	}
	spec, _ := man["spec"].(map[string]any)
	if r.Kind != "Pod" {
		if t, ok := spec["template"].(map[string]any); ok {
			spec, _ = t["spec"].(map[string]any)
		}
		if r.Kind == "CronJob" {
			return nil, errors.New("CronJob: open one of its Jobs or Pods")
		}
	}
	var out []string
	for _, f := range []string{"containers", "initContainers"} {
		list, _ := spec[f].([]any)
		for _, x := range list {
			if cm, ok := x.(map[string]any); ok {
				if n, _ := cm["name"].(string); n != "" {
					out = append(out, n)
				}
			}
		}
	}
	return out, nil
}

// StartLogs starts streaming logs for a resource. Lines arrive as "logs"
// events {id, lines} batched every ~150ms; the stream ends with {id, done, error}.
func (m *Manager) StartLogs(key string, req LogRequest) (string, error) {
	c, a, err := m.resolve(key)
	if err != nil {
		return "", err
	}
	if !HasLogs(req.Kind) {
		return "", fmt.Errorf("logs are not available for %s", req.Kind)
	}
	id := fmt.Sprintf("log-%d", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	m.logs.mu.Lock()
	if m.logs.cancel == nil {
		m.logs.cancel = map[string]context.CancelFunc{}
	}
	m.logs.cancel[id] = cancel
	m.logs.mu.Unlock()

	q := argocd.LogQuery{Namespace: req.Namespace, Container: req.Container, TailLines: req.TailLines, Follow: req.Follow, Previous: req.Previous}
	if req.Kind == "Pod" {
		q.PodName = req.Name
	} else {
		q.Group, q.Kind, q.ResourceName = req.Group, req.Kind, req.Name
	}
	if q.TailLines == 0 {
		q.TailLines = 500
	}

	go func() {
		defer m.StopLogs(id)
		var mu sync.Mutex
		var buf []LogLine
		seq := 0
		flush := func() {
			mu.Lock()
			defer mu.Unlock()
			if len(buf) > 0 {
				seq++
				m.emit("logs", map[string]any{"id": id, "seq": seq, "lines": buf})
				buf = nil
			}
		}
		tick := time.NewTicker(150 * time.Millisecond)
		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-tick.C:
					flush()
				case <-done:
					return
				}
			}
		}()
		err := c.client.Logs(ctx, a.Metadata.Name, a.Metadata.Namespace, a.Spec.Project, q, func(e argocd.LogEntry) {
			mu.Lock()
			buf = append(buf, LogLine{Pod: e.PodName, Time: e.TimeStamp, Content: e.Content})
			mu.Unlock()
		})
		close(done)
		tick.Stop()
		flush()
		msg := ""
		if err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
			msg = err.Error()
		}
		mu.Lock()
		seq++
		final := seq
		mu.Unlock()
		m.emit("logs", map[string]any{"id": id, "seq": final, "done": true, "error": msg})
	}()
	return id, nil
}

func (m *Manager) StopLogs(id string) {
	m.logs.mu.Lock()
	defer m.logs.mu.Unlock()
	if cancel := m.logs.cancel[id]; cancel != nil {
		cancel()
		delete(m.logs.cancel, id)
	}
}
