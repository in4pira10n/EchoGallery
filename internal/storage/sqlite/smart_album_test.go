package sqlite

import (
	"fmt"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestSmartAlbumRulesQueryAndPaginateDynamically(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	addMedia := func(uuid, name, kind string, durationMS int64, age time.Duration) *storage.Photo {
		t.Helper()
		photo := makePhoto(7, now.Add(-age))
		photo.UUID = uuid
		photo.OriginalName = name
		photo.MediaKind = kind
		photo.MimeType = "image/jpeg"
		if kind == storage.MediaKindVideo {
			photo.MimeType = "video/mp4"
		}
		photo.DurationMS = durationMS
		if err := db.SavePhoto(photo); err != nil {
			t.Fatalf("save %s: %v", uuid, err)
		}
		return photo
	}

	addMedia("smart-match-1", "clip 001.mp4", storage.MediaKindVideo, 9*60*1000, 3*time.Hour)
	addMedia("smart-match-2", "clip 002.mp4", storage.MediaKindVideo, 9*60*1000, 2*time.Hour)
	addMedia("smart-match-3", "clip 003.mp4", storage.MediaKindVideo, 9*60*1000, time.Hour)
	addMedia("smart-too-long", "clip 004.mp4", storage.MediaKindVideo, 11*60*1000, 30*time.Minute)
	addMedia("smart-image", "clip still.jpg", storage.MediaKindImage, 0, 10*time.Minute)

	album := &storage.Album{
		Name:           "8 到 10 分钟片段",
		Description:    "匹配的视频摘要",
		SourceKind:     "smart",
		SmartMatchMode: "all",
		SmartRules: []storage.SmartAlbumRule{
			{Field: "media_type", Operator: "is", Value: "video"},
			{Field: "duration", Operator: "between", Min: "8:00", Max: "10:00"},
			{Field: "filename", Operator: "contains", Value: "clip"},
		},
		CreatedBy: 7,
		CreatedAt: now,
	}
	if err := db.CreateAlbum(album); err != nil {
		t.Fatalf("create smart album: %v", err)
	}

	listed, err := db.ListAlbums(7)
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}
	if len(listed) != 1 || listed[0].PhotoCount != 3 || listed[0].CoverUUID != "smart-match-3" {
		t.Fatalf("smart card stats not computed from rules: %+v", listed)
	}
	if len(listed[0].SmartRules) != 3 || listed[0].SmartMatchMode != "all" || listed[0].Description != album.Description {
		t.Fatalf("smart definition did not persist: %+v", listed[0])
	}

	first, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{AlbumID: album.ID, UserID: 7, Limit: 2})
	if err != nil {
		t.Fatalf("read first smart page: %v", err)
	}
	if len(first.Photos) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	second, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{AlbumID: album.ID, UserID: 7, Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("read second smart page: %v", err)
	}
	if len(second.Photos) != 1 || second.HasMore {
		t.Fatalf("unexpected second page: %+v", second)
	}
	for _, page := range []*storage.PhotoPage{first, second} {
		for _, photo := range page.Photos {
			if photo.MediaKind != storage.MediaKindVideo || photo.DurationMS < 8*60*1000 || photo.DurationMS > 10*60*1000 {
				t.Errorf("non-matching photo leaked into smart album: %+v", photo)
			}
		}
	}

	albumDetails, err := db.GetAlbumByID(album.ID, 7)
	if err != nil || albumDetails == nil || albumDetails.PhotoCount != 3 {
		t.Fatalf("get smart album details: album=%+v err=%v", albumDetails, err)
	}
	matchedPhoto, err := db.GetPhotoByUUID("smart-match-1", 7)
	if err != nil || matchedPhoto == nil {
		t.Fatalf("get matching media: photo=%+v err=%v", matchedPhoto, err)
	}
	belongs, err := db.IsMediaInAlbum(album.ID, matchedPhoto.ID, 7)
	if err != nil || !belongs {
		t.Fatalf("smart membership check rejected a match: belongs=%v err=%v", belongs, err)
	}
	belongs, err = db.IsMediaInAlbum(album.ID, matchedPhoto.ID, 8)
	if err != nil || belongs {
		t.Fatalf("smart membership check crossed user boundary: belongs=%v err=%v", belongs, err)
	}
	photoAlbums, err := db.ListAlbumsForPhoto(matchedPhoto.ID, 7)
	if err != nil || len(photoAlbums) != 1 || photoAlbums[0].ID != album.ID {
		t.Fatalf("matching smart album should appear in media album menu: albums=%+v err=%v", photoAlbums, err)
	}
	nonMatchingPhoto, err := db.GetPhotoByUUID("smart-image", 7)
	if err != nil || nonMatchingPhoto == nil {
		t.Fatalf("get non-matching media: photo=%+v err=%v", nonMatchingPhoto, err)
	}
	photoAlbums, err = db.ListAlbumsForPhoto(nonMatchingPhoto.ID, 7)
	if err != nil || len(photoAlbums) != 0 {
		t.Fatalf("non-matching smart album should not appear in media album menu: albums=%+v err=%v", photoAlbums, err)
	}
	addMedia("smart-added-later", "clip added later.mp4", storage.MediaKindVideo, 8*60*1000, -time.Hour)
	listed, err = db.ListAlbums(7)
	if err != nil || listed[0].PhotoCount != 4 || listed[0].CoverUUID != "smart-added-later" {
		t.Fatalf("smart membership should update when new media is indexed: albums=%+v err=%v", listed, err)
	}
	if _, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{AlbumID: album.ID, UserID: 8, Limit: 10}); err != nil {
		t.Fatalf("querying another user's album should safely return empty: %v", err)
	}
	if err := db.AddPhotoToAlbum(album.ID, first.Photos[0].ID, 7); err == nil {
		t.Fatal("smart album must reject manual membership")
	}
	if err := db.RemovePhotoFromAlbum(album.ID, first.Photos[0].ID, 7); err == nil {
		t.Fatal("smart album must reject manual removal")
	}
	album.Name = "已更新的智能相册"
	album.Description = "编辑后的简介"
	album.SmartMatchMode = "any"
	album.SmartRules = []storage.SmartAlbumRule{{Field: "filename", Operator: "contains", Value: "still"}}
	if err := db.UpdateAlbum(album); err != nil {
		t.Fatalf("update smart album definition: %v", err)
	}
	updated, err := db.GetAlbumByID(album.ID, 7)
	if err != nil || updated == nil || updated.Name != album.Name || updated.Description != album.Description || updated.SmartMatchMode != "any" || updated.PhotoCount != 1 || updated.CoverUUID != "smart-image" {
		t.Fatalf("updated definition was not persisted and evaluated: album=%+v err=%v", updated, err)
	}
}

