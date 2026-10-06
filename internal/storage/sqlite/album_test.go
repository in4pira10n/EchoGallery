package sqlite

import (
	"fmt"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestCreateAlbum_Success(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{
		Name:      "旅行",
		CreatedBy: 1,
		CreatedAt: time.Now(),
	}
	if err := db.CreateAlbum(album); err != nil {
		t.Fatalf("创建相册失败: %v", err)
	}
	if album.ID == 0 {
		t.Error("创建后 ID 应该被填充")
	}
}

func TestGetAlbumByID_Success(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "测试相册", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	got, err := db.GetAlbumByID(album.ID, 1)
	if err != nil || got == nil {
		t.Fatalf("查询相册失败: %v", err)
	}
	if got.Name != "测试相册" {
		t.Errorf("相��名不匹配")
	}
}

func TestGetAlbumByID_WrongUser(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "私有相册", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	got, _ := db.GetAlbumByID(album.ID, 2)
	if got != nil {
		t.Error("不同用户不应该查到相册")
	}
}

func TestListAlbums_Success(t *testing.T) {
	db := newTestDB(t)
	db.CreateAlbum(&storage.Album{Name: "A", CreatedBy: 1, CreatedAt: time.Now()})
	db.CreateAlbum(&storage.Album{Name: "B", CreatedBy: 1, CreatedAt: time.Now()})
	db.CreateAlbum(&storage.Album{Name: "C", CreatedBy: 2, CreatedAt: time.Now()})

	albums, err := db.ListAlbums(1)
	if err != nil {
		t.Fatalf("查��失败: %v", err)
	}
	if len(albums) != 2 {
		t.Errorf("期望 2 个相册，得到 %d", len(albums))
	}
}

func TestListAlbumsForPhoto_Success(t *testing.T) {
	db := newTestDB(t)
	album1 := &storage.Album{Name: "A", CreatedBy: 1, CreatedAt: time.Now()}
	album2 := &storage.Album{Name: "B", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album1)
	db.CreateAlbum(album2)
	photo := makePhoto(1, time.Now())
	photo.UUID = "album-photo-in-two"
	photo.OriginalName = "in-albums.jpg"
	if err := db.SavePhoto(photo); err != nil {
		t.Fatalf("保存照片失败: %v", err)
	}
	if err := db.AddPhotoToAlbum(album1.ID, photo.ID, 1); err != nil {
		t.Fatalf("添加到相册 A 失败: %v", err)
	}
	if err := db.AddPhotoToAlbum(album2.ID, photo.ID, 1); err != nil {
		t.Fatalf("添加到相册 B 失败: %v", err)
	}

	albums, err := db.ListAlbumsForPhoto(photo.ID, 1)
	if err != nil {
		t.Fatalf("查询媒体所在相册失败: %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("期望 2 个相册，得到 %d", len(albums))
	}
}

func TestListAlbumsForPhoto_UsesCurrentSourceRelPathAfterMove(t *testing.T) {
	db := newTestDB(t)
	oldAlbum := &storage.Album{
		Name:          "old",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "old",
		CreatedBy:     1,
		CreatedAt:     time.Now().Add(-time.Hour),
	}
	newAlbum := &storage.Album{
		Name:          "new",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "new",
		CreatedBy:     1,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateAlbum(oldAlbum); err != nil {
		t.Fatalf("创建旧相册失败: %v", err)
	}
	if err := db.CreateAlbum(newAlbum); err != nil {
		t.Fatalf("创建新相册失败: %v", err)
	}
	photo := makePhoto(1, time.Now())
	photo.UUID = "moved-video"
	photo.MediaKind = storage.MediaKindVideo
	photo.MimeType = "video/mp4"
	photo.SourceRelPath = "new/video.mp4"
	if err := db.SavePhoto(photo); err != nil {
		t.Fatalf("保存移动后媒体失败: %v", err)
	}
	if err := db.AddPhotoToAlbum(oldAlbum.ID, photo.ID, 1); err != nil {
		t.Fatalf("添加旧相册残留关系失败: %v", err)
	}

	albums, err := db.ListAlbumsForPhoto(photo.ID, 1)
	if err != nil {
		t.Fatalf("查询媒体所在相册失败: %v", err)
	}
	if len(albums) != 1 || albums[0].ID != newAlbum.ID {
		t.Fatalf("期望按当前 source_rel_path 定位到新相册，得到 %#v", albums)
	}
}

func TestListAlbumsForPhoto_LegacyFolderAlbumUsesNameAsPath(t *testing.T) {
	db := newTestDB(t)
	legacyAlbum := &storage.Album{
		Name:        "legacy/folder",
		Description: "自动从文件夹导入",
		CreatedBy:   1,
		CreatedAt:   time.Now(),
	}
	if err := db.CreateAlbum(legacyAlbum); err != nil {
		t.Fatalf("创建旧版文件夹相册失败: %v", err)
	}
	photo := makePhoto(1, time.Now())
	photo.UUID = "legacy-folder-photo"
	photo.SourceRelPath = "legacy/folder/photo.jpg"
	if err := db.SavePhoto(photo); err != nil {
		t.Fatalf("保存旧版相册媒体失败: %v", err)
	}

	albums, err := db.ListAlbumsForPhoto(photo.ID, 1)
	if err != nil {
		t.Fatalf("查询媒体所在旧版相册失败: %v", err)
	}
	if len(albums) != 1 || albums[0].ID != legacyAlbum.ID {
		t.Fatalf("期望旧版文件夹相册按 name 定位，得到 %#v", albums)
	}
}

func TestListAlbumPhotos_FolderAlbumUsesCurrentSourceRelPath(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{
		Name:          "new",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "new",
		CreatedBy:     1,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateAlbum(album); err != nil {
		t.Fatalf("创建文件夹相册失败: %v", err)
	}
	photo := makePhoto(1, time.Now())
	photo.UUID = "moved-video-in-folder"
	photo.MediaKind = storage.MediaKindVideo
	photo.MimeType = "video/mp4"
	photo.SourceRelPath = "new/video.mp4"
	if err := db.SavePhoto(photo); err != nil {
		t.Fatalf("保存移动后媒体失败: %v", err)
	}
	nested := makePhoto(1, time.Now().Add(time.Minute))
	nested.UUID = "nested-video"
	nested.MediaKind = storage.MediaKindVideo
	nested.MimeType = "video/mp4"
	nested.SourceRelPath = "new/child/video.mp4"
	if err := db.SavePhoto(nested); err != nil {
		t.Fatalf("保存下级媒体失败: %v", err)
	}

	page, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{AlbumID: album.ID, UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查询文件夹相册媒体失败: %v", err)
	}
	if len(page.Photos) != 1 || page.Photos[0].ID != photo.ID {
		t.Fatalf("期望只按当前文件夹 source_rel_path 返回直接媒体，得到 %#v", page.Photos)
	}
}

func TestListAlbumPhotos_LegacyFolderAlbumUsesNameAsPath(t *testing.T) {
	db := newTestDB(t)
	legacyAlbum := &storage.Album{
		Name:        "legacy/folder",
		Description: "自动从文件夹导入",
		CreatedBy:   1,
		CreatedAt:   time.Now(),
	}
	if err := db.CreateAlbum(legacyAlbum); err != nil {
		t.Fatalf("创建旧版文件夹相册失败: %v", err)
	}
	photo := makePhoto(1, time.Now())
	photo.UUID = "legacy-folder-direct-photo"
	photo.SourceRelPath = "legacy/folder/photo.jpg"
	if err := db.SavePhoto(photo); err != nil {
		t.Fatalf("保存旧版相册直接媒体失败: %v", err)
	}
	nested := makePhoto(1, time.Now().Add(time.Minute))
	nested.UUID = "legacy-folder-nested-photo"
	nested.SourceRelPath = "legacy/folder/child/photo.jpg"
	if err := db.SavePhoto(nested); err != nil {
		t.Fatalf("保存旧版相册下级媒体失败: %v", err)
	}

	page, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{AlbumID: legacyAlbum.ID, UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查询旧版文件夹相册媒体失败: %v", err)
	}
	if len(page.Photos) != 1 || page.Photos[0].ID != photo.ID {
		t.Fatalf("期望旧版文件夹相册返回直接媒体，得到 %#v", page.Photos)
	}
}

func TestListAlbumPhotos_LegacyFolderAlbumKeepsPhotosWithoutSourcePath(t *testing.T) {
	db := newTestDB(t)
	legacyAlbum := &storage.Album{
		Name:        "legacy/folder",
		Description: "自动从文件夹导入",
		CreatedBy:   1,
		CreatedAt:   time.Now(),
	}
	if err := db.CreateAlbum(legacyAlbum); err != nil {
		t.Fatalf("创建旧版文件夹相册失败: %v", err)
	}
	photo := makePhoto(1, time.Now())
	photo.UUID = "legacy-photo-without-source-path"
	photo.SourceRelPath = ""
	if err := db.SavePhoto(photo); err != nil {
		t.Fatalf("保存旧版无源路径媒体失败: %v", err)
	}
	if err := db.AddPhotoToAlbum(legacyAlbum.ID, photo.ID, 1); err != nil {
		t.Fatalf("添加旧版相册关联失败: %v", err)
	}

	page, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{AlbumID: legacyAlbum.ID, UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查询旧版无源路径相册媒体失败: %v", err)
	}
	if len(page.Photos) != 1 || page.Photos[0].ID != photo.ID {
		t.Fatalf("期望保留旧版 album_photos 关联中的无源路径媒体，得到 %#v", page.Photos)
	}
}

func TestUpdateAlbum_Success(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "旧名字", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	album.Name = "新名字"
	album.Description = "描述"
	if err := db.UpdateAlbum(album); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	got, _ := db.GetAlbumByID(album.ID, 1)
	if got.Name != "新名字" || got.Description != "描述" {
		t.Errorf("更新后数据不匹配")
	}
}

func TestDeleteAlbum_Success(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "待删除", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	if err := db.DeleteAlbum(album.ID, 1); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	got, _ := db.GetAlbumByID(album.ID, 1)
	if got != nil {
		t.Error("删除后不应该查到相册")
	}
}

func TestAddAndRemovePhotoFromAlbum(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "相册", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)
	p := makePhoto(1, time.Now())
	db.SavePhoto(p)

	// 添加
	if err := db.AddPhotoToAlbum(album.ID, p.ID, 1); err != nil {
		t.Fatalf("添加图片到相册失败: %v", err)
	}
	inAlbum, _ := db.IsPhotoInAlbum(album.ID, p.ID)
	if !inAlbum {
		t.Error("图片应该在相册中")
	}

	// 重复添加应该不报错（INSERT OR IGNORE）
	if err := db.AddPhotoToAlbum(album.ID, p.ID, 1); err != nil {
		t.Errorf("重复添加不应该报错: %v", err)
	}

	// 移除
	if err := db.RemovePhotoFromAlbum(album.ID, p.ID, 1); err != nil {
		t.Fatalf("移除图片失败: %v", err)
	}
	inAlbum, _ = db.IsPhotoInAlbum(album.ID, p.ID)
	if inAlbum {
		t.Error("移除后不应该在相册中")
	}
}

func TestListAlbumPhotos_Pagination(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "相册", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = "album-photo-" + string(rune('a'+i))
		db.SavePhoto(p)
		db.AddPhotoToAlbum(album.ID, p.ID, 1)
	}

	page1, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{
		AlbumID: album.ID, UserID: 1, Limit: 3,
	})
	if err != nil {
		t.Fatalf("查询相册图片失败: %v", err)
	}
	if len(page1.Photos) != 3 || !page1.HasMore {
		t.Errorf("第一页应该有 3 张且有更多，实际 %d 张 hasMore=%v", len(page1.Photos), page1.HasMore)
	}

	page2, _ := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{
		AlbumID: album.ID, UserID: 1, Limit: 3, Cursor: page1.NextCursor,
	})
	if len(page2.Photos) != 2 || page2.HasMore {
		t.Errorf("第二页应该有 2 张且无更多，实际 %d 张 hasMore=%v", len(page2.Photos), page2.HasMore)
	}
}

