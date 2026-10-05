// mockkube is a tiny fake Kubernetes API server with Argo Rollouts, Workflows and
// Events objects (plus pods, logs and events) for developing Syncscope without a
// real cluster. It implements just what client-go's dynamic and core clients use:
// list / watch / get / create / merge-patch / update / delete, pod logs, events.
//
//	go run ./cmd/mockkube -port 8097 -name prod -kubeconfig /tmp/mock.kubeconfig
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	mrand "math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type obj = map[string]any

var (
	port       = flag.Int("port", 8097, "listen port")
	name       = flag.String("name", "mock", "cluster name (also used in data)")
	kubeconfig = flag.String("kubeconfig", "", "write/merge a kubeconfig context for this server into this file")
	token      = "mock-token"

	mu   sync.Mutex
	rv   = 100
	db   = map[string]map[string]obj{} // resource -> ns/name -> object
	subs = map[string]map[chan obj]struct{}{}
	rnd  = mrand.New(mrand.NewSource(7))
)

var kindOf = map[string]string{
	"rollouts": "Rollout", "workflows": "Workflow", "workflowtemplates": "WorkflowTemplate", "cronworkflows": "CronWorkflow",
	"eventsources": "EventSource", "sensors": "Sensor", "eventbus": "EventBus", "pods": "Pod", "events": "Event",
}

func nextRV() string { rv++; return fmt.Sprint(rv) }

func put(res string, o obj) {
	md := o["metadata"].(obj)
	if db[res] == nil {
		db[res] = map[string]obj{}
	}
	if md["uid"] == nil {
		md["uid"] = fmt.Sprintf("%s-%d", res, rnd.Int63())
	}
	if md["creationTimestamp"] == nil {
		md["creationTimestamp"] = time.Now().Add(-time.Duration(rnd.Intn(72*60)) * time.Minute).UTC().Format(time.RFC3339)
	}
	md["resourceVersion"] = nextRV()
	if res != "pods" && res != "events" {
		o["apiVersion"] = "argoproj.io/v1alpha1"
	} else {
		o["apiVersion"] = "v1"
	}
	o["kind"] = kindOf[res]
	k := md["namespace"].(string) + "/" + md["name"].(string)
	typ := "MODIFIED"
	if _, ok := db[res][k]; !ok {
		typ = "ADDED"
	}
	db[res][k] = o
	notify(res, typ, o)
}

func notify(res, typ string, o obj) {
	b, _ := json.Marshal(o)
	var cp obj
	_ = json.Unmarshal(b, &cp)
	for ch := range subs[res] {
		select {
		case ch <- obj{"type": typ, "object": cp}:
		default:
		}
	}
}

func meta(ns, n string, labels obj) obj {
	if labels == nil {
		labels = obj{}
	}
	return obj{"name": n, "namespace": ns, "labels": labels, "annotations": obj{}}
}

func ts(ago time.Duration) string { return time.Now().Add(-ago).UTC().Format(time.RFC3339) }

// ---- seed data -------------------------------------------------------------------------

func pod(ns, n string, labels obj, ann obj, containers []string, phase, reason string, restarts int) {
	cs := []obj{}
	st := []obj{}
	for _, c := range containers {
		cs = append(cs, obj{"name": c, "image": "ghcr.io/acme/" + c + ":v1"})
		state := obj{"running": obj{"startedAt": ts(time.Hour)}}
		if reason != "" {
			state = obj{"waiting": obj{"reason": reason}}
		}
		st = append(st, obj{"name": c, "ready": reason == "" && phase == "Running", "restartCount": restarts, "state": state})
	}
	m := meta(ns, n, labels)
	if ann != nil {
		m["annotations"] = ann
	}
	put("pods", obj{"metadata": m, "spec": obj{"containers": cs, "nodeName": "gke-pool-1-a1b2"},
		"status": obj{"phase": phase, "containerStatuses": st}})
}

