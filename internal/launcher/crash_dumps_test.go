//go:build windows

package launcher

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"testing"
	"time"
)

func TestCrashDumpRegistryOwnership(t *testing.T) {
	// Exercise a disposable HKCU key; never configure the real game's HKLM key.
	path := fmt.Sprintf(`Software\PoemDiagnosticTest-%d`, time.Now().UnixNano())
	folder := filepath.Join(t.TempDir(), "dumps")
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	if err := installDumpKey(registry.CURRENT_USER, path, folder); err != nil {
		t.Fatal(err)
	}
	if err := installDumpKey(registry.CURRENT_USER, path, folder); err != nil {
		t.Fatal("reacquire", err)
	}
	if err := installDumpKey(registry.CURRENT_USER, path, folder+"-other"); err == nil {
		t.Fatal("replaced other owner")
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.SetDWordValue("DumpType", 1); err != nil {
		t.Fatal(err)
	}
	k.Close()
	if err := removeDumpKey(registry.CURRENT_USER, path, folder); err == nil {
		t.Fatal("removed changed settings")
	}
	k, err = registry.OpenKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.SetDWordValue("DumpType", 2); err != nil {
		t.Fatal(err)
	}
	if err := k.SetStringValue("OtherTool", "keep"); err != nil {
		t.Fatal(err)
	}
	k.Close()
	if err := removeDumpKey(registry.CURRENT_USER, path, folder); err == nil {
		t.Fatal("removed other tool data")
	}
	k, err = registry.OpenKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.DeleteValue("OtherTool"); err != nil {
		t.Fatal(err)
	}
	k.Close()
	if err := removeDumpKey(registry.CURRENT_USER, path, folder); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, path, registry.READ); err != registry.ErrNotExist {
		t.Fatalf("key remains: %v", err)
	}
}
