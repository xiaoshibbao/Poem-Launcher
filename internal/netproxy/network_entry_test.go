//go:build windows

package netproxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeNetworkEntryRedirectsSystemProxy(t *testing.T) {
	helper := os.Getenv("POEM_NATIVE_NETWORK_FIXTURE")
	if helper == "" {
		t.Skip("native fixture not configured")
	}
	module, err := filepath.Abs("../../assets/NetworkEntry.dll")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	var wrong atomic.Int32
	clash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wrong.Add(1); io.WriteString(w, "official-via-clash") }))
	defer clash.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "poem-private-server") }))
	defer server.Close()
	p, err := New(server.URL, PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	port, err := p.Start(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	oldPort := clash.Listener.Addr().(*net.TCPAddr).Port
	// Four actual Winsock paths: IAT connect, dynamically resolved connect,
	// IAT WSAConnect, and dynamically resolved WSAConnect.
	cmd := exec.Command(helper, module, strconv.Itoa(oldPort), "poem-private-server")
	cmd.Env = append(os.Environ(), fmt.Sprintf("POEM_SYSTEM_PROXY_PORT=%d", oldPort), fmt.Sprintf("POEM_HTTP_PROXY_PORT=%d", port))
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "redirected=4") {
		t.Fatalf("native redirect: %v %s", err, output)
	}
	if wrong.Load() != 0 {
		t.Fatalf("requests reached wrong system proxy: %d", wrong.Load())
	}
	// Only the configured system-proxy port is redirected. Another port remains
	// untouched and therefore returns the simulated original upstream response.
	cmd = exec.Command(helper, module, strconv.Itoa(oldPort), "official-via-clash")
	cmd.Env = append(os.Environ(), "POEM_SYSTEM_PROXY_PORT=1", fmt.Sprintf("POEM_HTTP_PROXY_PORT=%d", port))
	output, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "redirected=0") {
		t.Fatalf("unrelated port was redirected: %v %s", err, output)
	}
	if wrong.Load() != 4 {
		t.Fatal("unrelated traffic behavior changed", wrong.Load())
	}
}