func TestListAlbumPhotos_SortByNameAndSize(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "排序相册", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	inputs := []struct {
		name string
		size int64
	}{
		{name: "C.jpg", size: 300},
		{name: "a.jpg", size: 100},
		{name: "B.jpg", size: 500},
		{name: "1（90）.jpg", size: 90},
		{name: "1（10）.jpg", size: 10},
		{name: "1（9）.jpg", size: 9},
	}
	for i, input := range inputs {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = "sorted-album-photo-" + string(rune('a'+i))
		p.OriginalName = input.name
		p.Size = input.size
		db.SavePhoto(p)
		db.AddPhotoToAlbum(album.ID, p.ID, 1)
	}

	byName, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{
		AlbumID: album.ID, UserID: 1, Limit: 4, Sort: "name",
	})
	if err != nil {
		t.Fatalf("按名称查询失败: %v", err)
	}
	if got := []string{byName.Photos[0].OriginalName, byName.Photos[1].OriginalName, byName.Photos[2].OriginalName, byName.Photos[3].OriginalName}; got[0] != "1（9）.jpg" || got[1] != "1（10）.jpg" || got[2] != "1（90）.jpg" || got[3] != "a.jpg" || !byName.HasMore {
		t.Fatalf("按名称第一页排序不正确: %+v hasMore=%v", got, byName.HasMore)
	}
	byNameNext, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{
		AlbumID: album.ID, UserID: 1, Limit: 4, Sort: "name", Cursor: byName.NextCursor,
	})
	if err != nil {
		t.Fatalf("按名称第二页查询失败: %v", err)
	}
	if len(byNameNext.Photos) != 2 || byNameNext.Photos[0].OriginalName != "B.jpg" || byNameNext.Photos[1].OriginalName != "C.jpg" {
		t.Fatalf("按名称第二页排序不正确: %+v", byNameNext.Photos)
	}

	bySize, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{
		AlbumID: album.ID, UserID: 1, Limit: 6, Sort: "size",
	})
	if err != nil {
		t.Fatalf("按大小查询失败: %v", err)
	}
	if got := []string{bySize.Photos[0].OriginalName, bySize.Photos[1].OriginalName, bySize.Photos[2].OriginalName}; got[0] != "B.jpg" || got[1] != "C.jpg" || got[2] != "a.jpg" {
		t.Fatalf("按大小排序不正确: %+v", got)
	}
}