func TestSmartAlbumAnyModeAndFilenameOperators(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	for i, name := range []string{"keep-this.jpg", "clip-video.mp4", "exclude-this.jpg"} {
		photo := makePhoto(9, now.Add(-time.Duration(i)*time.Minute))
		photo.UUID = fmt.Sprintf("smart-any-%d", i)
		photo.OriginalName = name
		if i == 1 {
			photo.MediaKind = storage.MediaKindVideo
			photo.MimeType = "video/mp4"
			photo.DurationMS = 9 * 60 * 1000
		}
		if err := db.SavePhoto(photo); err != nil {
			t.Fatalf("save test media: %v", err)
		}
	}
	album := &storage.Album{
		Name:           "名称或时长",
		SourceKind:     "smart",
		SmartMatchMode: "any",
		SmartRules: []storage.SmartAlbumRule{
			{Field: "filename", Operator: "starts_with", Value: "keep"},
			{Field: "duration", Operator: "at_least", Min: "8:00"},
			{Field: "filename", Operator: "not_contains", Value: "exclude"},
		},
		CreatedBy: 9,
		CreatedAt: now,
	}
	if err := db.CreateAlbum(album); err != nil {
		t.Fatalf("create smart album: %v", err)
	}
	page, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{AlbumID: album.ID, UserID: 9, Limit: 10})
	if err != nil {
		t.Fatalf("query any-mode smart album: %v", err)
	}
	if len(page.Photos) != 2 {
		t.Fatalf("any-mode rules should return the matching prefix and long video, got %d", len(page.Photos))
	}
}
