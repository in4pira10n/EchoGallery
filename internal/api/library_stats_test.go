package api

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestLibraryStatsExcludeEchoGalleryInternalPaths(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "stats.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE photos (
		uploaded_at TEXT, deleted_at TEXT, size INTEGER, media_kind TEXT,
		original_name TEXT, source_rel_path TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO photos(uploaded_at, size, media_kind, original_name, source_rel_path) VALUES
		('2020-01-01T00:00:00Z', 900, 'image', 'internal.jpg', '.echogallery/internal.jpg'),
		('2025-01-01T00:00:00Z', 100, 'image', 'photo.jpg', 'Photos/photo.jpg'),
		('2025-02-01T00:00:00Z', 200, 'video', 'clip.wmv', 'Videos/clip.wmv'),
		('2025-03-01T00:00:00Z', 75, 'image', 'uploaded.jpg', '')`)
	if err != nil {
		t.Fatal(err)
	}

	totalSize, photos, videos, unsupported, ok := readLibraryMediaStats(db)
	if !ok {
		t.Fatal("failed to read media stats")
	}
	if totalSize != 375 || photos != 2 || videos != 1 || unsupported != 1 {
		t.Fatalf("internal directory was included in stats: size=%d photos=%d videos=%d unsupported=%d", totalSize, photos, videos, unsupported)
	}
	createdAt, ok := readLibraryCreatedAt(db)
	if !ok || createdAt.Year() != 2025 {
		t.Fatalf("internal directory affected created-at stat: %v, ok=%v", createdAt, ok)
	}
}
