package sqlite

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestPortableLeaseBlocksSecondOwnerAndReleasesOnClose(t *testing.T) {
	source := filepath.Join(t.TempDir(), "metadata.sqlite")
	first, err := acquirePortableLease(source)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquirePortableLease(source); !errors.Is(err, ErrPortableLibraryOccupied) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("expected occupied lease, got %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := acquirePortableLease(source)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestPortableReopenReadsNewDurableState(t *testing.T) {
	source := filepath.Join(t.TempDir(), "metadata.sqlite")
	first, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	durable, err := New(source)
	if err != nil {
		t.Fatal(err)
	}
	album := &storage.Album{Name: "other-gallery", CreatedBy: 1, CreatedAt: time.Now()}
	if err := durable.CreateAlbum(album); err != nil {
		t.Fatal(err)
	}
	durable.Close()
	next, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	albums, err := next.ListAlbums(1)
	if err != nil || len(albums) != 1 {
		t.Fatalf("stale cache: %v, %v", albums, err)
	}
}

func TestPortableWorkingCopySyncPersistsAlbumToLibraryDatabase(t *testing.T) {
	source := filepath.Join(t.TempDir(), ".echogallery", "metadata.sqlite")
	working, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatalf("打开 SQLite 工作副本失败: %v", err)
	}
	album := &storage.Album{Name: "可持久化相册", CreatedBy: 7, CreatedAt: time.Now()}
	if err := working.CreateAlbum(album); err != nil {
		t.Fatalf("在工作副本创建相册失败: %v", err)
	}
	if err := working.SyncPortable(); err != nil {
		t.Fatalf("同步资源库数据库失败: %v", err)
	}
	if err := working.Close(); err != nil {
		t.Fatalf("关闭工作副本失败: %v", err)
	}

	durable, err := New(source)
	if err != nil {
		t.Fatalf("打开资源库数据库失败: %v", err)
	}
	defer durable.Close()
	albums, err := durable.ListAlbums(7)
	if err != nil {
		t.Fatalf("读取资源库相册失败: %v", err)
	}
	if len(albums) != 1 || albums[0].Name != album.Name {
		t.Fatalf("相册未写入资源库数据库: %+v", albums)
	}
}

func TestPortableUnchangedSyncAndReopenDoNotReplaceDurableDatabase(t *testing.T) {
	source := filepath.Join(t.TempDir(), "metadata.sqlite")
	working, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := working.EnsureLibraryMetadata("library", "Library", "#123456"); err != nil {
		t.Fatal(err)
	}
	if err := working.SyncPortable(); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(946684800, 0)
	if err := os.Chtimes(source, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	checkStamp := func() {
		t.Helper()
		info, err := os.Stat(source)
		if err != nil || !info.ModTime().Equal(stamp) {
			t.Fatalf("unchanged database was replaced: info=%v err=%v", info, err)
		}
	}
	if err := working.SyncPortable(); err != nil {
		t.Fatal(err)
	}
	if err := working.Close(); err != nil {
		t.Fatal(err)
	}
	checkStamp()
	next, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.EnsureLibraryMetadata("library", "Library", "#123456"); err != nil {
		t.Fatal(err)
	}
	if err := next.SyncPortable(); err != nil {
		t.Fatal(err)
	}
	checkStamp()
	if err := next.EnsureLibraryMetadata("library", "Renamed", "#123456"); err != nil {
		t.Fatal(err)
	}
	if err := next.SyncPortable(); err != nil {
		t.Fatal(err)
	}
	durable, err := New(source)
	if err != nil {
		t.Fatal(err)
	}
	defer durable.Close()
	if name, err := durable.getMeta("library_name"); err != nil || name != "Renamed" {
		t.Fatalf("changed metadata was not persisted: name=%q err=%v", name, err)
	}
}

func TestPortableFailedFinalSyncRetainsLeaseAndCanRetry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	source := filepath.Join(root, "metadata.sqlite")
	working, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatal(err)
	}
	defer working.Close()
	if err := working.CreateAlbum(&storage.Album{Name: "pending", CreatedBy: 1, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Make the destination unavailable without touching the working database.
	if err := os.Rename(root, root+"-offline"); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(root+"-offline", root)
	if err := os.WriteFile(root, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(root)
	if err := working.Close(); err == nil {
		t.Fatal("expected final persistence to fail")
	}
	if working.portableCopy == nil || working.portableCopy.lease.Ping() != nil {
		t.Fatal("failed close discarded the portable lease")
	}
	if albums, err := working.ListAlbums(1); err != nil || len(albums) != 1 {
		t.Fatalf("failed close discarded the working database: %v %v", albums, err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root+"-offline", root); err != nil {
		t.Fatal(err)
	}
	if err := working.Close(); err != nil {
		t.Fatal(err)
	}
	durable, err := New(source)
	if err != nil {
		t.Fatal(err)
	}
	defer durable.Close()
	if albums, err := durable.ListAlbums(1); err != nil || len(albums) != 1 {
		t.Fatalf("retry did not persist pending writes: %v %v", albums, err)
	}
}

func TestPortableRestartRecoversWorkingCopyAfterPendingSync(t *testing.T) {
	source := filepath.Join(t.TempDir(), ".echogallery", "metadata.sqlite")
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		t.Fatal(err)
	}
	durable, err := New(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := durable.Close(); err != nil {
		t.Fatal(err)
	}
	root, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(source))))[:24]
	dir := filepath.Join(root, "EchoGallery", "database", key)
	cache := filepath.Join(dir, "metadata.sqlite")
	if err := snapshotSQLiteDatabase(source, cache); err != nil {
		t.Fatal(err)
	}
	working, err := New(cache)
	if err != nil {
		t.Fatal(err)
	}
	album := &storage.Album{Name: "recovered bookmark data", CreatedBy: 22, CreatedAt: time.Now()}
	if err := working.CreateAlbum(album); err != nil {
		t.Fatal(err)
	}
	if err := working.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sync-pending"), []byte("pending\n"), 0600); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatalf("restart did not recover pending working copy: %v", err)
	}
	defer restarted.Close()
	albums, err := restarted.ListAlbums(22)
	if err != nil || len(albums) != 1 || albums[0].Name != album.Name {
		t.Fatalf("pending user data was not recovered: albums=%+v err=%v", albums, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sync-pending")); !os.IsNotExist(err) {
		t.Fatalf("recovery marker should be cleared, stat err=%v", err)
	}
}

func TestPortableScheduledSyncPersistsAlbumMutation(t *testing.T) {
	source := filepath.Join(t.TempDir(), ".echogallery", "metadata.sqlite")
	working, err := NewPortableWorkingCopy(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := working.CreateAlbum(&storage.Album{Name: "scheduled", CreatedBy: 31, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := working.SchedulePortableSync(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(working.portableCopy.pending); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduled library sync did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	durable, err := New(source)
	if err != nil {
		t.Fatal(err)
	}
	defer durable.Close()
	albums, err := durable.ListAlbums(31)
	if err != nil || len(albums) != 1 || albums[0].Name != "scheduled" {
		t.Fatalf("scheduled album mutation was not persisted: albums=%+v err=%v", albums, err)
	}
	if err := working.Close(); err != nil {
		t.Fatal(err)
	}
}
