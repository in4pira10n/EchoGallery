package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"echogallery/internal/storage"

	_ "modernc.org/sqlite"
)

// newTestDB 创建测试用内存数据库
func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// makePhoto 创建测试用 Photo
func makePhoto(userID int64, takenAt time.Time) *storage.Photo {
	return &storage.Photo{
		UUID:         "uuid-" + takenAt.Format("20060102150405"),
		OriginalName: "test.jpg",
		MediaKind:    storage.MediaKindImage,
		MimeType:     "image/jpeg",
		Size:         1024,
		Width:        800,
		Height:       600,
		DurationMS:   0,
		TakenAt:      takenAt,
		UploadedAt:   time.Now(),
		UploadedBy:   userID,
	}
}

// --- DB 初始化测试 ---

func TestNew_CreatesSchema(t *testing.T) {
	db := newTestDB(t)
	// 验证表存在
	tables := []string{"photos", "albums", "album_photos", "share_links"}
	for _, table := range tables {
		var name string
		err := db.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("表 %s 不存在: %v", table, err)
		}
	}
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := New("/nonexistent/path/test.db")
	if err == nil {
		t.Error("无效路径应该返回错误")
	}
}

func TestNew_MigratesLegacyPhotoColumns(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("创建旧数据库失败: %v", err)
	}
	_, err = raw.Exec(`
		CREATE TABLE photos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE,
			original_name TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			taken_at DATETIME NOT NULL,
			uploaded_at DATETIME NOT NULL,
			uploaded_by INTEGER NOT NULL,
			deleted_at DATETIME,
			deleted_by INTEGER
		);
		CREATE TABLE albums (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', cover_photo_id INTEGER, created_by INTEGER NOT NULL, created_at DATETIME NOT NULL);
		CREATE TABLE album_photos (album_id INTEGER NOT NULL, photo_id INTEGER NOT NULL, added_at DATETIME NOT NULL, PRIMARY KEY (album_id, photo_id));
		CREATE TABLE share_links (id INTEGER PRIMARY KEY AUTOINCREMENT, token TEXT NOT NULL UNIQUE, type TEXT NOT NULL, target_id INTEGER NOT NULL, created_by INTEGER NOT NULL, expires_at DATETIME, created_at DATETIME NOT NULL);
	`)
	if err != nil {
		raw.Close()
		t.Fatalf("初始化旧 schema 失败: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("关闭旧数据库失败: %v", err)
	}

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("迁移旧数据库失败: %v", err)
	}
	defer db.Close()

	for _, column := range []string{"media_kind", "duration_ms", "storage_rel_path", "source_rel_path"} {
		var found bool
		rows, err := db.db.Query(`PRAGMA table_info(photos)`)
		if err != nil {
			t.Fatalf("查询表结构失败: %v", err)
		}
		for rows.Next() {
			var cid int
			var name string
			var dataType string
			var notNull int
			var defaultValue any
			var pk int
			if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
				rows.Close()
				t.Fatalf("扫描表结构失败: %v", err)
			}
			if name == column {
				found = true
			}
		}
		rows.Close()
		if !found {
			t.Fatalf("迁移后缺少列 %s", column)
		}
	}
}