func makeAlbumWithPhotos(t *testing.T, db *DB, names []string, sizes []int64) (*storage.Album, []*storage.Photo) {
	t.Helper()
	album := &storage.Album{
		Name:      "定位测试相册",
		CreatedBy: 1,
		CreatedAt: time.Now(),
	}
	if err := db.CreateAlbum(album); err != nil {
		t.Fatalf("创建测试相册失败: %v", err)
	}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	photos := make([]*storage.Photo, 0, len(names))
	for i, name := range names {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = fmt.Sprintf("album-locate-%02d", i)
		p.OriginalName = name
		if i < len(sizes) && sizes[i] > 0 {
			p.Size = sizes[i]
		}
		if err := db.SavePhoto(p); err != nil {
			t.Fatalf("保存测试媒体失败: %v", err)
		}
		if err := db.AddPhotoToAlbum(album.ID, p.ID, 1); err != nil {
			t.Fatalf("添加测试媒体到相册失败: %v", err)
		}
		photos = append(photos, p)
	}
	return album, photos
}

func TestLocateAlbumWindow_NameSortProvidesStableNextCursor(t *testing.T) {
	db := newTestDB(t)
	album, photos := makeAlbumWithPhotos(t, db,
		[]string{"1.png", "2.png", "9.png", "10.png", "11.png", "90.png", "100.png", "101.png"},
		nil,
	)

	located, err := db.LocateAlbumWindow(storage.LocateAlbumParams{
		AlbumID: album.ID,
		UserID:  1,
		PhotoID: photos[6].ID,
		Limit:   2,
		Sort:    "name",
	})
	if err != nil {
		t.Fatalf("name 排序定位相册失败: %v", err)
	}
	if located == nil {
		t.Fatal("应返回定位结果")
	}
	assertPhotoIDs(t, located.Photos, []int64{
		photos[4].ID, photos[5].ID, photos[6].ID, photos[7].ID,
	})
	if located.TargetIndex != 2 {
		t.Fatalf("目标索引错误，得到 %d，期望 2", located.TargetIndex)
	}
	if located.MissingBeforeCount != 4 {
		t.Fatalf("前缺口数量错误，得到 %d，期望 4", located.MissingBeforeCount)
	}
	if located.NextCursor == "" {
		t.Fatal("定位结果应返回 next_cursor")
	}

	page, err := db.ListAlbumPhotos(storage.ListAlbumPhotosParams{
		AlbumID: album.ID,
		UserID:  1,
		Limit:   30,
		Sort:    "name",
		Cursor:  located.NextCursor,
	})
	if err != nil {
		t.Fatalf("使用定位 next_cursor 查询后续页面失败: %v", err)
	}
	if len(page.Photos) != 0 {
		t.Fatalf("定位窗口之后不应从头重复分页，得到 %d 条", len(page.Photos))
	}
}

