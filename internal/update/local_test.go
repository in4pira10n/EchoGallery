package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPlanContinuesAfterFailedTargetAndSignalsRestart(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.WriteFile(source, []byte("new executable"), 0755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "bad")
	if err := os.Mkdir(bad, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "occupied"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(root, "good")
	signal := filepath.Join(root, "ready")
	planPath := filepath.Join(root, "plan.json")
	if err := writeJSON(planPath, ApplyPlan{MainSourcePath: source, RestartSignalPath: signal, Targets: []ReplaceTarget{{MainDest: bad}, {MainDest: good}}}); err != nil {
		t.Fatal(err)
	}
	if err := RunApplyPlan(planPath); err == nil {
		t.Fatal("expected failed target report")
	}
	if data, err := os.ReadFile(good); err != nil || string(data) != "new executable" {
		t.Fatalf("later target not updated: %s %v", data, err)
	}
	if _, err := os.Stat(signal); err != nil {
		t.Fatalf("restart not signalled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "update-error.log")); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsRestartWatcherReusesCurrentConsole(t *testing.T) {
	script := buildWindowsRestartWatcherScript(`C:\Temp\ready`, StartOptions{CurrentExecutable: `C:\Gallery One\EchoGallery.exe`})
	if !strings.Contains(script, `start "EchoGallery" /b /wait "C:\Gallery One\EchoGallery.exe"`) || strings.Contains(script, "call ") {
		t.Fatalf("invalid launch: %s", script)
	}
}

func TestIsWindowsProtectedInstallPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "windows apps", path: `C:\Program Files\WindowsApps\EchoGallery\EchoGallery.exe`, want: true},
		{name: "long windows apps path", path: `\\?\C:\Program Files\WindowsApps\EchoGallery\EchoGallery.exe`, want: true},
		{name: "modifiable windows apps", path: `C:\Program Files\ModifiableWindowsApps\EchoGallery\EchoGallery.exe`, want: true},
		{name: "ordinary portable install", path: `C:\Apps\EchoGallery\EchoGallery.exe`, want: false},
		{name: "windows apps substring", path: `C:\Users\WindowsAppsBackup\EchoGallery.exe`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWindowsProtectedInstallPath(tt.path); got != tt.want {
				t.Fatalf("isWindowsProtectedInstallPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateWindowsUpdateTargetsRejectsProtectedPath(t *testing.T) {
	err := validateWindowsUpdateTargets([]ReplaceTarget{{
		MainDest:    `C:\Program Files\WindowsApps\EchoGallery\EchoGallery.exe`,
		UpdaterDest: `C:\Program Files\WindowsApps\EchoGallery\EchoGalleryUpdater.exe`,
	}})
	if err == nil {
		t.Fatal("expected protected WindowsApps target to be rejected")
	}
}
