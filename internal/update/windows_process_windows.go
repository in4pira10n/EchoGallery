//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func hiddenWindowsProcessAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}

func startUpdaterWindows(updaterPath string, planPath string, targets []ReplaceTarget) error {
	if needsWindowsElevation(targets) {
		return startUpdaterElevatedWindows(updaterPath, planPath)
	}

	cmd := exec.Command(updaterPath, "apply-plan", planPath)
	cmd.Dir = filepath.Dir(updaterPath)
	cmd.SysProcAttr = hiddenWindowsProcessAttr()
	return cmd.Start()
}

func needsWindowsElevation(targets []ReplaceTarget) bool {
	for _, target := range targets {
		for _, path := range []string{target.MainDest, target.UpdaterDest} {
			if path == "" || !directoryIsWritable(filepath.Dir(path)) {
				return true
			}
		}
	}
	return false
}

func directoryIsWritable(dir string) bool {
	if dir == "" {
		return false
	}
	// The updater runs after the main process exits, so a temporary file in the
	// destination directory is the most reliable non-destructive permission probe.
	probe, err := os.CreateTemp(dir, ".eg-update-permission-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}