func TestLocateAlbumWindow_AndBefore_ForTimelineDesc(t *testing.T) {
	db := newTestDB(t)
	album, photos := makeAlbumWithPhotos(t, db,
		[]string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg", "f.jpg", "g.jpg", "h.jpg"},
		nil,
	)

	located, err := db.LocateAlbumWindow(storage.LocateAlbumParams{
		AlbumID: album.ID,
		UserID:  1,
		PhotoID: photos[4].ID,
		Limit:   2,
		Sort:    "timeline_desc",
	})
	if err != nil {
		t.Fatalf("倒序定位相册失败: %v", err)
	}
	assertPhotoIDs(t, located.Photos, []int64{
		photos[6].ID, photos[5].ID, photos[4].ID, photos[3].ID, photos[2].ID,
	})
	before, err := db.ListAlbumPhotosBefore(storage.LocateAlbumParams{
		AlbumID: album.ID,
		UserID:  1,
		PhotoID: located.Photos[0].ID,
		Limit:   2,
		Sort:    "timeline_desc",
	})
	if err != nil {
		t.Fatalf("倒序前置页面失败: %v", err)
	}
	assertPhotoIDs(t, before.Photos, []int64{photos[7].ID})
}

func TestLocateAlbumWindow_AndBefore_ForTimelineAsc(t *testing.T) {
	db := newTestDB(t)
	album, photos := makeAlbumWithPhotos(t, db,
		[]string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg", "f.jpg", "g.jpg", "h.jpg"},
		nil,
	)

	located, err := db.LocateAlbumWindow(storage.LocateAlbumParams{
		AlbumID: album.ID,
		UserID:  1,
		PhotoID: photos[4].ID,
		Limit:   2,
		Sort:    "timeline_asc",
	})
	if err != nil {
		t.Fatalf("正序定位相册失败: %v", err)
	}
	assertPhotoIDs(t, located.Photos, []int64{
		photos[2].ID, photos[3].ID, photos[4].ID, photos[5].ID, photos[6].ID,
	})
	before, err := db.ListAlbumPhotosBefore(storage.LocateAlbumParams{
		AlbumID: album.ID,
		UserID:  1,
		PhotoID: located.Photos[0].ID,
		Limit:   2,
		Sort:    "timeline_asc",
	})
	if err != nil {
		t.Fatalf("正序前置页面失败: %v", err)
	}
	assertPhotoIDs(t, before.Photos, []int64{photos[0].ID, photos[1].ID})
}