func TestNew_MigratesLegacyVideoPlaybackPreferencesToShared(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy-playback.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("创建旧数据库失败: %v", err)
	}
	_, err = raw.Exec(`
		CREATE TABLE photos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE,
			original_name TEXT NOT NULL,
			media_kind TEXT NOT NULL DEFAULT 'video',
			mime_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			storage_rel_path TEXT NOT NULL DEFAULT '',
			source_rel_path TEXT NOT NULL DEFAULT '',
			exif_json TEXT NOT NULL DEFAULT '',
			source_mod_unix INTEGER NOT NULL DEFAULT 0,
			random_sort_key INTEGER NOT NULL DEFAULT 0,
			is_favorite INTEGER NOT NULL DEFAULT 0,
			is_super_favorite INTEGER NOT NULL DEFAULT 0,
			taken_at DATETIME NOT NULL,
			uploaded_at DATETIME NOT NULL,
			uploaded_by INTEGER NOT NULL,
			deleted_at DATETIME,
			deleted_by INTEGER
		);
		CREATE TABLE albums (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', cover_photo_id INTEGER, source_kind TEXT NOT NULL DEFAULT '', source_rel_path TEXT NOT NULL DEFAULT '', created_by INTEGER NOT NULL, created_at DATETIME NOT NULL);
		CREATE TABLE album_photos (album_id INTEGER NOT NULL, photo_id INTEGER NOT NULL, added_at DATETIME NOT NULL, PRIMARY KEY (album_id, photo_id));
		CREATE TABLE share_links (id INTEGER PRIMARY KEY AUTOINCREMENT, token TEXT NOT NULL UNIQUE, type TEXT NOT NULL, target_id INTEGER NOT NULL, created_by INTEGER NOT NULL, expires_at DATETIME, created_at DATETIME NOT NULL);
		CREATE TABLE video_playback_preferences (
			photo_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			volume REAL NOT NULL DEFAULT 1,
			muted INTEGER NOT NULL DEFAULT 0,
			resume_time INTEGER NOT NULL DEFAULT 0,
			bookmarks_json TEXT NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (photo_id, user_id)
		);
		INSERT INTO photos (id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms, taken_at, uploaded_at, uploaded_by)
		VALUES (1, 'video-legacy', 'clip.mp4', 'video', 'video/mp4', 1024, 1920, 1080, 60000, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 1);
		INSERT INTO video_playback_preferences (photo_id, user_id, volume, muted, resume_time, bookmarks_json, updated_at)
		VALUES
			(1, 1, 0.4, 0, 12, '[{"slot":1,"time":12}]', '2026-01-01T00:00:00Z'),
			(1, 2, 0.7, 1, 34, '[{"slot":1,"time":12},{"slot":2,"time":34}]', '2026-01-02T00:00:00Z');
	`)
	if err != nil {
		raw.Close()
		t.Fatalf("初始化旧播放偏好 schema 失败: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("关闭旧数据库失败: %v", err)
	}

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("迁移旧播放偏好数据库失败: %v", err)
	}
	defer db.Close()

	pref, err := db.GetVideoPlaybackPreference(1, 1)
	if err != nil {
		t.Fatalf("读取迁移后的播放偏好失败: %v", err)
	}
	if pref == nil || pref.ResumeTime != 34 || !pref.Muted || len(pref.Bookmarks) != 2 {
		t.Fatalf("期望保留最新共享播放偏好，得到 %+v", pref)
	}
}

func TestListPhotos_AttachesSharedVideoPlaybackMetadata(t *testing.T) {
	db := newTestDB(t)
	photo := &storage.Photo{
		UUID:         "video-grid-1",
		OriginalName: "clip.mp4",
		MediaKind:    storage.MediaKindVideo,
		MimeType:     "video/mp4",
		Size:         1024,
		Width:        1920,
		Height:       1080,
		DurationMS:   60000,
		TakenAt:      time.Now(),
		UploadedAt:   time.Now(),
		UploadedBy:   1,
	}
	if err := db.SavePhoto(photo); err != nil {
		t.Fatalf("保存视频失败: %v", err)
	}
	if _, err := db.UpsertVideoPlaybackPreference(photo.ID, 99, 0.6, true, 45, []storage.VideoPlaybackBookmark{{Slot: 1, Time: 12}, {Slot: 2, Time: 34}}); err != nil {
		t.Fatalf("保存共享播放偏好失败: %v", err)
	}

	page, err := db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("读取时间线失败: %v", err)
	}
	if len(page.Photos) != 1 {
		t.Fatalf("期望 1 条媒体，得到 %d", len(page.Photos))
	}
	got := page.Photos[0]
	if got.VideoBookmarkCount != 2 || got.VideoResumeTime != 45 {
		t.Fatalf("期望列表附带共享书签与续播信息，得到 count=%d resume=%d", got.VideoBookmarkCount, got.VideoResumeTime)
	}
}

// --- Photo 测试 ---

func TestSavePhoto_Success(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())

	if err := db.SavePhoto(p); err != nil {
		t.Fatalf("保存图片失败: %v", err)
	}
	if p.ID == 0 {
		t.Error("保存后 ID 应该被填充")
	}
}

