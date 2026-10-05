package kube

import (
	"bufio"
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

type PodInfo struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Phase       string            `json:"phase"`
	Reason      string            `json:"reason,omitempty"`
	Node        string            `json:"node,omitempty"`
	Containers  []string          `json:"containers"`
	Init        []string          `json:"init,omitempty"`
	Ready       string            `json:"ready"`
	Restarts    int32             `json:"restarts"`
	Created     string            `json:"created"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

func podInfo(p corev1.Pod) PodInfo {
	pi := PodInfo{Name: p.Name, Namespace: p.Namespace, Phase: string(p.Status.Phase), Reason: p.Status.Reason, Node: p.Spec.NodeName,
		Created: p.CreationTimestamp.Format(time.RFC3339), Annotations: p.Annotations, Labels: p.Labels}
	for _, c := range p.Spec.InitContainers {
		pi.Init = append(pi.Init, c.Name)
	}
	for _, c := range p.Spec.Containers {
		pi.Containers = append(pi.Containers, c.Name)
	}
	ready := 0
	for _, s := range p.Status.ContainerStatuses {
		if s.Ready {
			ready++
		}
		pi.Restarts += s.RestartCount
		if w := s.State.Waiting; w != nil && w.Reason != "" {
			pi.Reason = w.Reason
		} else if t := s.State.Terminated; t != nil && t.Reason != "" && pi.Reason == "" {
			pi.Reason = t.Reason
		}
	}
	pi.Ready = itoa(ready) + "/" + itoa(len(p.Spec.Containers))
	return pi
}

func itoa(i int) string { return strconv.Itoa(i) }

// Pods lists pods matching a label selector.
func (c *Client) Pods(ctx context.Context, ns, selector string) ([]PodInfo, error) {
	list, err := c.Core.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	out := make([]PodInfo, 0, len(list.Items))
	for _, p := range list.Items {
		out = append(out, podInfo(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created < out[j].Created })
	return out, nil
}

type LogOptions struct {
	Container string
	TailLines int64
	Follow    bool
	Previous  bool
}

// Logs streams a container's log lines until the stream ends or ctx is done.
func (c *Client) Logs(ctx context.Context, ns, pod string, o LogOptions, fn func(time, line string)) error {
	opts := &corev1.PodLogOptions{Container: o.Container, Follow: o.Follow, Previous: o.Previous, Timestamps: true}
	if o.TailLines > 0 {
		opts.TailLines = &o.TailLines
	}
	rc, err := c.Core.CoreV1().Pods(ns).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return err
	}
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 1<<16), 8<<20)
	for sc.Scan() {
		line := sc.Text()
		ts := ""
		if i := strings.IndexByte(line, ' '); i > 0 && i < 40 && strings.Contains(line[:i], "T") {
			ts, line = line[:i], line[i+1:]
		}
		fn(ts, line)
	}
	return sc.Err()
}

type Event struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Count     int32  `json:"count"`
	First     string `json:"first"`
	Last      string `json:"last"`
	Object    string `json:"object"`
	Component string `json:"component"`
}

// Events returns events for an object (name) in a namespace, newest first.
func (c *Client) Events(ctx context.Context, ns, kind, name string) ([]Event, error) {
	sel := fields.Set{"involvedObject.name": name}
	if kind != "" {
		sel["involvedObject.kind"] = kind
	}
	list, err := c.Core.CoreV1().Events(ns).List(ctx, metav1.ListOptions{FieldSelector: sel.String()})
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(list.Items))
	for _, e := range list.Items {
		last := e.LastTimestamp.Time
		if last.IsZero() {
			last = e.EventTime.Time
		}
		if last.IsZero() {
			last = e.CreationTimestamp.Time
		}
		first := e.FirstTimestamp.Time
		if first.IsZero() {
			first = last
		}
		cnt := e.Count
		if cnt == 0 {
			cnt = 1
		}
		out = append(out, Event{Type: e.Type, Reason: e.Reason, Message: e.Message, Count: cnt,
			First: first.Format(time.RFC3339), Last: last.Format(time.RFC3339),
			Object: e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name, Component: e.Source.Component})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last > out[j].Last })
	return out, nil
}