func TestLocateAlbumWindow_AndBefore_ForSizeSort(t *testing.T) {
	db := newTestDB(t)
	album, photos := makeAlbumWithPhotos(t, db,
		[]string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg", "f.jpg"},
		[]int64{100, 200, 300, 400, 500, 600},
	)

	located, err := db.LocateAlbumWindow(storage.LocateAlbumParams{
		AlbumID: album.ID,
		UserID:  1,
		PhotoID: photos[2].ID,
		Limit:   2,
		Sort:    "size",
	})
	if err != nil {
		t.Fatalf("按大小定位相册失败: %v", err)
	}
	assertPhotoIDs(t, located.Photos, []int64{
		photos[4].ID, photos[3].ID, photos[2].ID, photos[1].ID, photos[0].ID,
	})
	before, err := db.ListAlbumPhotosBefore(storage.LocateAlbumParams{
		AlbumID: album.ID,
		UserID:  1,
		PhotoID: located.Photos[0].ID,
		Limit:   2,
		Sort:    "size",
	})
	if err != nil {
		t.Fatalf("按大小前置页面失败: %v", err)
	}
	assertPhotoIDs(t, before.Photos, []int64{photos[5].ID})
}

func TestAlbumPhotoCount(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "计数测试", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	photos := make([]*storage.Photo, 3)
	for i := 0; i < 3; i++ {
		p := makePhoto(1, time.Now())
		p.UUID = "count-" + string(rune('a'+i))
		db.SavePhoto(p)
		db.AddPhotoToAlbum(album.ID, p.ID, 1)
		photos[i] = p
	}

	got, _ := db.GetAlbumByID(album.ID, 1)
	if got.PhotoCount != 3 {
		t.Errorf("期望 PhotoCount=3，得到 %d", got.PhotoCount)
	}

	// 软删除一张后计数应该减少
	db.SoftDeletePhoto(photos[0].ID, 1, 1)
	got, _ = db.GetAlbumByID(album.ID, 1)
	if got.PhotoCount != 2 {
		t.Errorf("软删除后期望 PhotoCount=2，得到 %d", got.PhotoCount)
	}

	// ListAlbums 也要验证
	albums, _ := db.ListAlbums(1)
	if len(albums) == 0 || albums[0].PhotoCount != 2 {
		t.Errorf("ListAlbums 软删除后期望 PhotoCount=2，得到 %d", albums[0].PhotoCount)
	}
}