func seed() {
	// Rollouts
	apps := []string{"checkout", "payments", "search", "identity", "billing", "notifications"}
	for i := 0; i < 24; i++ {
		app := apps[i%len(apps)]
		ns := app
		n := fmt.Sprintf("%s-%s", app, []string{"api", "web", "worker", "gateway"}[i%4])
		steps := []obj{{"setWeight": 20}, {"pause": obj{}}, {"setWeight": 50}, {"pause": obj{"duration": "10m"}}, {"setWeight": 100}}
		r := obj{"metadata": meta(ns, n, obj{"app.kubernetes.io/instance": n + "-" + *name}),
			"spec": obj{"replicas": 4, "selector": obj{"matchLabels": obj{"app": n}},
				"template": obj{"metadata": obj{"labels": obj{"app": n}}, "spec": obj{"containers": []obj{{"name": "app", "image": "ghcr.io/acme/" + n + ":v2.4.0"}}}},
				"strategy": obj{"canary": obj{"steps": steps}}},
			"status": obj{"phase": "Healthy", "currentStepIndex": 5, "replicas": 4, "readyReplicas": 4, "updatedReplicas": 4, "availableReplicas": 4,
				"stableRS": "7d9f8b6c5d", "currentPodHash": "7d9f8b6c5d"}}
		r["metadata"].(obj)["annotations"] = obj{"rollout.argoproj.io/revision": fmt.Sprint(3 + i%5)}
		switch i % 8 {
		case 1:
			r["status"] = obj{"phase": "Paused", "message": "CanaryPauseStep", "currentStepIndex": 1, "replicas": 5, "readyReplicas": 5, "updatedReplicas": 1, "availableReplicas": 5,
				"stableRS": "7d9f8b6c5d", "currentPodHash": "5c6d7e8f9a", "pauseConditions": []obj{{"reason": "CanaryPauseStep", "startTime": ts(12 * time.Minute)}}}
		case 3:
			r["status"] = obj{"phase": "Degraded", "message": "RolloutAborted: Rollout aborted update to revision 7: Metric \"success-rate\" assessed Failed due to failed (3) > failureLimit (2)",
				"abort": true, "currentStepIndex": 0, "replicas": 4, "readyReplicas": 4, "updatedReplicas": 0, "availableReplicas": 4, "stableRS": "7d9f8b6c5d", "currentPodHash": "6b5a4c3d2e"}
		case 5:
			r["status"] = obj{"phase": "Progressing", "message": "more replicas need to be updated", "currentStepIndex": 2, "replicas": 5, "readyReplicas": 4, "updatedReplicas": 2, "availableReplicas": 4,
				"stableRS": "7d9f8b6c5d", "currentPodHash": "8a7b6c5d4e"}
		case 6:
			r["spec"].(obj)["strategy"] = obj{"blueGreen": obj{"activeService": n + "-active", "previewService": n + "-preview", "autoPromotionEnabled": false}}
			r["status"] = obj{"phase": "Paused", "message": "BlueGreenPause", "replicas": 8, "readyReplicas": 8, "updatedReplicas": 4, "availableReplicas": 8,
				"stableRS": "7d9f8b6c5d", "currentPodHash": "9f8e7d6c5b", "pauseConditions": []obj{{"reason": "BlueGreenPause", "startTime": ts(30 * time.Minute)}}}
		}
		put("rollouts", r)
		for p := 0; p < 2; p++ {
			pod(ns, fmt.Sprintf("%s-7d9f8b6c5d-%c%c%c%c%c", n, 'a'+p, 'k', 'x', '2', 'p'), obj{"app": n, "rollouts-pod-template-hash": "7d9f8b6c5d"}, nil, []string{"app"}, "Running", "", 0)
		}
	}

	// WorkflowTemplates & CronWorkflows
	tmpls := []string{"etl-daily", "ml-train", "db-backup", "report-export", "image-build", "data-quality", "cleanup", "sync-crm"}
	for _, t := range tmpls {
		put("workflowtemplates", obj{"metadata": meta("argo", t, nil), "spec": obj{"entrypoint": "main",
			"arguments": obj{"parameters": []obj{{"name": "date", "value": "today"}, {"name": "env", "value": "prod", "enum": []string{"prod", "staging"}}, {"name": "dry-run", "value": "false"}}},
			"templates": []obj{{"name": "main"}, {"name": "extract"}, {"name": "transform"}, {"name": "load"}}}})
	}
	for i, t := range []string{"etl-daily", "db-backup", "report-export", "cleanup", "sync-crm", "data-quality"} {
		c := obj{"metadata": meta("argo", t+"-cron", nil), "spec": obj{"schedule": []string{"0 2 * * *", "*/30 * * * *", "0 6 * * 1", "15 3 * * *", "*/5 * * * *", "0 * * * *"}[i],
			"timezone": "Europe/Zurich", "concurrencyPolicy": "Forbid", "suspend": i == 3,
			"workflowSpec": obj{"workflowTemplateRef": obj{"name": t}}},
			"status": obj{"lastScheduledTime": ts(time.Duration(i+1) * 25 * time.Minute), "succeeded": 40 + i, "failed": i % 3}}
		if i == 4 {
			c["status"].(obj)["conditions"] = []obj{{"type": "SubmissionError", "status": "True", "message": "Failed to submit Workflow: workflows.argoproj.io \"sync-crm\" not found"}}
		}
		put("cronworkflows", c)
	}

	// Workflows
	for i := 0; i < 60; i++ {
		t := tmpls[i%len(tmpls)]
		n := fmt.Sprintf("%s-%05d", t, 10000+i*37)
		labels := obj{"workflows.argoproj.io/workflow-template": t}
		if i%3 == 0 {
			labels["workflows.argoproj.io/cron-workflow"] = t + "-cron"
		}
		phase := []string{"Succeeded", "Succeeded", "Failed", "Running", "Succeeded", "Error", "Succeeded", "Running"}[i%8]
		put("workflows", workflowObj("argo", n, labels, t, phase, time.Duration(i)*17*time.Minute))
	}

	// Argo Events
	put("eventbus", obj{"metadata": meta("argo-events", "default", nil), "spec": obj{"jetstream": obj{"version": "2.10.10", "replicas": 3}},
		"status": obj{"phase": "Running", "conditions": []obj{{"type": "Configured", "status": "True"}, {"type": "Deployed", "status": "True"}}}})
	ok := []obj{{"type": "SourcesProvided", "status": "True"}, {"type": "Deployed", "status": "True"}}
	sources := []struct {
		n   string
		typ string
		evs obj
		bad bool
	}{
		{"webhook", "webhook", obj{"deploy": obj{"port": "12000", "endpoint": "/deploy"}, "rollback": obj{"port": "12000", "endpoint": "/rollback"}}, false},
		{"github", "github", obj{"push": obj{"repositories": []obj{{"owner": "acme", "names": []string{"deploy"}}}}}, false},
		{"kafka", "kafka", obj{"orders": obj{"url": "kafka:9092", "topic": "orders"}}, true},
		{"calendar", "calendar", obj{"nightly": obj{"schedule": "0 1 * * *"}}, false},
	}
	for _, s := range sources {
		conds := ok
		if s.bad {
			conds = []obj{{"type": "SourcesProvided", "status": "True"}, {"type": "Deployed", "status": "False", "reason": "DeploymentFailed",
				"message": "failed to connect to kafka:9092: dial tcp 10.0.4.2:9092: i/o timeout"}}
		}
		put("eventsources", obj{"metadata": meta("argo-events", s.n, nil), "spec": obj{s.typ: s.evs}, "status": obj{"conditions": conds}})
		reason := ""
		if s.bad {
			reason = "CrashLoopBackOff"
		}
		pod("argo-events", s.n+"-eventsource-6f7c9-x2k9p", obj{"eventsource-name": s.n}, nil, []string{"main"}, "Running", reason, map[bool]int{true: 14}[s.bad])
	}
	sensors := []struct {
		n    string
		deps []obj
		trig []obj
		bad  bool
	}{
		{"deploy-sensor", []obj{{"name": "dep-deploy", "eventSourceName": "webhook", "eventName": "deploy"}},
			[]obj{{"template": obj{"name": "run-image-build", "argoWorkflow": obj{"operation": "submit"}}}}, false},
		{"github-sensor", []obj{{"name": "push", "eventSourceName": "github", "eventName": "push"}},
			[]obj{{"template": obj{"name": "trigger-ci", "argoWorkflow": obj{"operation": "submit"}}}, {"template": obj{"name": "notify-slack", "slack": obj{"channel": "#deploys"}}}}, false},
		{"orders-sensor", []obj{{"name": "orders", "eventSourceName": "kafka", "eventName": "orders"}},
			[]obj{{"template": obj{"name": "sync-crm", "http": obj{"url": "http://crm/api/sync"}}}}, true},
		{"nightly-sensor", []obj{{"name": "tick", "eventSourceName": "calendar", "eventName": "nightly"}, {"name": "rollback", "eventSourceName": "webhook", "eventName": "rollback"}},
			[]obj{{"template": obj{"name": "nightly-etl", "argoWorkflow": obj{"operation": "submit"}, "conditions": "tick"}}, {"template": obj{"name": "k8s-scale", "k8s": obj{"operation": "patch"}}}}, false},
	}
	for _, s := range sensors {
		conds := []obj{{"type": "DependenciesProvided", "status": "True"}, {"type": "TriggersProvided", "status": "True"}, {"type": "Deployed", "status": "True"}}
		if s.bad {
			conds[2] = obj{"type": "Deployed", "status": "False", "reason": "Unhealthy", "message": "sensor pod is crashing: dependency \"orders\" never receives events (event source kafka not running)"}
		}
		put("sensors", obj{"metadata": meta("argo-events", s.n, nil), "spec": obj{"dependencies": s.deps, "triggers": s.trig}, "status": obj{"conditions": conds}})
		pod("argo-events", s.n+"-sensor-5d8b7-q8w7e", obj{"sensor-name": s.n}, nil, []string{"main"}, "Running", "", 0)
	}
}

