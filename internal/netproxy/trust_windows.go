//go:build windows

package netproxy

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
)

// InstallTrust adds the proxy root CA certificate to the Windows trusted root
// store so the game accepts our MITM TLS certificates.
// Safe to call multiple times — certutil is idempotent for the same thumbprint.
func (ca *caState) InstallTrust() error {
	absPath, err := filepath.Abs(caCertFile)
	if err != nil {
		return err
	}
	// Ensure the cert file is on disk (it should already be from getCA, but
	// write it again to be safe).
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, ca.certPEM, 0644); err != nil {
			return err
		}
	}
	cmd := exec.Command("certutil", "-addstore", "-user", "Root", absPath)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil: %s", out)
	}
	return nil
}
