package store

import (
	"testing"
	"time"

	"syncscope/internal/config"
)

// A connection whose goroutine never finishes (e.g. a request hanging on a dead
// network) must not freeze reconnects, the UI status or quitting the app.
func TestStuckConnectionDoesNotFreezeTheApp(t *testing.T) {
	t.Setenv("SYNCSCOPE_CONFIG_DIR", t.TempDir())
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	cfg, _ := config.Open()
	c, _ := cfg.Upsert(config.Context{Name: "stuck", Server: "https://argocd.invalid", Disabled: true})
	m := NewManager(cfg, func(string, any) {})

	stuck := newConn(m, c)
	stuck.cancel = func() {}         // cancelling does nothing…
	stuck.done = make(chan struct{}) // …and the goroutine never exits
	stuck.dirty.Store(true)
	m.conns[c.ID] = stuck

	finished := func(name string, f func()) {
		t.Helper()
		ch := make(chan struct{})
		go func() { f(); close(ch) }()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s blocked on a stuck connection", name)
		}
	}
	finished("Reconnect", func() { _ = m.Reconnect(c.ID) })
	finished("Statuses", func() { _ = m.Statuses() })
	finished("SaveCaches (quit)", m.SaveCaches)
	finished("DeleteContext", func() { _ = m.DeleteContext(c.ID) })
}
