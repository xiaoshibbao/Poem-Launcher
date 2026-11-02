//go:build windows

package launcher

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyThreadNotificationPatch(t *testing.T) {
	source, err := os.ReadFile("../../assets/ProxyHook.dll")
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(source)
	patched, err := configureProxyThreadNotifications(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, original) || len(source) != len(patched) {
		t.Fatal("source or layout changed")
	}
	for i := range source {
		if source[i] != patched[i] && !(i >= 23632 && i < 23680 || i >= 1837344 && i < 1837352 || i >= 58865 && i < 58868 || i >= 59240 && i < 59244 || i >= 59332 && i < 59335) {
			t.Fatalf("unexpected edit %x", i)
		}
	}
	if _, err := configureHookRSA(patched, ""); err != nil {
		t.Fatalf("RSA compatibility: %v", err)
	}
	source[0] ^= 1
	if _, err := configureProxyThreadNotifications(source); err == nil {
		t.Fatal("unknown module accepted")
	}
}

func TestProxyNativeThreadNotifications(t *testing.T) {
	helper := os.Getenv("POEM_THREAD_EXIT_FIXTURE")
	if helper == "" {
		t.Skip("native fixture not configured")
	}
	sourcePath, _ := filepath.Abs("../../assets/ProxyHook.dll")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	patched, err := configureProxyThreadNotifications(source)
	if err != nil {
		t.Fatal(err)
	}
	patchedPath := filepath.Join(t.TempDir(), "Poem.Proxy.dll")
	if err := os.WriteFile(patchedPath, patched, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{sourcePath, patchedPath} {
		cmd := exec.Command(helper, path)
		cmd.Env = append(os.Environ(), "POEM_DLLMAIN_OBSERVE=1")
		if path == patchedPath {
			cmd.Env = append(cmd.Env, "POEM_CHECK_GUARD_UNWIND=1")
		}
		out, err := cmd.CombinedOutput()
		t.Logf("%s: %s", path, out)
		if err != nil {
			t.Fatalf("fixture: %v %s", err, out)
		}
		if path == patchedPath && !strings.Contains(string(out), "threadStateBefore=0 threadStateAfter=0 attachResult=1 detachResult=1") {
			t.Fatalf("thread notifications still enter managed runtime: %s", out)
		}
		if path == sourcePath && strings.Contains(string(out), "threadStateBefore=0 threadStateAfter=0") {
			t.Fatal("fixture did not expose original managed thread attachment")
		}
	}
	cmd := exec.Command(helper, patchedPath)
	cmd.Env = append(os.Environ(), "POEM_NAOT_OBSERVE=1")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "exitedThreadStillListed=false") {
		t.Fatalf("managed callback cleanup: %v %s", err, out)
	}
}