// workflowObj builds a DAG workflow: main -> extract -> (transform-a, transform-b) -> load
func workflowObj(ns, n string, labels obj, tmpl, phase string, ago time.Duration) obj {
	start := time.Now().Add(-ago - 10*time.Minute)
	type nd struct {
		id, display, tmplName, typ string
		children                   []string
	}
	ids := func(s string) string { return n + "-" + s }
	nodes := []nd{
		{ids("root"), n, "main", "DAG", []string{ids("extract")}},
		{ids("extract"), "extract", "extract", "Pod", []string{ids("ta"), ids("tb")}},
		{ids("ta"), "transform-orders", "transform", "Pod", []string{ids("load")}},
		{ids("tb"), "transform-customers", "transform", "Pod", []string{ids("load")}},
		{ids("load"), "load", "load", "Pod", nil},
	}
	status := obj{}
	done := 0
	for i, x := range nodes {
		ph := "Succeeded"
		msg := ""
		switch phase {
		case "Running":
			if i == 2 || i == 3 {
				ph = "Running"
			} else if i == 4 {
				ph = "Pending"
			}
		case "Failed":
			if i == 3 {
				ph, msg = "Failed", "Error (exit code 1): psycopg2.OperationalError: connection to server at \"10.0.3.4\", port 5432 failed: Connection refused"
			} else if i == 4 {
				ph, msg = "Omitted", "omitted: depends condition not met"
			} else if i == 0 {
				ph = "Failed"
			}
		case "Error":
			if i == 1 {
				ph, msg = "Error", "pod deleted during operation"
			} else if i > 1 {
				ph, msg = "Omitted", ""
			} else {
				ph = "Error"
			}
		}
		if ph == "Succeeded" && x.typ == "Pod" {
			done++
		}
		node := obj{"id": x.id, "name": n + "." + x.display, "displayName": x.display, "type": x.typ, "templateName": x.tmplName, "phase": ph,
			"startedAt": start.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC3339)}
		if ph != "Running" && ph != "Pending" {
			node["finishedAt"] = start.Add(time.Duration(i+1) * time.Minute).UTC().Format(time.RFC3339)
		}
		if msg != "" {
			node["message"] = msg
		}
		if len(x.children) > 0 {
			node["children"] = x.children
		}
		if x.typ == "Pod" {
			node["inputs"] = obj{"parameters": []obj{{"name": "date", "value": "2026-10-05"}}}
			if ph == "Succeeded" {
				node["outputs"] = obj{"parameters": []obj{{"name": "rows", "value": fmt.Sprint(1000 + rnd.Intn(9000))}}, "exitCode": "0"}
			}
			podPhase := map[string]string{"Succeeded": "Succeeded", "Failed": "Failed", "Running": "Running", "Error": "Failed"}[ph]
			if podPhase != "" {
				pod(ns, x.id, obj{"workflows.argoproj.io/workflow": n}, obj{"workflows.argoproj.io/node-id": x.id, "workflows.argoproj.io/node-name": n + "." + x.display},
					[]string{"wait", "main"}, podPhase, "", 0)
			}
		}
		status[x.id] = node
	}
	wf := obj{"metadata": meta(ns, n, labels), "spec": obj{"entrypoint": "main", "workflowTemplateRef": obj{"name": tmpl},
		"arguments": obj{"parameters": []obj{{"name": "date", "value": "2026-10-05"}}}},
		"status": obj{"phase": phase, "startedAt": start.UTC().Format(time.RFC3339), "progress": fmt.Sprintf("%d/4", done), "nodes": status}}
	if phase != "Running" {
		wf["status"].(obj)["finishedAt"] = start.Add(6 * time.Minute).UTC().Format(time.RFC3339)
	}
	if phase == "Failed" {
		wf["status"].(obj)["message"] = "child '" + n + "-tb' failed"
	}
	if phase == "Error" {
		wf["status"].(obj)["message"] = "pod deleted during operation"
	}
	return wf
}

