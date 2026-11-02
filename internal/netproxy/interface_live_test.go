//go:build windows

package netproxy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestLiveBoundServer(t *testing.T) {
	index, err := strconv.Atoi(os.Getenv("POEM_LIVE_INTERFACE"))
	if err != nil {
		t.Skip("optional live check")
	}
	t.Chdir(t.TempDir())
	p, err := New("http://43.142.131.90:21000/", PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	var record string
	err = p.BindServerInterface(uint32(index), func(format string, args ...any) { record = fmt.Sprintf(format, args...); t.Log(record) })
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.client.Get("http://43.142.131.90:21000/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	t.Logf("server HTTP status=%d", response.StatusCode)
	if !strings.Contains(record, "server socket connected") || strings.Contains(record, "local=198.18.") {
		t.Fatal("expected physical source address")
	}

}
