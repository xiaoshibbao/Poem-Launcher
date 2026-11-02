//go:build windows

package injector

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestThreadTraceDetectsLostAndReusedNodes(t *testing.T) {
	memory := map[uint64][]byte{}
	node := func(address, next uint64, id, state uint32) {
		b := make([]byte, 0xc0)
		binary.LittleEndian.PutUint32(b[0x38:], state)
		binary.LittleEndian.PutUint64(b[0x58:], next)
		binary.LittleEndian.PutUint64(b[0xb8:], uint64(id))
		memory[address] = b
	}
	read := func(address uint64, b []byte) error {
		data, ok := memory[address]
		if !ok {
			return fmt.Errorf("not mapped")
		}
		copy(b, data)
		return nil
	}
	node(0x1000, 0x2000, 10, 1)
	node(0x2000, 0, 20, 9)
	nodes, err := readThreadNodes(read, 0x1000)
	if err != nil || len(nodes) != 2 || nodes[1].id != 20 {
		t.Fatalf("normal list: %v %v", nodes, err)
	}
	node(0x2000, 0x3f8000000004000f, 0, 0x3013d)
	nodes, err = readThreadNodes(read, 0x1000)
	if err == nil || len(nodes) != 1 {
		t.Fatalf("reused node not detected: %v %v", nodes, err)
	}
	node(0x2000, 0x1000, 20, 1)
	if _, err = readThreadNodes(read, 0x1000); err == nil {
		t.Fatal("cycle not detected")
	}
	delete(memory, 0x2000)
	if _, err = readThreadNodes(read, 0x1000); err == nil {
		t.Fatal("unreadable node not detected")
	}
}

func TestProxyThreadTraceOwnProcess(t *testing.T) {
	helper := os.Getenv("POEM_SMOKE_HELPER")
	if helper == "" {
		t.Skip("set POEM_SMOKE_HELPER")
	}
	source := filepath.Join(filepath.Dir(filepath.Dir(helper)), "assets", "ProxyHook.dll")
	module := filepath.Join(t.TempDir(), "Poem.Proxy.dll")
	bytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(module, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "report")
	p, err := LaunchWithDLLs(helper, []string{module}, map[string]string{"POEM_SMOKE_STAGE": "1", "POEM_SMOKE_OUTPUT": report, "POEM_PROXY_NAME": "Poem.Proxy.dll"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	logPath := filepath.Join(t.TempDir(), "trace.log")
	stop, err := StartProxyThreadTrace(p, logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	time.Sleep(800 * time.Millisecond)
	if err = os.WriteFile(report+".release", []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	p.Wait()
	stop()
	log, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(log), "trace start") {
		t.Fatalf("trace failed: %v %s", err, log)
	}
}