func TestAlbumCoverUUID(t *testing.T) {
	db := newTestDB(t)
	album := &storage.Album{Name: "封面测试", CreatedBy: 1, CreatedAt: time.Now()}
	db.CreateAlbum(album)

	// 无图片时 CoverUUID 为空
	got, _ := db.GetAlbumByID(album.ID, 1)
	if got.CoverUUID != "" {
		t.Errorf("无图片时 CoverUUID 应为空，得到 %s", got.CoverUUID)
	}

	// 添加图片后 CoverUUID 应为该图片的 UUID
	p := makePhoto(1, time.Now())
	p.UUID = "cover-test-uuid"
	db.SavePhoto(p)
	db.AddPhotoToAlbum(album.ID, p.ID, 1)

	got, _ = db.GetAlbumByID(album.ID, 1)
	if got.CoverUUID != "cover-test-uuid" {
		t.Errorf("期望 CoverUUID=cover-test-uuid，得到 %s", got.CoverUUID)
	}

	// 软删除图片后 CoverUUID 应再次为空
	db.SoftDeletePhoto(p.ID, 1, 1)
	got, _ = db.GetAlbumByID(album.ID, 1)
	if got.CoverUUID != "" {
		t.Errorf("软删除后 CoverUUID 应为空，得到 %s", got.CoverUUID)
	}
}