func TestSavePhoto_VideoFieldsPersisted(t *testing.T) {
	db := newTestDB(t)
	p := &storage.Photo{
		UUID:         "video-uuid",
		OriginalName: "demo.mp4",
		MediaKind:    storage.MediaKindVideo,
		MimeType:     "video/mp4",
		Size:         2048,
		Width:        1920,
		Height:       1080,
		DurationMS:   12345,
		EXIF:         &storage.PhotoEXIF{VideoCodec: "h264", VideoFrameRate: 29.97},
		TakenAt:      time.Now(),
		UploadedAt:   time.Now(),
		UploadedBy:   1,
	}

	if err := db.SavePhoto(p); err != nil {
		t.Fatalf("保存视频失败: %v", err)
	}
	got, err := db.GetPhotoByID(p.ID, 1)
	if err != nil {
		t.Fatalf("查询视频失败: %v", err)
	}
	if got == nil {
		t.Fatal("应能查询到视频记录")
	}
	if got.MediaKind != storage.MediaKindVideo || got.DurationMS != 12345 {
		t.Fatalf("视频字段未正确持久化: %+v", got)
	}
	if got.EXIF == nil || got.EXIF.VideoCodec != "h264" || got.EXIF.VideoFrameRate != 29.97 {
		t.Fatalf("视频元数据未正确持久化: %+v", got.EXIF)
	}
}

func TestSavePhoto_DuplicateUUID(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())
	db.SavePhoto(p)

	p2 := makePhoto(1, time.Now())
	p2.UUID = p.UUID
	if err := db.SavePhoto(p2); err == nil {
		t.Error("重复 UUID 应该返回错误")
	}
}

func TestGetPhotoByID_Success(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())
	db.SavePhoto(p)

	got, err := db.GetPhotoByID(p.ID, 1)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if got == nil || got.UUID != p.UUID {
		t.Errorf("查询结果不匹配")
	}
}

func TestGetPhotoByID_WrongUser(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())
	db.SavePhoto(p)

	got, err := db.GetPhotoByID(p.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("不同用户不应该能查到图片")
	}
}

func TestGetPhotoByUUID_Success(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())
	db.SavePhoto(p)

	got, err := db.GetPhotoByUUID(p.UUID, 1)
	if err != nil || got == nil {
		t.Fatalf("按 UUID 查询失败: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("期望 ID=%d，得到 %d", p.ID, got.ID)
	}
}

func TestGetPhotoByUUIDAny_IncludesDeleted(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())
	db.SavePhoto(p)
	db.SoftDeletePhoto(p.ID, 1, 1)

	// 普通查询不到
	got, _ := db.GetPhotoByUUID(p.UUID, 1)
	if got != nil {
		t.Error("软删除后 GetPhotoByUUID 不应该返回")
	}

	// Any 查询得到
	got, err := db.GetPhotoByUUIDAny(p.UUID, 1)
	if err != nil || got == nil {
		t.Fatalf("GetPhotoByUUIDAny 应该返回软删除图片: %v", err)
	}
	if got.DeletedAt == nil {
		t.Error("DeletedAt 应该不为 nil")
	}
}

func TestListPhotos_Pagination(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 插入 5 张图片
	for i := 0; i < 5; i++ {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = filepath.Join("uuid", string(rune('a'+i)))
		db.SavePhoto(p)
	}

	// 第一页，limit=3
	page1, err := db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 3})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(page1.Photos) != 3 {
		t.Errorf("期望 3 张，得到 %d", len(page1.Photos))
	}
	if !page1.HasMore {
		t.Error("应该有更多")
	}

	// 第二页
	page2, err := db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 3, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("查询第二页失败: %v", err)
	}
	if len(page2.Photos) != 2 {
		t.Errorf("期望 2 张，得到 %d", len(page2.Photos))
	}
	if page2.HasMore {
		t.Error("不应该有更多")
	}
}

func TestListPhotos_ReverseOrder(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 4; i++ {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = fmt.Sprintf("reverse-%d", i)
		if err := db.SavePhoto(p); err != nil {
			t.Fatalf("保存测试图片失败: %v", err)
		}
	}

	page, err := db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 4, Reverse: true})
	if err != nil {
		t.Fatalf("倒序查询失败: %v", err)
	}
	if len(page.Photos) != 4 {
		t.Fatalf("期望 4 张，得到 %d", len(page.Photos))
	}
	for i := 1; i < len(page.Photos); i++ {
		if page.Photos[i-1].TakenAt.After(page.Photos[i].TakenAt) {
			t.Fatalf("期望按时间正序返回，%v 在 %v 之后", page.Photos[i-1].TakenAt, page.Photos[i].TakenAt)
		}
	}
}

