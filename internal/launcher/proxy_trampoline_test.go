//go:build windows

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeProxyAllocatorUsesActualAddress(t *testing.T) {
	helper := os.Getenv("POEM_TRAMPOLINE_FIXTURE")
	if helper == "" {
		t.Skip("native allocation fixture not configured")
	}
	sourcePath, err := filepath.Abs("../../assets/ProxyHook.dll")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	patched, err := configureProxyThreadNotifications(source)
	if err != nil {
		t.Fatal(err)
	}
	patchedPath := filepath.Join(t.TempDir(), "Poem.Proxy.dll")
	if err = os.WriteFile(patchedPath, patched, 0600); err != nil {
		t.Fatal(err)
	}
	for _, allocator := range []string{"f080", "f220", "protect"} {
		for _, version := range []struct{ path, want string }{{sourcePath, "matches=0"}, {patchedPath, "matches=1"}} {
			if allocator == "protect" {
				version.want = "protectionWritable=0"
				if version.path == patchedPath {
					version.want = "protectionWritable=1"
				}
			}
			output, err := exec.Command(helper, version.path, allocator).CombinedOutput()
			t.Logf("%s %s: %s", filepath.Base(version.path), allocator, output)
			if err != nil || !strings.Contains(string(output), version.want) {
				t.Fatalf("allocator result: %v %s", err, output)
			}
			if allocator != "protect" && version.path == patchedPath && !strings.Contains(string(output), "safeWrite64=1") {
				t.Fatalf("trampoline buffer not writable: %s", output)
			}
		}
	}
}
func TestProxyAllocationPatchRejectsUnknownLayout(t *testing.T) {
	source, err := os.ReadFile("../../assets/ProxyHook.dll")
	if err != nil {
		t.Fatal(err)
	}
	source[59240] ^= 1
	before := append([]byte(nil), source...)
	if err := patchProxyAllocationResult(source); err == nil {
		t.Fatal("unknown layout accepted")
	}
	for i := range source {
		if source[i] != before[i] {
			t.Fatal("partially modified rejected module")
		}
	}
}