// ---- JSON merge patch (RFC 7386) ----------------------------------------------------

func merge(dst obj, patch obj) {
	for k, v := range patch {
		if v == nil {
			delete(dst, k)
			continue
		}
		if pm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				merge(dm, pm)
				continue
			}
			nm := obj{}
			merge(nm, pm)
			dst[k] = nm
			continue
		}
		dst[k] = v
	}
}

// ---- HTTP -------------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func status(w http.ResponseWriter, code int, reason, msg string) {
	writeJSON(w, code, obj{"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": reason, "message": msg, "code": code})
}

func matchLabels(o obj, selector string) bool {
	if selector == "" {
		return true
	}
	labels, _ := o["metadata"].(obj)["labels"].(obj)
	for _, part := range strings.Split(selector, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 || fmt.Sprint(labels[kv[0]]) != kv[1] {
			return false
		}
	}
	return true
}

func list(res, ns, selector string) []obj {
	var out []obj
	for k, o := range db[res] {
		if ns != "" && !strings.HasPrefix(k, ns+"/") {
			continue
		}
		if matchLabels(o, selector) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["metadata"].(obj)["name"].(string) < out[j]["metadata"].(obj)["name"].(string)
	})
	return out
}

func handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+token {
		status(w, 401, "Unauthorized", "Unauthorized")
		return
	}
	p := strings.Trim(r.URL.Path, "/")
	if p == "version" {
		writeJSON(w, 200, obj{"major": "1", "minor": "31", "gitVersion": "v1.31.4-mock-" + *name})
		return
	}
	parts := strings.Split(p, "/")
	// /apis/argoproj.io/v1alpha1/... or /api/v1/...
	var rest []string
	switch {
	case len(parts) >= 3 && parts[0] == "apis" && parts[1] == "argoproj.io":
		rest = parts[3:]
	case len(parts) >= 2 && parts[0] == "api" && parts[1] == "v1":
		rest = parts[2:]
	default:
		status(w, 404, "NotFound", "the server could not find the requested resource")
		return
	}
	ns := ""
	if len(rest) >= 2 && rest[0] == "namespaces" {
		ns, rest = rest[1], rest[2:]
	}
	if len(rest) == 0 {
		status(w, 404, "NotFound", "not found")
		return
	}
	res := rest[0]
	if _, known := kindOf[res]; !known {
		status(w, 404, "NotFound", "the server could not find the requested resource ("+res+")")
		return
	}
	q := r.URL.Query()
	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			if q.Get("watch") == "true" || q.Get("watch") == "1" {
				watch(w, r, res, ns)
				return
			}
			mu.Lock()
			items := list(res, ns, q.Get("labelSelector"))
			if res == "events" {
				items = filterEvents(items, q.Get("fieldSelector"))
			}
			v := fmt.Sprint(rv)
			mu.Unlock()
			if items == nil {
				items = []obj{}
			}
			api := "argoproj.io/v1alpha1"
			if res == "pods" || res == "events" {
				api = "v1"
			}
			writeJSON(w, 200, obj{"apiVersion": api, "kind": kindOf[res] + "List", "metadata": obj{"resourceVersion": v}, "items": items})
		case http.MethodPost:
			var o obj
			_ = json.NewDecoder(r.Body).Decode(&o)
			md, _ := o["metadata"].(map[string]any)
			if md == nil {
				status(w, 400, "BadRequest", "metadata required")
				return
			}
			if md["name"] == nil {
				md["name"] = fmt.Sprint(md["generateName"]) + randSuffix()
			}
			md["namespace"] = ns
			mu.Lock()
			if res == "workflows" {
				spec, _ := o["spec"].(map[string]any)
				tmpl := ""
				if ref, ok := spec["workflowTemplateRef"].(map[string]any); ok {
					tmpl, _ = ref["name"].(string)
				}
				labels, _ := md["labels"].(map[string]any)
				wf := workflowObj(ns, md["name"].(string), labels, tmpl, "Running", -10*time.Minute)
				wf["spec"] = spec
				o = wf
				go finishLater(ns, md["name"].(string))
			}
			put(res, o)
			mu.Unlock()
			writeJSON(w, 201, o)
		default:
			status(w, 405, "MethodNotAllowed", "method not allowed")
		}
		return
	}
	n := rest[1]
	sub := ""
	if len(rest) >= 3 {
		sub = rest[2]
	}
	if res == "pods" && sub == "log" {
		podLog(w, r, ns, n)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	o, ok := db[res][ns+"/"+n]
	if !ok {
		status(w, 404, "NotFound", fmt.Sprintf("%s %q not found", res, n))
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, o)
	case http.MethodPatch:
		var patch obj
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			status(w, 400, "BadRequest", err.Error())
			return
		}
		if sub != "status" {
			delete(patch, "status") // like a real status subresource split
		}
		merge(o, patch)
		reconcile(res, o)
		put(res, o)
		writeJSON(w, 200, o)
	case http.MethodPut:
		var nu obj
		_ = json.NewDecoder(r.Body).Decode(&nu)
		if nu["metadata"].(map[string]any)["resourceVersion"] != o["metadata"].(obj)["resourceVersion"] {
			status(w, 409, "Conflict", "the object has been modified; please apply your changes to the latest version and try again")
			return
		}
		put(res, nu)
		writeJSON(w, 200, nu)
	case http.MethodDelete:
		delete(db[res], ns+"/"+n)
		notify(res, "DELETED", o)
		writeJSON(w, 200, obj{"kind": "Status", "status": "Success"})
	}
}