func TestListRandomPhotos_PaginatesWithoutLoadingAll(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = fmt.Sprintf("random-%d", i)
		p.RandomSortKey = int64(i + 1)
		if err := db.SavePhoto(p); err != nil {
			t.Fatalf("保存测试图片失败: %v", err)
		}
	}

	var got []int64
	cursor := ""
	for {
		page, err := db.ListRandomPhotos(storage.RandomPhotosParams{UserID: 1, Seed: 5, Cursor: cursor, Limit: 3, SkipTotal: cursor != ""})
		if err != nil {
			t.Fatalf("乱序分页失败: %v", err)
		}
		for _, p := range page.Photos {
			got = append(got, p.RandomSortKey)
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}

	want := []int64{5, 6, 7, 1, 2, 3, 4}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("乱序分页顺序错误，得到 %v，期望 %v", got, want)
	}
}

func TestListPhotos_UserIsolation(t *testing.T) {
	db := newTestDB(t)
	db.SavePhoto(makePhoto(1, time.Now()))
	db.SavePhoto(makePhoto(2, time.Now()))

	page, _ := db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(page.Photos) != 1 {
		t.Errorf("用户隔离失败，期望 1 张，得到 %d", len(page.Photos))
	}
}

func TestSoftDeletePhoto_And_Restore(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())
	db.SavePhoto(p)

	// 软删除
	if err := db.SoftDeletePhoto(p.ID, 1, 1); err != nil {
		t.Fatalf("软删除失败: %v", err)
	}

	// 正常查询应该查不到
	got, _ := db.GetPhotoByID(p.ID, 1)
	if got != nil {
		t.Error("软删除后不应该在正常查询中出现")
	}

	// 回收站应该能查到
	trashed, _ := db.ListTrashedPhotos(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(trashed.Photos) != 1 {
		t.Error("软删除后应该在回收站中出现")
	}

	// 恢复
	if err := db.RestorePhoto(p.ID, 1); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	got, _ = db.GetPhotoByID(p.ID, 1)
	if got == nil {
		t.Error("恢复后应该能正常查到")
	}
}

func TestSearchPhotos_FiltersAndPaginates(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	items := []*storage.Photo{
		makePhoto(1, base.Add(3*time.Hour)),
		makePhoto(1, base.Add(2*time.Hour)),
		makePhoto(1, base.Add(1*time.Hour)),
		makePhoto(2, base.Add(4*time.Hour)),
	}
	items[0].UUID = "search-photo"
	items[0].OriginalName = "Mountain Sunrise.jpg"
	items[1].UUID = "search-video"
	items[1].OriginalName = "Family Movie.mp4"
	items[1].MediaKind = storage.MediaKindVideo
	items[1].MimeType = "video/mp4"
	items[2].UUID = "deleted-photo"
	items[2].OriginalName = "Mountain Deleted.jpg"
	items[3].UUID = "other-user"
	items[3].OriginalName = "Mountain Private.jpg"
	for _, photo := range items {
		if err := db.SavePhoto(photo); err != nil {
			t.Fatalf("保存媒体失败: %v", err)
		}
	}
	if err := db.SoftDeletePhoto(items[2].ID, 1, 1); err != nil {
		t.Fatalf("软删除媒体失败: %v", err)
	}

	page, err := db.SearchPhotos(storage.SearchPhotosParams{UserID: 1, Query: "mountain", Limit: 1, IncludeTotal: true})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(page.Photos) != 1 || page.HasMore || page.Total != 1 {
		t.Fatalf("期望仅返回未删除且归属当前用户的 1 条结果，got len=%d hasMore=%v total=%d", len(page.Photos), page.HasMore, page.Total)
	}
	if page.Photos[0].ID != items[0].ID {
		t.Fatalf("返回了错误的媒体 ID: %d", page.Photos[0].ID)
	}

	fastPage, err := db.SearchPhotos(storage.SearchPhotosParams{UserID: 1, Query: "mountain", Limit: 1})
	if err != nil {
		t.Fatalf("快速搜索失败: %v", err)
	}
	if fastPage.Total != 0 {
		t.Fatalf("快速搜索不应强制统计总数，得到 total=%d", fastPage.Total)
	}

	videoPage, err := db.SearchPhotos(storage.SearchPhotosParams{UserID: 1, Query: "video", Limit: 10})
	if err != nil {
		t.Fatalf("按类型搜索失败: %v", err)
	}
	if len(videoPage.Photos) != 1 || videoPage.Photos[0].ID != items[1].ID {
		t.Fatalf("期望按 mime/media kind 命中视频，得到 %d 条", len(videoPage.Photos))
	}
}

