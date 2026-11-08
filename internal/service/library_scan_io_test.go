package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestScanIOCancellationDoesNotWaitForOSRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	done := make(chan error, 1)
	go func() {
		_, err := scanIO(ctx, "blocked", func() (int, error) { close(entered); <-release; return 3, nil })
		done <- err
	}()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation waited for the OS")
	}
}

func TestDirectoryProgressIsReportedBeforeBlockingRead(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var last LibraryScanProgress
	ctx = WithLibraryScanProgress(ctx, func(p LibraryScanProgress) { last = p })
	err := walkLibraryDirectories(ctx, root, func(path string) ([]fs.DirEntry, error) {
		if last.Operation != "read_directory" || last.Path != root || last.PendingDirectories != 1 {
			t.Fatalf("missing live directory progress: %+v", last)
		}
		cancel()
		return nil, ctx.Err()
	}, func(string, fs.DirEntry, []fs.DirEntry) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSMBScanSingleEnumerationAndIncrementalChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a double-slash alias to exercise the UNC policy on a local fixture")
	}
	svc, _ := newTestPhotoService(t)
	svc.sourcePath = "/" + svc.sourcePath
	if !svc.IsSMBLibrary() {
		t.Fatal("UNC path not detected")
	}
	write := func(rel string) {
		t.Helper()
		path := filepath.Join(svc.sourcePath, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, createJPEGBytes(16, 16), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("A/one.jpg")
	write("B/two.jpg")
	scan := func() (*ImportSummary, []LibraryScanProgress) {
		t.Helper()
		var events []LibraryScanProgress
		ctx := WithLibraryScanProgress(WithBackgroundLibraryScan(context.Background()), func(p LibraryScanProgress) { events = append(events, p) })
		summary, err := svc.ImportExistingPhotosContext(ctx, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]int{}
		for _, p := range events {
			if p.Operation == "read_directory" {
				seen[p.Path]++
				if seen[p.Path] > 1 {
					t.Fatalf("directory enumerated more than once: %s", p.Path)
				}
			}
		}
		if len(seen) != 3 {
			t.Fatalf("expected 3 directory reads, got %v", seen)
		}
		return summary, events
	}
	first, _ := scan()
	if first.Imported != 2 {
		t.Fatalf("first: %+v", first)
	}
	second, _ := scan()
	if second.Imported != 0 || second.Pruned != 0 || second.Changed {
		t.Fatalf("unchanged: %+v", second)
	}
	// A prior interrupted import may leave an indexed directory with a missing
	// media row. Reuse the same enumeration to recover it even if the directory
	// itself is stable.
	rows, _ := svc.repo.ListPhotos(storage.ListPhotosParams{UserID: 1})
	delete(svc.repo.(*mockRepo).photos, rows.Photos[0].ID)
	recovered, _ := scan()
	if recovered.Imported != 1 {
		t.Fatalf("stable directory did not recover its missing media record: %+v", recovered)
	}
	snapshot, _ := svc.repo.GetLibraryScanSnapshot()
	if snapshot.Version != 2 {
		t.Fatal("snapshot version changed")
	}
	if needed, err := svc.NeedsAutomaticLibraryBuild(context.Background()); err != nil || needed {
		t.Fatalf("indexed SMB should open without scanning: %v %v", needed, err)
	}
	write("A/new.jpg")
	third, _ := scan()
	if third.Imported != 1 {
		t.Fatalf("added: %+v", third)
	}
	if err := os.Remove(filepath.Join(svc.sourcePath, "B", "two.jpg")); err != nil {
		t.Fatal(err)
	}
	fourth, _ := scan()
	if fourth.Pruned != 1 {
		t.Fatalf("removed: %+v", fourth)
	}
	if err := os.Rename(filepath.Join(svc.sourcePath, "A"), filepath.Join(svc.sourcePath, "Moved")); err != nil {
		t.Fatal(err)
	}
	fifth, _ := scan()
	if fifth.Imported != 0 || fifth.Pruned != 0 || !fifth.Changed {
		t.Fatalf("moved: %+v", fifth)
	}
	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 20})
	if err != nil || len(page.Photos) != 2 {
		t.Fatalf("lost media after move: %+v %v", page, err)
	}
	for _, p := range page.Photos {
		if !strings.HasPrefix(p.SourceRelPath, "Moved/") {
			t.Fatalf("old source: %s", p.SourceRelPath)
		}
	}
}

func TestCancelledDiscoveryPreservesSnapshotAndCanRetry(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	path := filepath.Join(svc.sourcePath, "photo.jpg")
	if err := os.WriteFile(path, createJPEGBytes(16, 16), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportExistingPhotos(1, nil); err != nil {
		t.Fatal(err)
	}
	previous, _ := svc.repo.GetLibraryScanSnapshot()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithLibraryScanProgress(ctx, func(p LibraryScanProgress) {
		if p.Operation == "read_directory" {
			cancel()
		}
	})
	if _, err := svc.ImportExistingPhotosContext(ctx, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	after, _ := svc.repo.GetLibraryScanSnapshot()
	if !reflect.DeepEqual(previous, after) {
		t.Fatal("cancelled scan replaced snapshot")
	}
	page, _ := svc.repo.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(page.Photos) != 1 {
		t.Fatal("cancelled scan pruned media")
	}
	retry, err := svc.ImportExistingPhotos(1, nil)
	if err != nil || retry.Pruned != 1 {
		t.Fatalf("retry failed: %+v %v", retry, err)
	}
}

func TestFailedDiscoveryCanRetryAndScanLockCanCancel(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	original := svc.sourcePath
	svc.sourcePath = filepath.Join(original, "missing")
	if _, err := svc.ImportExistingPhotos(1, nil); err == nil {
		t.Fatal("missing root must fail")
	}
	svc.sourcePath = original
	if _, err := svc.ImportExistingPhotos(1, nil); err != nil {
		t.Fatal(err)
	}
	svc.scanMu.Lock()
	defer svc.scanMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.ImportExistingPhotosContext(ctx, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSMBPathPolicy(t *testing.T) {
	for _, path := range []string{`\\192.168.1.4\Pictures`, `//nas/photos`, `\\?\UNC\nas\photos`} {
		if !IsSMBPath(path) {
			t.Errorf("not SMB: %s", path)
		}
	}
	for _, path := range []string{t.TempDir(), `C:\Photos`, `\\?\C:\Photos`, `\\.\C:\Photos`, `\\server@SSL\DavWWWRoot\photos`} {
		if IsSMBPath(path) {
			t.Errorf("unexpected SMB: %s", path)
		}
	}
}
