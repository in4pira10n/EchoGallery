package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFileManagerRevealCommand(t *testing.T) {
	originalOS := currentOS
	t.Cleanup(func() { currentOS = originalOS })

	target := filepath.Join("demo", "nested", "photo.jpg")

	tests := []struct {
		name     string
		goos     string
		wantName string
		wantArgs []string
	}{
		{
			name:     "darwin",
			goos:     "darwin",
			wantName: "open",
			wantArgs: []string{"-R", filepath.Clean(target)},
		},
		{
			name:     "windows",
			goos:     "windows",
			wantName: "explorer",
			wantArgs: []string{"/select,", filepath.Clean(target)},
		},
		{
			name:     "linux",
			goos:     "linux",
			wantName: "xdg-open",
			wantArgs: []string{filepath.Dir(filepath.Clean(target))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentOS = tt.goos
			gotName, gotArgs := fileManagerRevealCommand(target)
			if gotName != tt.wantName {
				t.Fatalf("command = %q, want %q", gotName, tt.wantName)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("args len = %d, want %d", len(gotArgs), len(tt.wantArgs))
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Fatalf("arg[%d] = %q, want %q", i, gotArgs[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestSystemOpenCommand(t *testing.T) {
	originalOS := currentOS
	t.Cleanup(func() { currentOS = originalOS })

	target := filepath.Join("demo", "nested", "video.mp4")

	tests := []struct {
		name     string
		goos     string
		wantName string
		wantArgs []string
	}{
		{
			name:     "darwin",
			goos:     "darwin",
			wantName: "open",
			wantArgs: []string{filepath.Clean(target)},
		},
		{
			name:     "windows",
			goos:     "windows",
			wantName: "cmd",
			wantArgs: []string{"/c", "start", "", filepath.Clean(target)},
		},
		{
			name:     "linux",
			goos:     "linux",
			wantName: "xdg-open",
			wantArgs: []string{filepath.Clean(target)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentOS = tt.goos
			gotName, gotArgs := systemOpenCommand(target)
			if gotName != tt.wantName {
				t.Fatalf("command = %q, want %q", gotName, tt.wantName)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("args len = %d, want %d", len(gotArgs), len(tt.wantArgs))
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Fatalf("arg[%d] = %q, want %q", i, gotArgs[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestRevealInFileManager_WindowsExitCodeOneIsIgnored(t *testing.T) {
	originalOS := currentOS
	originalExec := execCommand
	t.Cleanup(func() {
		currentOS = originalOS
		execCommand = originalExec
	})

	currentOS = "windows"
	execCommand = func(name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestRevealInFileManagerHelper", "--", name}
		cmdArgs = append(cmdArgs, args...)
		cmd := exec.Command(os.Args[0], cmdArgs...)
		exitCode := "1"
		if name == "powershell" {
			exitCode = "0"
		}
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_EXIT_CODE="+exitCode)
		return cmd
	}

	if err := revealInFileManager(filepath.Join("demo", "photo.jpg")); err != nil {
		t.Fatalf("windows exit code 1 应被忽略，得到 %v", err)
	}
}

func TestRevealInFileManager_PropagatesOtherErrors(t *testing.T) {
	originalOS := currentOS
	originalExec := execCommand
	t.Cleanup(func() {
		currentOS = originalOS
		execCommand = originalExec
	})

	currentOS = "windows"
	execCommand = func(name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestRevealInFileManagerHelper", "--", name}
		cmdArgs = append(cmdArgs, args...)
		cmd := exec.Command(os.Args[0], cmdArgs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_EXIT_CODE=2")
		return cmd
	}

	if err := revealInFileManager(filepath.Join("demo", "photo.jpg")); err == nil {
		t.Fatal("期望返回非 1 的错误退出码")
	}
}

func TestWindowsFileManagerFocusCommand(t *testing.T) {
	name, args := windowsFileManagerFocusCommand()
	if name != "powershell" {
		t.Fatalf("command = %q, want powershell", name)
	}
	if len(args) < 4 {
		t.Fatalf("args len = %d, want >= 4", len(args))
	}
	if args[0] != "-NoProfile" || args[1] != "-WindowStyle" || args[2] != "Hidden" || args[3] != "-Command" {
		t.Fatalf("unexpected args prefix: %v", args[:4])
	}
}

func TestRevealInFileManagerHelper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code := 0
	if _, err := fmt.Sscanf(os.Getenv("HELPER_EXIT_CODE"), "%d", &code); err != nil {
		code = 0
	}
	os.Exit(code)
}
