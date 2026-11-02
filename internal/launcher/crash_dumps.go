//go:build windows

package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows/registry"
)

// Set only in the diagnostic build. Normal releases do not configure WER.
var fullCrashDiagnostics = "false"

const localDumpsPath = `SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps`

var dumpSettings = struct {
	sync.Mutex
	refs map[string]int
}{refs: make(map[string]int)}

// Keep app-specific WER settings while its game sessions are alive. Do not
// replace another tool's settings, and remove only an unchanged owned key.
func acquireCrashDumps(dir, game string) (func(), error) {
	if fullCrashDiagnostics != "true" {
		return func() {}, nil
	}
	folder := filepath.Join(dir, "crash-dumps")
	if err := os.MkdirAll(folder, 0700); err != nil {
		return nil, err
	}
	name := filepath.Base(game)
	path := localDumpsPath + `\` + name
	dumpSettings.Lock()
	defer dumpSettings.Unlock()
	if dumpSettings.refs[path] == 0 {
		if err := installDumpKey(registry.LOCAL_MACHINE, path, folder); err != nil {
			return nil, err
		}
	}
	dumpSettings.refs[path]++
	var once sync.Once
	return func() {
		once.Do(func() {
			dumpSettings.Lock()
			defer dumpSettings.Unlock()
			dumpSettings.refs[path]--
			if dumpSettings.refs[path] == 0 {
				delete(dumpSettings.refs, path)
				if err := removeDumpKey(registry.LOCAL_MACHINE, path, folder); err != nil {
					writeLaunchLog(dir, "crash dump configuration cleanup failed: %v", err)
				}
			}
		})
	}, nil
}

func installDumpKey(root registry.Key, path, folder string) error {
	k, existed, err := registry.CreateKey(root, path, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		return fmt.Errorf("开启完整崩溃转储失败：%w", err)
	}
	defer func() {
		if k != 0 {
			_ = k.Close()
		}
	}()
	if existed {
		owner, _, e := k.GetStringValue("PoemDiagnosticOwner")
		if e != nil || owner != folder || !dumpKeyMatches(k, folder) {
			return fmt.Errorf("已有其他崩溃转储设置，未覆盖：%s", path)
		}
		return nil
	}
	// The marker is written last, after all required settings have succeeded.
	err = k.SetExpandStringValue("DumpFolder", folder)
	if err == nil {
		err = k.SetDWordValue("DumpCount", 2)
	}
	if err == nil {
		err = k.SetDWordValue("DumpType", 2)
	}
	if err == nil {
		err = k.SetStringValue("PoemDiagnosticOwner", folder)
	}
	if err != nil {
		k.Close()
		k = 0
		_ = registry.DeleteKey(root, path)
		return fmt.Errorf("配置完整崩溃转储失败：%w", err)
	}
	return nil
}

func dumpKeyMatches(k registry.Key, folder string) bool {
	owner, _, e1 := k.GetStringValue("PoemDiagnosticOwner")
	dest, _, e2 := k.GetStringValue("DumpFolder")
	kind, _, e3 := k.GetIntegerValue("DumpType")
	count, _, e4 := k.GetIntegerValue("DumpCount")
	names, e5 := k.ReadValueNames(-1)
	info, e6 := k.Stat()
	return e1 == nil && e2 == nil && e3 == nil && e4 == nil && e5 == nil && e6 == nil &&
		owner == folder && dest == folder && kind == 2 && count == 2 && len(names) == 4 && info.SubKeyCount == 0
}

func removeDumpKey(root registry.Key, path, folder string) error {
	k, err := registry.OpenKey(root, path, registry.READ|registry.WOW64_64KEY)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	owned := dumpKeyMatches(k, folder)
	k.Close()
	if !owned {
		return fmt.Errorf("转储设置已变化，保留当前设置：%s", path)
	}
	return registry.DeleteKey(root, path)
}
