// Package notify turns state transitions (an app starts failing, a sync or a
// workflow fails, a rollout degrades) into batched desktop notifications.
package notify

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Key     string // what to open when clicked
	Product string // cd | workflows | rollouts | events
	Title   string // e.g. "payments-api"
	Detail  string // the reason
	Good    bool   // recovery
}

type Message struct {
	ID       string
	Title    string
	Subtitle string
	Body     string
	Key      string // single event: open it
	Product  string
}

type Notifier struct {
	mu      sync.Mutex
	pending []Event
	timer   *time.Timer
	window  time.Duration
	send    func(Message)
	enabled func() (failures, recoveries bool)
}

func New(window time.Duration, enabled func() (bool, bool), send func(Message)) *Notifier {
	return &Notifier{window: window, enabled: enabled, send: send}
}

func (n *Notifier) Push(e Event) {
	f, r := n.enabled()
	if (e.Good && !r) || (!e.Good && !f) {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending = append(n.pending, e)
	if n.timer == nil {
		n.timer = time.AfterFunc(n.window, n.flush)
	}
}

func (n *Notifier) flush() {
	n.mu.Lock()
	evs := n.pending
	n.pending, n.timer = nil, nil
	n.mu.Unlock()
	for _, m := range Build(evs) {
		n.send(m)
	}
}

var productName = map[string]string{"cd": "Argo CD", "workflows": "Argo Workflows", "rollouts": "Argo Rollouts", "events": "Argo Events"}

// Build groups events into at most two messages (failures, recoveries).
func Build(evs []Event) []Message {
	// de-duplicate per key, keeping the last state
	last := map[string]Event{}
	var order []string
	for _, e := range evs {
		if _, ok := last[e.Key]; !ok {
			order = append(order, e.Key)
		}
		last[e.Key] = e
	}
	var bad, good []Event
	for _, k := range order {
		if e := last[k]; e.Good {
			good = append(good, e)
		} else {
			bad = append(bad, e)
		}
	}
	var out []Message
	mk := func(list []Event, verb string) {
		if len(list) == 0 {
			return
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].Product < list[j].Product })
		m := Message{ID: fmt.Sprintf("syncscope-%d-%s", time.Now().UnixNano(), verb)}
		if len(list) == 1 {
			e := list[0]
			m.Title = e.Title + " " + verb
			m.Subtitle = productName[e.Product]
			m.Body = e.Detail
			m.Key, m.Product = e.Key, e.Product
		} else {
			m.Title = fmt.Sprintf("%d items %s", len(list), verb)
			var lines []string
			for i, e := range list {
				if i == 4 {
					lines = append(lines, fmt.Sprintf("… and %d more", len(list)-4))
					break
				}
				lines = append(lines, e.Title+": "+e.Detail)
			}
			m.Body = strings.Join(lines, "\n")
			m.Product = list[0].Product
		}
		if len(m.Body) > 240 {
			m.Body = m.Body[:240] + "…"
		}
		out = append(out, m)
	}
	mk(bad, "failing")
	mk(good, "recovered")
	return out
}
