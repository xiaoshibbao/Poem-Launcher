//go:build windows

package injector

import (
	"encoding/json"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Uses our disposable process, never the user's game or an existing process.
func TestTwoModulesLoadBeforeMainAndUnicodeEnvironment(t *testing.T) {
	helper := os.Getenv("POEM_SMOKE_HELPER")
	if helper == "" {
		t.Skip("set POEM_SMOKE_HELPER to the compiled injection-helper fixture")
	}
	root := filepath.Dir(filepath.Dir(helper))
	report := filepath.Join(t.TempDir(), "report.json")
	proc, err := LaunchWithDLLs(helper, []string{filepath.Join(root, "assets", "GameFeatures.dll"), filepath.Join(root, "assets", "ProxyHook.dll")}, map[string]string{"POEM_SMOKE_OUTPUT": report, "POEM_PROXY": "127.0.0.1:45678", "POEM_UNICODE": "旅途测试"})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	proc.Wait()
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		GameLoaded  bool
		ProxyLoaded bool
		Proxy       string
		Unicode     string
	}
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !got.GameLoaded || !got.ProxyLoaded || got.Proxy != "127.0.0.1:45678" || got.Unicode != "旅途测试" {
		t.Fatalf("injection fixture: %s", b)
	}
	t.Logf("both original modules loaded in disposable process: %s", b)
	if _, err = LaunchWithDLLs(helper, []string{filepath.Join(root, "assets", "does-not-exist.dll")}, map[string]string{"POEM_SMOKE_OUTPUT": report}); err == nil {
		t.Fatal("missing DLL reported success")
	}
}

func TestStagedModulesLoadAcrossMain(t *testing.T) {
	helper := os.Getenv("POEM_SMOKE_HELPER")
	if helper == "" {
		t.Skip("set POEM_SMOKE_HELPER")
	}
	root := filepath.Dir(filepath.Dir(helper))
	report := filepath.Join(t.TempDir(), "staged.json")
	ready := func(_ windows.Handle, _ uint32) error {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			b, err := os.ReadFile(report + ".before")
			if err == nil {
				var before map[string]bool
				if err = json.Unmarshal(b, &before); err != nil {
					return err
				}
				if !before["gameLoaded"] || before["proxyLoaded"] {
					t.Errorf("wrong startup stage: %s", b)
				}
				return nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		return os.ErrDeadlineExceeded
	}
	proc, err := launchWithStages(helper, []string{filepath.Join(root, "assets", "GameFeatures.dll")}, []string{filepath.Join(root, "assets", "ProxyHook.dll")}, map[string]string{"POEM_SMOKE_OUTPUT": report, "POEM_SMOKE_STAGE": "1"}, ready)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	if err = os.WriteFile(report+".release", []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	proc.Wait()
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	if err = json.Unmarshal(b, &after); err != nil {
		t.Fatal(err)
	}
	if after["gameLoaded"] != true || after["proxyLoaded"] != true {
		t.Fatalf("missing deferred module: %s", b)
	}
}

func TestBlurModuleLoadsIntoDisposableProcess(t *testing.T) {
	helper := os.Getenv("POEM_SMOKE_HELPER")
	if helper == "" {
		t.Skip("set POEM_SMOKE_HELPER")
	}
	root := filepath.Dir(filepath.Dir(helper))
	report := filepath.Join(t.TempDir(), "blur.json")
	proc, err := LaunchWithDLLs(helper, []string{filepath.Join(root, "assets", "VisualPatch.dll")}, map[string]string{"POEM_SMOKE_OUTPUT": report})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	proc.Wait()
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["blurLoaded"] != true {
		t.Fatalf("blur module missing: %s", b)
	}
}

func TestThreeModulesWithBlurBeforeProxy(t *testing.T) {
	helper := os.Getenv("POEM_SMOKE_HELPER")
	if helper == "" {
		t.Skip("set POEM_SMOKE_HELPER")
	}
	root := filepath.Dir(filepath.Dir(helper))
	aliasDir := t.TempDir()
	for original, alias := range map[string]string{"GameFeatures.dll": "Poem.Game.dll", "ProxyHook.dll": "Poem.Proxy.dll", "VisualPatch.dll": "Poem.Visual.dll"} {
		b, err := os.ReadFile(filepath.Join(root, "assets", original))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(aliasDir, alias), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for run := 0; run < 12; run++ {
		report := filepath.Join(t.TempDir(), "combined.json")
		ready := func(_ windows.Handle, _ uint32) error {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if b, err := os.ReadFile(report + ".before"); err == nil {
					var before map[string]bool
					if err := json.Unmarshal(b, &before); err != nil {
						return err
					}
					if !before["gameLoaded"] || !before["blurLoaded"] || before["proxyLoaded"] {
						t.Errorf("incorrect combined stage: %s", b)
					}
					return nil
				}
				time.Sleep(20 * time.Millisecond)
			}
			return os.ErrDeadlineExceeded
		}
		proc, err := launchWithStages(helper, []string{filepath.Join(aliasDir, "Poem.Game.dll"), filepath.Join(aliasDir, "Poem.Visual.dll")}, []string{filepath.Join(aliasDir, "Poem.Proxy.dll")}, map[string]string{"POEM_SMOKE_OUTPUT": report, "POEM_SMOKE_STAGE": "1", "POEM_GAME_NAME": "Poem.Game.dll", "POEM_PROXY_NAME": "Poem.Proxy.dll", "POEM_VISUAL_NAME": "Poem.Visual.dll", "DOTNET_gcConcurrent": "0"}, ready)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(report+".release", []byte("ready"), 0600); err != nil {
			proc.Close()
			t.Fatal(err)
		}
		proc.Wait()
		code, err := proc.ExitCode()
		proc.Close()
		if err != nil || code != 0 {
			t.Fatalf("run %d exit=0x%X error=%v", run, code, err)
		}
		b, err := os.ReadFile(report)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got["gcConcurrent"] != "0" {
			t.Fatalf("runtime setting missing: %s", b)
		}
		for _, name := range []string{"gameLoaded", "blurLoaded", "proxyLoaded"} {
			if got[name] != true {
				t.Fatalf("missing %s: %s", name, b)
			}
		}
	}
}
