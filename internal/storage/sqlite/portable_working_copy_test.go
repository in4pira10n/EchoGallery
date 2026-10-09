package sqlite

import (
	"errors"
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
