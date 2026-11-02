package dialog

import (
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Service struct{}

func (f *Service) PickFile(filter string) (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		ResolvesAliases(true)
	dialog.SetTitle("Select a file/directory")
	if filter == "exe" {
		dialog.AddFilter("Executable Files (*.exe)", "*.exe")
	}
	if filter == "lua" {
		dialog.AddFilter("Lua Scripts (*.lua)", "*.lua")
	}
	if path, err := dialog.PromptForSingleSelection(); err == nil {
		return path, nil
	}
	return "", nil
}

func (f *Service) OpenFolder(path string) (bool, string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, "failed to resolve absolute path: " + err.Error()
	}

	if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
		return false, "directory not found: " + absPath
	}

	url := "file:///" + filepath.ToSlash(absPath)
	application.Get().Browser.OpenURL(url)

	return true, ""
}
