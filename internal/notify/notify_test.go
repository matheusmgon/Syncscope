package notify

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildGroupsAndDedups(t *testing.T) {
	ms := Build([]Event{
		{Key: "a", Product: "cd", Title: "api", Detail: "CrashLoopBackOff"},
		{Key: "a", Product: "cd", Title: "api", Detail: "ImagePullBackOff"},
		{Key: "b", Product: "workflows", Title: "etl-1", Detail: "step load failed"},
		{Key: "c", Product: "cd", Title: "web", Detail: "healthy again", Good: true},
	})
	if len(ms) != 2 || ms[0].Title != "2 items failing" || !strings.Contains(ms[0].Body, "ImagePullBackOff") || ms[1].Key != "c" {
		t.Fatalf("unexpected: %+v", ms)
	}
}

func TestBatchingAndOptOut(t *testing.T) {
	var mu sync.Mutex
	var got []Message
	n := New(50*time.Millisecond, func() (bool, bool) { return true, false }, func(m Message) { mu.Lock(); got = append(got, m); mu.Unlock() })
	n.Push(Event{Key: "a", Product: "rollouts", Title: "checkout", Detail: "aborted"})
	n.Push(Event{Key: "b", Product: "cd", Title: "x", Detail: "ok", Good: true}) // recoveries disabled
	time.Sleep(120 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Key != "a" || got[0].Subtitle != "Argo Rollouts" {
		t.Fatalf("unexpected: %+v", got)
	}
}
