package api

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"echogallery/internal/config"
	imgpkg "echogallery/internal/image"
	"echogallery/internal/sessionlock"
	"echogallery/internal/storage"
	"echogallery/internal/storage/sqlite"
)

func TestRuntimeReleaseEvictsWorkingCopyAndReloads(t *testing.T) {
	root := t.TempDir()
	library := config.Library{ID: "handoff", Name: "Handoff", Path: root}
	cfg := &config.Config{AppDataDir: t.TempDir(), StoragePath: root, Libraries: []config.Library{library}}
	provider, err := NewLibraryRuntimeProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	locks, err := sessionlock.New(filepath.Join(t.TempDir(), "locks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer locks.Close()
	if _, err := locks.Acquire(library, "admin", "admin", "page", "browse"); err != nil {
		t.Fatal(err)
	}
	first, err := provider.ForLibrary(cfg, library)
	if err != nil {
		t.Fatal(err)
	}
	provider.rememberSession("page", library.ID)
	if err := provider.SyncBeforeRelease(locks, "page", ""); err != nil {
		t.Fatal(err)
	}
	if len(provider.repos) != 0 || len(provider.services) != 0 {
		t.Fatal("released runtime retained")
	}
	locks.ReleaseSession("page")
	path, err := cfg.DatabasePathForStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := sqlite.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := durable.CreateAlbum(&storage.Album{Name: "from another Gallery", CreatedBy: 1, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	durable.Close()
	next, err := provider.ForLibrary(cfg, library)
	if err != nil {
		t.Fatal(err)
	}
	if next == first {
		t.Fatal("reused released service")
	}
	albums, err := provider.repos[library.ID].ListAlbums(1)
	if err != nil || len(albums) != 1 {
		t.Fatalf("did not reload latest state: %v %v", albums, err)
	}
}

func TestRuntimeProviderReusesPortableDatabaseAndThumbnails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "moved-library")
	dataRoot := config.LibraryDataRoot(root)
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.New(filepath.Join(dataRoot, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	photo := &storage.Photo{
		UUID: "35bd948d-019e-435d-bf41-01c63dd4d945", OriginalName: "a.jpg",
		MediaKind: storage.MediaKindImage, MimeType: "image/jpeg", SourceRelPath: "a.jpg",
		UploadedBy: 1, TakenAt: time.Now(), UploadedAt: time.Now(),
	}
	if err := db.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	oldThumbnail := imgpkg.ThumbnailShardPath(filepath.Join(dataRoot, "thumbnails", "lib_original"), photo.UUID)
	if err := os.MkdirAll(filepath.Dir(oldThumbnail), 0755); err != nil {
		t.Fatal(err)
	}
	if err := imgpkg.GenerateThumbnail(bytes.NewReader(pngSample(t)), "image/png", oldThumbnail, 64); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		AppDataDir: t.TempDir(), StoragePath: root,
		Libraries: []config.Library{{Name: "Moved", Path: root}},
	}
	provider, err := NewLibraryRuntimeProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.DatabasePathForStorage(root); err != nil {
		t.Fatal(err)
	}
	library := cfg.Libraries[0]
	if library.ID != "lib_original" {
		t.Fatalf("未恢复原资源库 ID: %s", library.ID)
	}
	registrar, err := provider.ForLibrary(cfg, library)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := registrar.GetPhotoByUUIDAny(photo.UUID, 1)
	if err != nil || loaded == nil {
		t.Fatalf("已有数据库媒体不可用: %+v, %v", loaded, err)
	}
	paths := registrar.ThumbnailCandidates(loaded)
	if len(paths) == 0 || paths[0] != config.NormalizeStoragePath(oldThumbnail) {
		t.Fatalf("未使用资源库原有缩略图: %+v", paths)
	}
	if resolved := resolveExistingThumbnailCandidates(paths...); resolved != paths[0] {
		t.Fatalf("页面无法读取资源库现有缩略图: %s", resolved)
	}
}

func TestRuntimeProviderRecordsNewLibraryIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new-library")
	library := config.Library{ID: "lib_custom", Name: "New", Path: root}
	cfg := &config.Config{
		AppDataDir: t.TempDir(), StoragePath: root,
		Libraries: []config.Library{library},
	}
	provider, err := NewLibraryRuntimeProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ForLibrary(cfg, library); err != nil {
		t.Fatal(err)
	}
	path, err := cfg.DatabasePathForStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id string
	if err := db.QueryRow(`SELECT value FROM app_meta WHERE key = 'library_id'`).Scan(&id); err != nil || id != library.ID {
		t.Fatalf("新资源库身份未写入数据库: id=%s err=%v", id, err)
	}
}