// reconcile emulates the controllers reacting to patches.
func reconcile(res string, o obj) {
	spec, _ := o["spec"].(obj)
	st, _ := o["status"].(obj)
	if st == nil {
		st = obj{}
		o["status"] = st
	}
	switch res {
	case "workflows":
		if sd, _ := spec["shutdown"].(string); sd != "" && st["phase"] == "Running" {
			st["phase"], st["message"] = "Failed", "Stopped with strategy '"+sd+"'"
			st["finishedAt"] = time.Now().UTC().Format(time.RFC3339)
		}
	case "rollouts":
		if st["abort"] == true {
			st["phase"], st["message"] = "Degraded", "RolloutAborted: aborted by user"
		} else if st["phase"] == "Degraded" && st["abort"] == false {
			st["phase"], st["message"] = "Progressing", "retrying rollout"
		}
		if st["pauseConditions"] == nil && spec["paused"] != true && st["phase"] == "Paused" {
			st["phase"], st["message"] = "Progressing", "promoted"
		}
		if st["promoteFull"] == true {
			st["phase"], st["message"], st["currentStepIndex"] = "Healthy", "", float64(5)
			delete(st, "promoteFull")
		}
		if spec["paused"] == true {
			st["phase"], st["message"] = "Paused", "manually paused"
		}
	}
}

