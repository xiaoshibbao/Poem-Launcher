//go:build windows

package netproxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBoundServerSocket(t *testing.T) {
	t.Chdir(t.TempDir())
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	index := 0
	for _, i := range interfaces {
		if i.Flags&net.FlagLoopback != 0 {
			index = i.Index
			break
		}
	}
	if index == 0 {
		t.Fatal("loopback interface missing")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "bound-ok") }))
	defer server.Close()
	p, err := New(server.URL, PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	var logged string
	if err = p.BindServerInterface(uint32(index), func(format string, args ...any) { logged = fmt.Sprintf(format, args...) }); err != nil {
		t.Fatal(err)
	}
	resp, err := p.client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "bound-ok" || !strings.Contains(logged, "server socket connected") {
		t.Fatalf("body=%s log=%s err=%v", body, logged, err)
	}
}
