package danmaku

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMediaFileCleanerRejectsActiveDownloads(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := NewClient(server.Client())
	client.BaseURL = server.URL
	manager := NewManager(client)
	root := t.TempDir()
	bvid := "BV1SopR6wE2x"
	if err := WriteMetadata(root, Metadata{Video: Video{BVID: bvid}, MediaUUID: "missing"}); err != nil {
		t.Fatal(err)
	}
	cleaner, err := NewMediaFileCleaner(root)
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.Start(context.Background(), root, bvid, 1, false, 1, "missing")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cancel(task.ID)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("test download did not start")
	}
	if err := cleaner.DeleteMediaFiles(context.Background(), "missing"); !errors.Is(err, ErrTaskRunning) {
		t.Fatalf("running download must block cleanup: %v", err)
	}
	if _, err := ReadMetadata(root, bvid); err != nil {
		t.Fatal(err)
	}
	manager.Cancel(task.ID)
	select {
	case <-manager.get(task.ID).done:
	case <-time.After(3 * time.Second):
		t.Fatal("test download did not stop")
	}
	if err := cleaner.DeleteMediaFiles(context.Background(), "missing"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMetadata(root, bvid); !os.IsNotExist(err) {
		t.Fatalf("cache survived after download stopped: %v", err)
	}
}

func TestMediaFileCleanerPreservesChangedAssociations(t *testing.T) {
	root := t.TempDir()
	metadata := Metadata{Video: Video{BVID: "BV1SopR6wE2x"}, MediaUUID: "old"}
	if err := WriteMetadata(root, metadata); err != nil {
		t.Fatal(err)
	}
	cleaner, err := NewMediaFileCleaner(root)
	if err != nil {
		t.Fatal(err)
	}
	metadata.MediaUUID = "live"
	if err := WriteMetadata(root, metadata); err != nil {
		t.Fatal(err)
	}
	if err := cleaner.DeleteMediaFiles(context.Background(), "old"); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadMetadata(root, metadata.BVID); err != nil || got.MediaUUID != "live" {
		t.Fatalf("new association lost: %+v %v", got, err)
	}
}