func TestHardDeleteTrashedPhotos(t *testing.T) {
	db := newTestDB(t)
	p1 := makePhoto(1, time.Now())
	p2 := &storage.Photo{UUID: "uuid-2", OriginalName: "b.jpg", MediaKind: storage.MediaKindImage, MimeType: "image/jpeg", Size: 512, TakenAt: time.Now(), UploadedAt: time.Now(), UploadedBy: 1}
	db.SavePhoto(p1)
	db.SavePhoto(p2)
	db.SoftDeletePhoto(p1.ID, 1, 1)
	db.SoftDeletePhoto(p2.ID, 1, 1)

	photos, err := db.HardDeleteTrashedPhotos(1)
	if err != nil {
		t.Fatalf("清空回收站失败: %v", err)
	}
	if len(photos) != 2 {
		t.Errorf("期望 2 条记录，得到 %d", len(photos))
	}

	trashed, _ := db.ListTrashedPhotos(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(trashed.Photos) != 0 {
		t.Error("清空后回收站应该为空")
	}
}

func TestSavePhoto_ConcurrentWritesDoNotBusy(t *testing.T) {
	db := newTestDB(t)
	const n = 20

	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := makePhoto(1, time.Now().Add(time.Duration(i)*time.Second))
			p.UUID = fmt.Sprintf("concurrent-%d", i)
			if err := db.SavePhoto(p); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("并发写入不应失败，得到错误: %v", err)
	}

	page, err := db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 100})
	if err != nil {
		t.Fatalf("查询并发写入结果失败: %v", err)
	}
	if len(page.Photos) != n {
		t.Fatalf("期望保存 %d 张图片，实际 %d 张", n, len(page.Photos))
	}
}

// --- 游标编解码测试 ---

func TestCursorEncodeDecode(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	encoded := encodeCursor(now, 42)
	c, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if c.ID != 42 {
		t.Errorf("期望 ID=42，得到 %d", c.ID)
	}
	if !c.TakenAt.Equal(now) {
		t.Errorf("时间不匹配")
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	_, err := decodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("无效游标应该返回错误")
	}
}

func TestListPhotos_PaginationManySameTimestamp(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 200; i++ {
		p := makePhoto(1, base)
		p.UUID = fmt.Sprintf("same-ts-%03d", i)
		if err := db.SavePhoto(p); err != nil {
			t.Fatalf("保存第 %d 张图片失败: %v", i, err)
		}
	}

	count := 0
	cursor := ""
	for {
		page, err := db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 30, Cursor: cursor})
		if err != nil {
			t.Fatalf("分页查询失败: %v", err)
		}
		count += len(page.Photos)
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" {
			t.Fatal("HasMore=true 时 NextCursor 不应为空")
		}
		cursor = page.NextCursor
	}

	if count != 200 {
		t.Fatalf("期望遍历 200 张图片，实际 %d 张", count)
	}
}

func TestListTrashedPhotos_PaginationUsesDeletedAtCursor(t *testing.T) {
	db := newTestDB(t)
	baseTakenAt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 80; i++ {
		p := makePhoto(1, baseTakenAt.Add(time.Duration(i)*time.Hour))
		p.UUID = fmt.Sprintf("trash-%03d", i)
		if err := db.SavePhoto(p); err != nil {
			t.Fatalf("保存第 %d 张图片失败: %v", i, err)
		}
		if err := db.SoftDeletePhoto(p.ID, 1, 1); err != nil {
			t.Fatalf("软删除第 %d 张图片失败: %v", i, err)
		}
	}

	count := 0
	cursor := ""
	for {
		page, err := db.ListTrashedPhotos(storage.ListPhotosParams{UserID: 1, Limit: 30, Cursor: cursor})
		if err != nil {
			t.Fatalf("回收站分页查询失败: %v", err)
		}
		count += len(page.Photos)
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" {
			t.Fatal("HasMore=true 时 NextCursor 不应为空")
		}
		cursor = page.NextCursor
	}

	if count != 80 {
		t.Fatalf("期望遍历 80 张回收站图片，实际 %d 张", count)
	}
}

// 确保测试文件不依赖 os 包报错
var _ = os.DevNull
