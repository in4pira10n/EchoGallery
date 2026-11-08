package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckLibraryReachable(t *testing.T) {
	root := t.TempDir()
	if err := CheckLibraryReachable(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err := CheckLibraryReachable(context.Background(), filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing directory was reported reachable")
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckLibraryReachable(context.Background(), file); err == nil {
		t.Fatal("file was reported as a library directory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckLibraryReachable(ctx, root); err != context.Canceled {
		t.Fatalf("cancelled check returned %v", err)
	}
}
