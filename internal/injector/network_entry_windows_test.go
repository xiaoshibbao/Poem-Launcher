//go:build windows

package injector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkEntryReadyBeforeMain(t *testing.T) {
	helper := os.Getenv("POEM_SMOKE_HELPER")
	if helper == "" {
		t.Skip("own process fixture not configured")
	}
	source, err := os.ReadFile("../../assets/NetworkEntry.dll")
	if err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(t.TempDir(), "Poem.Network.dll")
	if err = os.WriteFile(module, source, 0600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "report.json")
	proc, err := LaunchWithDLLs(helper, []string{module}, map[string]string{"POEM_SYSTEM_PROXY_PORT": "7890", "POEM_HTTP_PROXY_PORT": "45678", "POEM_SMOKE_OUTPUT": report})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	proc.Wait()
	if code, err := proc.ExitCode(); err != nil || code != 0 {
		t.Fatalf("own process failed %x %v", code, err)
	}
	if _, err = os.ReadFile(report); err != nil {
		t.Fatal("main did not execute after network readiness", err)
	}
}