func finishLater(ns, n string) {
	time.Sleep(6 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	o, ok := db["workflows"][ns+"/"+n]
	if !ok {
		return
	}
	st := o["status"].(obj)
	if st["phase"] != "Running" {
		return
	}
	st["phase"], st["progress"], st["finishedAt"] = "Succeeded", "4/4", time.Now().UTC().Format(time.RFC3339)
	for _, nd := range st["nodes"].(obj) {
		m := nd.(obj)
		m["phase"] = "Succeeded"
		m["finishedAt"] = st["finishedAt"]
	}
	put("workflows", o)
}

func randSuffix() string {
	const a = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 5)
	for i := range b {
		b[i] = a[rnd.Intn(len(a))]
	}
	return string(b)
}

func filterEvents(items []obj, fs string) []obj {
	want := map[string]string{}
	for _, p := range strings.Split(fs, ",") {
		if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
			want[kv[0]] = kv[1]
		}
	}
	var out []obj
	for _, e := range items {
		io := e["involvedObject"].(obj)
		if (want["involvedObject.name"] == "" || io["name"] == want["involvedObject.name"]) &&
			(want["involvedObject.kind"] == "" || io["kind"] == want["involvedObject.kind"]) {
			out = append(out, e)
		}
	}
	return out
}

func watch(w http.ResponseWriter, r *http.Request, res, ns string) {
	fl, _ := w.(http.Flusher)
	ch := make(chan obj, 256)
	mu.Lock()
	if subs[res] == nil {
		subs[res] = map[chan obj]struct{}{}
	}
	subs[res][ch] = struct{}{}
	mu.Unlock()
	defer func() { mu.Lock(); delete(subs[res], ch); mu.Unlock() }()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	fl.Flush()
	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			if ns != "" && ev["object"].(obj)["metadata"].(obj)["namespace"] != ns {
				continue
			}
			_ = enc.Encode(ev)
			fl.Flush()
		}
	}
}

