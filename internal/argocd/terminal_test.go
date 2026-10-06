package argocd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// Argo CD servers usually speak HTTP/2 over TLS. The terminal websocket must
// still negotiate HTTP/1.1, even though the REST client shares the TLS config.
func TestTerminalOverHTTP2Server(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/terminal" {
			http.NotFound(w, r)
			return
		}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteJSON(map[string]string{"operation": "stdout", "data": "hello"})
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	cl, err := NewClient(Options{Server: srv.URL, Insecure: true}, Credentials{Token: "t"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// make the REST transport negotiate h2 first, as it does in the app
	if _, err := cl.http.Get(srv.URL + "/api/version"); err != nil {
		t.Fatal(err)
	}
	term, err := cl.OpenTerminal(context.Background(), TerminalRequest{App: "a", Namespace: "ns", Pod: "p", Container: "c"})
	if err != nil {
		t.Fatalf("open terminal: %v", err)
	}
	defer term.Close()
	var got strings.Builder
	_ = term.Read(func(s string) { got.WriteString(s) })
	if got.String() != "hello" {
		t.Fatalf("got %q", got.String())
	}
}
