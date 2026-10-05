package argocd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// TerminalRequest identifies the container to exec into.
type TerminalRequest struct {
	App, AppNamespace, Project string
	Namespace, Pod, Container  string
}

// Terminal is an interactive shell in a pod through the Argo CD web terminal
// (requires exec.enabled in argocd-cm and the "exec, create" RBAC permission).
type Terminal struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

type termMsg struct {
	Operation string `json:"operation"`
	Data      string `json:"data"`
	Rows      uint16 `json:"rows"`
	Cols      uint16 `json:"cols"`
}

func (c *Client) OpenTerminal(ctx context.Context, r TerminalRequest) (*Terminal, error) {
	c.refreshIfExpiring(ctx)
	u := *c.base
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(c.base.Path, "/") + "/terminal"
	q := url.Values{"pod": {r.Pod}, "container": {r.Container}, "appName": {r.App}, "projectName": {r.Project}, "namespace": {r.Namespace}}
	if r.AppNamespace != "" {
		q.Set("appNamespace", r.AppNamespace)
	}
	u.RawQuery = q.Encode()

	var tlsCfg *tls.Config
	if tr, ok := c.http.Transport.(*http.Transport); ok {
		tlsCfg = tr.TLSClientConfig
	}
	d := websocket.Dialer{TLSClientConfig: tlsCfg, Proxy: http.ProxyFromEnvironment}
	h := http.Header{}
	for k, v := range c.opts.Headers {
		h.Set(k, v)
	}
	tok := c.Credentials().Token
	h.Set("Authorization", "Bearer "+tok)
	h.Set("Cookie", "argocd.token="+tok)
	h.Set("Origin", c.base.Scheme+"://"+c.base.Host)
	conn, resp, err := d.DialContext(ctx, u.String(), h)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("terminal refused (%s): exec must be enabled in argocd-cm (exec.enabled: \"true\") and your role needs the 'exec, create' permission", resp.Status)
			}
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
				e := readAPIError(resp)
				return nil, fmt.Errorf("terminal not available: %v", e)
			}
			return nil, fmt.Errorf("terminal: %s", resp.Status)
		}
		return nil, err
	}
	return &Terminal{conn: conn}, nil
}

func (t *Terminal) write(m termMsg) error {
	b, _ := json.Marshal(m)
	t.wmu.Lock()
	defer t.wmu.Unlock()
	return t.conn.WriteMessage(websocket.TextMessage, b)
}

func (t *Terminal) Input(data string) error { return t.write(termMsg{Operation: "stdin", Data: data}) }

func (t *Terminal) Resize(rows, cols uint16) error {
	return t.write(termMsg{Operation: "resize", Rows: rows, Cols: cols})
}

// Read blocks and calls fn with every chunk of output until the session ends.
func (t *Terminal) Read(fn func(string)) error {
	for {
		_, b, err := t.conn.ReadMessage()
		if err != nil {
			return err
		}
		var m termMsg
		if json.Unmarshal(b, &m) == nil && m.Operation != "" {
			if m.Operation == "stdout" {
				fn(m.Data)
			}
			continue
		}
		fn(string(b))
	}
}

func (t *Terminal) Close() error { return t.conn.Close() }
