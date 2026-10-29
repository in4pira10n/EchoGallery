package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIncrementalSnapshotRemainsReusableAfterRootChanges(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	root := svc.sourcePath
	nested := filepath.Join(root, "stable", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"stable/one.jpg", "stable/nested/two.jpg", "root.jpg"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), createJPEGBytes(16, 16), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ImportExistingPhotos(1, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.jpg"), createJPEGBytes(16, 16), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportExistingPhotos(1, nil); err != nil {
		t.Fatal(err)
	}
	needsBuild, err := svc.NeedsLibraryBuild(context.Background())
	if err != nil || needsBuild {
		t.Fatalf("completed incremental scan must be reusable: needsBuild=%v err=%v", needsBuild, err)
	}
	snapshot, err := svc.repo.GetLibraryScanSnapshot()
	if err != nil || snapshot == nil {
		t.Fatalf("missing snapshot: %v", err)
	}
	if snapshot.FileCount != 4 || len(snapshot.DirectoryModTimes) != 3 {
		t.Fatalf("snapshot lost stable subtree: files=%d directories=%d", snapshot.FileCount, len(snapshot.DirectoryModTimes))
	}
	for rel, expected := range snapshot.DirectoryEntryHashes {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || expected != hashDirectoryEntries(entries) {
			t.Fatalf("new snapshot contains an incomplete entry hash for %q: err=%v", rel, err)
		}
	}
}

func TestLegacyV2SnapshotSkipsUnchangedLibraryButDetectsNewDirectories(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	root := svc.sourcePath
	if err := os.MkdirAll(filepath.Join(root, "existing", "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), createJPEGBytes(16, 16), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportExistingPhotos(1, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.repo.GetLibraryScanSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var legacyEntries []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			legacyEntries = append(legacyEntries, entry)
		}
	}
	snapshot.DirectoryEntryHashes["."] = hashDirectoryEntries(legacyEntries)
	if err := svc.repo.SaveLibraryScanSnapshot(*snapshot); err != nil {
		t.Fatal(err)
	}
	if needs, err := svc.NeedsLibraryBuild(context.Background()); err != nil || needs {
		t.Fatalf("legacy v2 snapshot should not require rebuilding: needs=%v err=%v", needs, err)
	}
	summary, err := svc.ImportExistingPhotos(1, nil)
	if err != nil || summary.Imported != 0 || summary.Skipped != 1 {
		t.Fatalf("unchanged legacy snapshot must skip media: summary=%+v err=%v", summary, err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "new-folder"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(root, rootInfo.ModTime(), rootInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if needs, err := svc.NeedsLibraryBuild(context.Background()); err != nil || !needs {
		t.Fatalf("new directory must still be detected with unchanged mtime: needs=%v err=%v", needs, err)
	}
}

func TestSnapshotFindsChangedDescendantBelowStableParents(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	root := svc.sourcePath
	nested := filepath.Join(root, "parent", "child")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "one.jpg"), createJPEGBytes(16, 16), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportExistingPhotos(1, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "two.jpg"), createJPEGBytes(16, 16), 0644); err != nil {
		t.Fatal(err)
	}
	summary, err := svc.ImportExistingPhotos(1, nil)
	if err != nil || summary.Imported != 1 {
		t.Fatalf("new media under stable parents not imported: summary=%+v err=%v", summary, err)
	}
	if needs, err := svc.NeedsLibraryBuild(context.Background()); err != nil || needs {
		t.Fatalf("completed descendant scan must be reusable: needs=%v err=%v", needs, err)
	}
}
