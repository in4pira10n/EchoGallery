//go:build !windows

package update

import "fmt"

func startUpdaterElevatedWindows(updaterPath string, planPath string) error {
	return fmt.Errorf("windows elevation is not supported on this platform")
}