func TestFolderAlbumCoverUUIDFallsBackToDescendantPhoto(t *testing.T) {
	db := newTestDB(t)
	parent := &storage.Album{
		Name:          "1",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "1",
		CreatedBy:     1,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateAlbum(parent); err != nil {
		t.Fatalf("创建父级文件夹相册失败: %v", err)
	}
	child := &storage.Album{
		Name:          "1/2",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "1/2",
		CreatedBy:     1,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateAlbum(child); err != nil {
		t.Fatalf("创建子级文件夹相册失败: %v", err)
	}
	p := makePhoto(1, time.Now())
	p.UUID = "descendant-cover-uuid"
	p.SourceRelPath = "1/2/photo.jpg"
	if err := db.SavePhoto(p); err != nil {
		t.Fatalf("保存子级照片失败: %v", err)
	}
	if err := db.AddPhotoToAlbum(child.ID, p.ID, 1); err != nil {
		t.Fatalf("加入子级相册失败: %v", err)
	}
	if err := db.RefreshFolderAlbumCovers(1); err != nil {
		t.Fatalf("刷新文件夹相册封面失败: %v", err)
	}

	got, err := db.GetAlbumByID(parent.ID, 1)
	if err != nil {
		t.Fatalf("查询父级相册失败: %v", err)
	}
	if got.CoverUUID != "descendant-cover-uuid" {
		t.Fatalf("期望父级相册递归使用子级照片封面，得到 %q", got.CoverUUID)
	}
	if got.PhotoCount != 0 {
		t.Fatalf("父级相册不应把子级照片计入直接媒体数量，得到 %d", got.PhotoCount)
	}

	albums, err := db.ListAlbums(1)
	if err != nil {
		t.Fatalf("查询相册列表失败: %v", err)
	}
	var listedParent *storage.Album
	for _, album := range albums {
		if album.ID == parent.ID {
			listedParent = album
			break
		}
	}
	if listedParent == nil || listedParent.CoverUUID != "descendant-cover-uuid" {
		t.Fatalf("相册列表也应递归使用子级照片封面，得到 %#v", listedParent)
	}
}

func TestFolderAlbumTrashLifecycle_GroupsTreeAndExcludesMediaTrash(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	parent := &storage.Album{
		Name:          "旅行",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "旅行",
		CreatedBy:     1,
		CreatedAt:     now,
	}
	child := &storage.Album{
		Name:          "旅行/第一天",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "旅行/第一天",
		CreatedBy:     1,
		CreatedAt:     now,
	}
	personal := &storage.Album{Name: "个人相册", CreatedBy: 1, CreatedAt: now}
	for _, album := range []*storage.Album{parent, child, personal} {
		if err := db.CreateAlbum(album); err != nil {
			t.Fatalf("创建相册失败: %v", err)
		}
	}

	photoInRoot := makePhoto(1, now)
	photoInRoot.UUID = "folder-root-photo"
	photoInRoot.SourceRelPath = "旅行/root.jpg"
	photoInChild := makePhoto(1, now.Add(time.Second))
	photoInChild.UUID = "folder-child-photo"
	photoInChild.SourceRelPath = "旅行/第一天/child.jpg"
	photoOutside := makePhoto(1, now.Add(2*time.Second))
	photoOutside.UUID = "outside-photo"
	photoOutside.SourceRelPath = "旅行2/outside.jpg"
	for _, photo := range []*storage.Photo{photoInRoot, photoInChild, photoOutside} {
		if err := db.SavePhoto(photo); err != nil {
			t.Fatalf("保存媒体失败: %v", err)
		}
	}

	entry, err := db.SoftDeleteFolderAlbumTree(parent.ID, 1, 1, "folder-group-1")
	if err != nil {
		t.Fatalf("软删除文件夹树失败: %v", err)
	}
	if entry.PhotoCount != 2 || entry.FolderCount != 2 {
		t.Fatalf("批次统计错误: %+v", entry)
	}

	trashedMedia, err := db.ListTrashedPhotos(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查询普通媒体回收站失败: %v", err)
	}
	if len(trashedMedia.Photos) != 0 {
		t.Fatalf("文件夹批次媒体不应重复出现在普通媒体回收站，得到 %d 条", len(trashedMedia.Photos))
	}
	trashAlbums, err := db.ListTrashedFolderAlbums(1)
	if err != nil {
		t.Fatalf("查询文件夹回收站失败: %v", err)
	}
	if len(trashAlbums) != 1 || trashAlbums[0].GroupID != "folder-group-1" {
		t.Fatalf("应聚合为 1 个文件夹批次，得到 %+v", trashAlbums)
	}
	if trashAlbums[0].CoverUUID != "folder-child-photo" {
		t.Fatalf("文件夹回收站应返回批次封面 UUID，得到 %q", trashAlbums[0].CoverUUID)
	}
	if albums, err := db.ListAlbums(1); err != nil {
		t.Fatalf("查询活动相册失败: %v", err)
	} else if len(albums) != 1 || albums[0].ID != personal.ID {
		t.Fatalf("软删除文件夹后只应保留个人相册，得到 %+v", albums)
	}

	if err := db.RestoreFolderAlbumTrash("folder-group-1", 1); err != nil {
		t.Fatalf("恢复文件夹批次失败: %v", err)
	}
	if photo, err := db.GetPhotoByID(photoInChild.ID, 1); err != nil || photo == nil {
		t.Fatalf("恢复后子目录媒体不可见: photo=%+v err=%v", photo, err)
	}

	if _, err := db.SoftDeleteFolderAlbumTree(parent.ID, 1, 1, "folder-group-2"); err != nil {
		t.Fatalf("再次软删除文件夹树失败: %v", err)
	}
	deleted, err := db.HardDeleteFolderAlbumTrash("folder-group-2", 1)
	if err != nil {
		t.Fatalf("永久删除文件夹批次失败: %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("永久删除应返回 2 条媒体记录，得到 %d", len(deleted))
	}
	if photo, err := db.GetPhotoByIDAny(photoInRoot.ID, 1); err != nil || photo != nil {
		t.Fatalf("永久删除后根目录媒体记录仍存在: photo=%+v err=%v", photo, err)
	}
	trashAlbums, err = db.ListTrashedFolderAlbums(1)
	if err != nil {
		t.Fatalf("再次查询文件夹回收站失败: %v", err)
	}
	if len(trashAlbums) != 0 {
		t.Fatalf("永久删除后文件夹批次应为空，得到 %+v", trashAlbums)
	}
}

func TestFolderAlbumTrashTreatsPatternCharactersLiterally(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	percentFolder := &storage.Album{
		Name:          "100%",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "100%",
		CreatedBy:     1,
		CreatedAt:     now,
	}
	otherFolder := &storage.Album{
		Name:          "100X",
		Description:   "自动从文件夹导入",
		SourceKind:    "folder",
		SourceRelPath: "100X",
		CreatedBy:     1,
		CreatedAt:     now,
	}
	if err := db.CreateAlbum(percentFolder); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateAlbum(otherFolder); err != nil {
		t.Fatal(err)
	}
	percentPhoto := makePhoto(1, now)
	percentPhoto.UUID = "percent-folder-photo"
	percentPhoto.SourceRelPath = "100%/photo.jpg"
	otherPhoto := makePhoto(1, now.Add(time.Second))
	otherPhoto.UUID = "other-folder-photo"
	otherPhoto.SourceRelPath = "100X/photo.jpg"
	if err := db.SavePhoto(percentPhoto); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePhoto(otherPhoto); err != nil {
		t.Fatal(err)
	}

	if _, err := db.SoftDeleteFolderAlbumTree(percentFolder.ID, 1, 1, "literal-group"); err != nil {
		t.Fatal(err)
	}
	if photo, err := db.GetPhotoByID(otherPhoto.ID, 1); err != nil || photo == nil {
		t.Fatalf("相邻目录不应被误删: photo=%+v err=%v", photo, err)
	}
}
