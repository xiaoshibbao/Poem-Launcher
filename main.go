package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"poem/internal/dialog"
	"poem/internal/injector"
	"poem/internal/launcher"
	"poem/internal/window"
)

//go:embed all:frontend/dist
var frontend embed.FS

//go:embed assets/ProxyHook.dll assets/GameFeatures.dll assets/VisualPatch.dll assets/NetworkEntry.dll assets/LuaEntry.dll assets/appicon.png
var assets embed.FS

func mustRead(name string) []byte {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		log.Fatalf("embedded asset %s missing: %v", name, err)
	}
	return b
}

func main() {
	// The same executable doubles as the console recorder child process.
	if injector.ConsoleRecorderMain() {
		return
	}
	if exe, err := os.Executable(); err == nil {
		_ = os.Chdir(filepath.Dir(exe))
	}

	service := launcher.New(launcher.Modules{
		Proxy:    mustRead("ProxyHook.dll"),
		Features: mustRead("GameFeatures.dll"),
		Visual:   mustRead("VisualPatch.dll"),
		Network:  mustRead("NetworkEntry.dll"),
		Lua:      mustRead("LuaEntry.dll"),
	})
	app := application.New(application.Options{
		Name:        "Poem",
		Description: "Poem Launcher · 星穹铁道启动器",
		Icon:        mustRead("appicon.png"),
		Services: []application.Service{
			application.NewService(service),
			application.NewService(&dialog.Service{}),
			application.NewService(&window.Service{}),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend)},
	})
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "Poem",
		Width:                      1280,
		Height:                     800,
		MinWidth:                   1100,
		MinHeight:                  720,
		Frameless:                  true,
		BackgroundColour:           application.NewRGB(13, 18, 25),
		URL:                        "/",
		DevToolsEnabled:            false,
		DefaultContextMenuDisabled: true,
		ZoomControlEnabled:         false,
	})
	// Closing while a game is running only minimises, so the proxy stays alive.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if service.GetStatus().Running > 0 {
			e.Cancel()
			win.Minimise()
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
