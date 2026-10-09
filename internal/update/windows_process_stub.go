//go:build !windows

package update

import "syscall"

func hiddenWindowsProcessAttr() *syscall.SysProcAttr { return nil }

func startUpdaterWindows(updaterPath string, planPath string, targets []ReplaceTarget) error {
	return startUpdaterElevatedWindows(updaterPath, planPath)
}
