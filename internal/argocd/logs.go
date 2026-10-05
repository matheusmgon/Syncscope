package argocd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// LogQuery selects the logs to read: a single pod (PodName) or every pod of a
// workload (Group/Kind/ResourceName), like the Argo CD UI does.
type LogQuery struct {
	Namespace    string
	PodName      string
	Group        string
	Kind         string
	ResourceName string
	Container    string
	TailLines    int64
	SinceSeconds int64
	Follow       bool
	Previous     bool
}

type LogEntry struct {
	Content   string `json:"content"`
	TimeStamp string `json:"timeStamp"`
	PodName   string `json:"podName"`
	Last      bool   `json:"last"`
}

// Logs streams log entries until the server ends the stream (or ctx is done).
func (c *Client) Logs(ctx context.Context, app, appNs, project string, q LogQuery, fn func(LogEntry)) error {
	v := url.Values{"namespace": {q.Namespace}, "follow": {strconv.FormatBool(q.Follow)}, "previous": {strconv.FormatBool(q.Previous)}}
	if appNs != "" {
		v.Set("appNamespace", appNs)
	}
	if project != "" {
		v.Set("project", project)
	}
	if q.PodName != "" {
		v.Set("podName", q.PodName)
	} else {
		v.Set("group", q.Group)
		v.Set("kind", q.Kind)
		v.Set("resourceName", q.ResourceName)
	}
	if q.Container != "" {
		v.Set("container", q.Container)
	}
	if q.TailLines > 0 {
		v.Set("tailLines", strconv.FormatInt(q.TailLines, 10))
	}
	if q.SinceSeconds > 0 {
		v.Set("sinceSeconds", strconv.FormatInt(q.SinceSeconds, 10))
	}
	resp, err := c.do(ctx, c.stream, http.MethodGet, "/api/v1/applications/"+url.PathEscape(app)+"/logs", v, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 1<<16), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(sc.Bytes()), []byte("data:")))
		if len(line) == 0 {
			continue
		}
		var msg struct {
			Result *LogEntry `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(line, &msg) != nil {
			continue
		}
		if msg.Error != nil {
			return errors.New(msg.Error.Message)
		}
		if msg.Result != nil {
			if msg.Result.Last {
				return nil
			}
			fn(*msg.Result)
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return io.EOF
}

// ResourceManifest returns the live manifest of a managed (or child) resource.
func (c *Client) ResourceManifest(ctx context.Context, app, appNs, project string, r ResourceAction) (map[string]any, error) {
	v := url.Values{"namespace": {r.Namespace}, "resourceName": {r.Name}, "version": {r.Version}, "group": {r.Group}, "kind": {r.Kind}}
	if appNs != "" {
		v.Set("appNamespace", appNs)
	}
	if project != "" {
		v.Set("project", project)
	}
	var out struct {
		Manifest string `json:"manifest"`
	}
	if err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(app)+"/resource", v, &out); err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out.Manifest), &m); err != nil {
		return nil, err
	}
	return m, nil
}
