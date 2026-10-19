package danmaku

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveMatchKeepsFilesAndOtherMatches(t *testing.T) {
	root := t.TempDir()
	page := Page{Page: 1, CID: 123, File: "P1.xml", Status: "completed"}
	for bvid, uuid := range map[string]string{"BV1SopR6wE2x": "target", "BV1SopR6wE2y": "other"} {
		if _, err := WritePage(root, bvid, page, []byte("<i>retained</i>"), false); err != nil {
			t.Fatal(err)
		}
		if err := WriteMetadata(root, Metadata{Video: Video{BVID: bvid, Pages: []Page{page}}, MediaID: 42, MediaUUID: uuid}); err != nil {
			t.Fatal(err)
		}
	}
	manager := NewManager(nil)
	manager.active["BV1SopR6wE2x"] = "running"
	if err := manager.RemoveMatch(root, "target"); !errors.Is(err, ErrTaskRunning) {
		t.Fatalf("expected running-task conflict, got %v", err)
	}
	delete(manager.active, "BV1SopR6wE2x")
	for attempt := 0; attempt < 2; attempt++ {
		if err := manager.RemoveMatch(root, "target"); err != nil {
			t.Fatal(err)
		}
	}
	target, err := ReadMetadata(root, "BV1SopR6wE2x")
	if err != nil || target.MediaUUID != "" || target.MediaID != 0 || target.Pages[0].Status != "completed" {
		t.Fatalf("unexpected removed match: %#v, %v", target, err)
	}
	other, err := ReadMetadata(root, "BV1SopR6wE2y")
	if err != nil || other.MediaUUID != "other" || other.MediaID != 42 {
		t.Fatalf("unrelated match changed: %#v, %v", other, err)
	}
	data, err := os.ReadFile(filepath.Join(root, target.BVID, page.File))
	if err != nil || string(data) != "<i>retained</i>" {
		t.Fatalf("XML changed: %q, %v", data, err)
	}
}

func TestWritePageAndMetadata(t *testing.T) {
	root := t.TempDir()
	page := Page{Page: 1, CID: 123, File: "P1.xml"}
	skipped, err := WritePage(root, "BV1SopR6wE2x", page, []byte("<i></i>"), false)
	if err != nil || skipped {
		t.Fatalf("first write: skipped=%v err=%v", skipped, err)
	}
	skipped, err = WritePage(root, "BV1SopR6wE2x", page, []byte("replacement"), false)
	if err != nil || !skipped {
		t.Fatalf("second write: skipped=%v err=%v", skipped, err)
	}
	if _, err := WritePage(root, "BV1SopR6wE2x", page, []byte("replacement"), true); err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{Video: Video{BVID: "BV1SopR6wE2x", Title: "title", Owner: "owner", Pages: []Page{page}}, MediaID: 42, MediaUUID: "media-uuid"}
	metadata.Pages[0].Status = "completed"
	if err := WriteMetadata(root, metadata); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadMetadata(root, metadata.BVID)
	if err != nil || loaded.MediaID != 42 || loaded.MediaUUID != "media-uuid" || loaded.Pages[0].Status != "completed" {
		t.Fatalf("metadata roundtrip: %#v err=%v", loaded, err)
	}
	data, err := os.ReadFile(filepath.Join(root, metadata.BVID, "P1.xml"))
	if err != nil || string(data) != "replacement" {
		t.Fatalf("xml contents %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, metadata.BVID, "P1.xml.part")); !os.IsNotExist(err) {
		t.Fatalf("unexpected part file: %v", err)
	}
}
