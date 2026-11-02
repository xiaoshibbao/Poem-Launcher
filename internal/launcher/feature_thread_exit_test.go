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

func TestFeatureThreadExitPatch(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "assets", "GameFeatures.dll"))
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(source)
	patched, err := configureFeatureThreadExit(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, original) {
		t.Fatal("source mutated")
	}
	if len(patched) != len(source) {
		t.Fatal("binary layout changed")
	}
	for i := range source {
		if source[i] != patched[i] && !(i >= 560856 && i < 560872 || i >= 33191 && i < 33194) {
			t.Fatalf("unexpected edit at %x", i)
		}
	}

	source[0] ^= 1
	if _, err = configureFeatureThreadExit(source); err == nil {
		t.Fatal("unknown module accepted")
	}
}

func TestFeatureNativeThreadCleanup(t *testing.T) {
	helper := os.Getenv("POEM_THREAD_EXIT_FIXTURE")
	if helper == "" {
		t.Skip("set POEM_THREAD_EXIT_FIXTURE to the native-thread fixture")
	}
	sourcePath, err := filepath.Abs(filepath.Join("..", "..", "assets", "GameFeatures.dll"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	patched, err := configureFeatureThreadExit(source)
	if err != nil {
		t.Fatal(err)
	}
	patchedPath := filepath.Join(t.TempDir(), "Poem.Game.dll")
	if err := os.WriteFile(patchedPath, patched, 0600); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ path, want string }{{sourcePath, "exitCode=0 cleanupCallbacks=0"}, {patchedPath, "exitCode=0 cleanupCallbacks=1"}} {
		output, err := exec.Command(helper, item.path).CombinedOutput()
		if err != nil || !strings.Contains(string(output), item.want) {
			t.Fatalf("native thread cleanup: %v %s", err, output)
		}
	}
}
