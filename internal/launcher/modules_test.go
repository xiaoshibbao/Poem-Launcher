//go:build windows

package launcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeCleanupRetainsUnknownFiles(t *testing.T) {
	dir, err := os.MkdirTemp("", "Poem-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "Poem.Game.dll"), []byte("module"), 0600)
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0600)
	cleanupRuntimeModules(dir)
	if _, err := os.Stat(filepath.Join(dir, "Poem.Game.dll")); !os.IsNotExist(err) {
		t.Fatal("module was not cleaned")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatal("unknown file was removed")
	}
}
func TestLegacyCleanupChecksContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, legacyFeatureName), []byte("expected"), 0600)
	os.WriteFile(filepath.Join(dir, legacyVisualName), []byte("other"), 0600)
	cleanupLegacyModules(dir, []byte("expected"))
	if _, err := os.Stat(filepath.Join(dir, legacyFeatureName)); !os.IsNotExist(err) {
		t.Fatal("matching legacy module remains")
	}
	if _, err := os.Stat(filepath.Join(dir, legacyVisualName)); err != nil {
		t.Fatal("unrecognized content removed")
	}
}