func podLog(w http.ResponseWriter, r *http.Request, ns, pod string) {
	fl, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/plain")
	q := r.URL.Query()
	bad := strings.Contains(pod, "kafka") || strings.Contains(pod, "-tb")
	line := func(i int, t time.Time) string {
		msg := fmt.Sprintf(`{"level":"info","msg":"processed batch","batch":%d,"container":%q}`, i, q.Get("container"))
		if bad && i%5 == 4 {
			msg = `{"level":"error","msg":"connection refused","error":"dial tcp 10.0.3.4:5432: connect: connection refused"}`
		}
		if q.Get("timestamps") == "true" {
			return t.UTC().Format(time.RFC3339Nano) + " " + msg
		}
		return msg
	}
	for i := 0; i < 120; i++ {
		fmt.Fprintln(w, line(i, time.Now().Add(time.Duration(i-120)*time.Second)))
	}
	fl.Flush()
	if q.Get("follow") != "true" {
		return
	}
	for i := 120; ; i++ {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(800 * time.Millisecond):
			fmt.Fprintln(w, line(i, time.Now()))
			fl.Flush()
		}
	}
}

func seedEvents() {
	add := func(ns, kind, n, typ, reason, msg string, count int, ago time.Duration) {
		put("events", obj{"metadata": meta(ns, fmt.Sprintf("%s.%x", n, rnd.Int63()), nil), "type": typ, "reason": reason, "message": msg, "count": count,
			"firstTimestamp": ts(ago * 3), "lastTimestamp": ts(ago), "involvedObject": obj{"kind": kind, "name": n, "namespace": ns}, "source": obj{"component": "controller"}})
	}
	for _, o := range list("rollouts", "", "") {
		md := o["metadata"].(obj)
		add(md["namespace"].(string), "Rollout", md["name"].(string), "Normal", "RolloutUpdated", "Rollout updated to revision 7", 1, 20*time.Minute)
		if st := o["status"].(obj); st["phase"] == "Degraded" {
			add(md["namespace"].(string), "Rollout", md["name"].(string), "Warning", "RolloutAborted", fmt.Sprint(st["message"]), 1, 5*time.Minute)
		}
	}
	add("argo-events", "Sensor", "orders-sensor", "Warning", "SensorUnhealthy", "dependency orders has no running event source", 22, time.Minute)
	add("argo-events", "EventSource", "kafka", "Warning", "BackOff", "Back-off restarting failed container main", 14, 2*time.Minute)
}

