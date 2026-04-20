package service

import (
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
