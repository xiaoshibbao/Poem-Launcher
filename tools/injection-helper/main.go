package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	if os.Getenv("POEM_CONSOLE_FIXTURE") == "1" {
		syscall.NewLazyDLL("kernel32.dll").NewProc("AllocConsole").Call()
		out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			panic(err)
		}
		defer out.Close()
		fmt.Fprintln(out, "Poem recorder fixture ready")
		time.Sleep(2 * time.Second)
		fmt.Fprintln(out, "ShareBuff fixture marker, not an actual buffer failure")
		time.Sleep(2 * time.Second)
		return
	}

	get := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	has := func(name string) bool {
		ptr, _ := syscall.UTF16PtrFromString(name)
		v, _, _ := get.Call(uintptr(unsafe.Pointer(ptr)))
		return v != 0
	}
	moduleName := func(key, fallback string) string {
		if name := os.Getenv(key); name != "" {
			return name
		}
		return fallback
	}
	if os.Getenv("POEM_SMOKE_STAGE") == "1" {
		before, _ := json.Marshal(map[string]bool{"blurLoaded": has(moduleName("POEM_VISUAL_NAME", "VisualPatch.dll")), "gameLoaded": has(moduleName("POEM_GAME_NAME", "GameFeatures.dll")), "proxyLoaded": has(moduleName("POEM_PROXY_NAME", "ProxyHook.dll"))})
		_ = os.WriteFile(os.Getenv("POEM_SMOKE_OUTPUT")+".before", before, 0644)
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			if _, err := os.Stat(os.Getenv("POEM_SMOKE_OUTPUT") + ".release"); err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	b, _ := json.Marshal(map[string]any{"blurLoaded": has(moduleName("POEM_VISUAL_NAME", "VisualPatch.dll")), "gameLoaded": has(moduleName("POEM_GAME_NAME", "GameFeatures.dll")), "proxyLoaded": has(moduleName("POEM_PROXY_NAME", "ProxyHook.dll")), "gcConcurrent": os.Getenv("DOTNET_gcConcurrent"), "proxy": os.Getenv("POEM_PROXY"), "unicode": os.Getenv("POEM_UNICODE")})
	_ = os.WriteFile(os.Getenv("POEM_SMOKE_OUTPUT"), b, 0644)
}