func writeKubeconfig(path string) {
	type ctx struct{ name, server string }
	// merge with existing contexts written by other mock instances
	existing := map[string]string{}
	if b, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "# mock ") {
				f := strings.Fields(l)
				if len(f) == 4 {
					existing[f[2]] = f[3]
				}
			}
		}
	}
	existing["mock-"+*name] = fmt.Sprintf("https://127.0.0.1:%d", *port)
	var names []string
	for n := range existing {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "# mock %s %s\n", n, existing[n])
	}
	b.WriteString("apiVersion: v1\nkind: Config\nclusters:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- name: %s\n  cluster:\n    server: %s\n    insecure-skip-tls-verify: true\n", n, existing[n])
	}
	b.WriteString("users:\n- name: mock-user\n  user:\n    token: " + token + "\ncontexts:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- name: %s\n  context:\n    cluster: %s\n    user: mock-user\n    namespace: argo\n", n, n)
	}
	fmt.Fprintf(&b, "current-context: %s\n", names[0])
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		log.Fatal(err)
	}
}

func main() {
	flag.Parse()
	mu.Lock()
	seed()
	seedEvents()
	mu.Unlock()
	if *kubeconfig != "" {
		writeKubeconfig(*kubeconfig)
	}
	// running workflows advance over time
	go func() {
		for range time.Tick(5 * time.Second) {
			mu.Lock()
			var running []string
			for k, o := range db["workflows"] {
				if st := o["status"].(obj); st["phase"] == "Running" && rnd.Intn(6) == 0 {
					running = append(running, k)
				}
			}
			mu.Unlock()
			for _, k := range running {
				ns, n, _ := strings.Cut(k, "/")
				finishLaterNow(ns, n)
			}
		}
	}()
	// client-go only sends credentials over TLS, so serve HTTPS with a throwaway cert
	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", *port), Handler: http.HandlerFunc(handle),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{selfSigned()}}}
	log.Printf("mock kube %q on https://127.0.0.1:%d (token %s)", *name, *port, token)
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

func finishLaterNow(ns, n string) {
	mu.Lock()
	defer mu.Unlock()
	o, ok := db["workflows"][ns+"/"+n]
	if !ok {
		return
	}
	st := o["status"].(obj)
	st["phase"], st["progress"], st["finishedAt"] = "Succeeded", "4/4", time.Now().UTC().Format(time.RFC3339)
	for _, nd := range st["nodes"].(obj) {
		nd.(obj)["phase"] = "Succeeded"
	}
	put("workflows", o)
}

func selfSigned() tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "mockkube"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		DNSNames: []string{"localhost"}, IPAddresses: nil, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
