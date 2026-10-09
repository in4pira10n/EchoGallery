package service

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

func createJPEGBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func newTestPhotoService(t *testing.T) (*PhotoService, string) {
	t.Helper()
	sourceDir := t.TempDir()
	dataDir := t.TempDir()
	trashDir := t.TempDir()
	repo := newMockRepo()
	svc := newPhotoServiceSync(repo, sourceDir, dataDir, trashDir)
	return svc, dataDir
}

func TestPhotoService_UserAlbumIsVisibleAfterCreation(t *testing.T) {
	svc, _ := newTestPhotoService(t)

	created, err := svc.CreateAlbum("用户相册", "", 1)
	if err != nil {
		t.Fatalf("创建用户相册失败: %v", err)
	}

	albums, err := svc.ListAlbums(1)
	if err != nil {
		t.Fatalf("查询相册列表失败: %v", err)
	}
	if len(albums) != 1 || albums[0].ID != created.ID {
		t.Fatalf("新建用户相册应出现在列表中，得到 %+v", albums)
	}

	detail, err := svc.GetAlbum(created.ID, 1)
	if err != nil {
		t.Fatalf("查询用户相册详情失败: %v", err)
	}
	if detail == nil || detail.ID != created.ID {
		t.Fatalf("新建用户相册应可打开，得到 %+v", detail)
	}
}

func readTrashLinks(t *testing.T, svc *PhotoService) []string {
	t.Helper()
	links, err := svc.collectTrashLinks(1)
	if err != nil {
		t.Fatalf("读取回收站数据库失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(svc.trashBaseDir(), "trash-links.txt")); !os.IsNotExist(err) {
		t.Fatalf("不应再生成 trash-links.txt: %v", err)
	}
	return links
}

// --- Upload 测试 ---

func TestUpload_Success(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	data := createJPEGBytes(800, 600)

	result, err := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(data),
		OriginalName: "test.jpg",
		Size:         int64(len(data)),
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if result.Photo.ID == 0 {
		t.Error("上传后 ID 应该被填充")
	}
	if result.Photo.UUID == "" {
		t.Error("UUID 不能为空")
	}
	if result.Photo.Width != 800 || result.Photo.Height != 600 {
		t.Errorf("尺寸不匹配: %dx%d", result.Photo.Width, result.Photo.Height)
	}
	if result.Photo.SourceRelPath == "" {
		t.Fatal("上传图片应记录资源库内源文件路径")
	}
	if result.Photo.StorageRelPath != "" {
		t.Fatalf("上传图片不应写入应用托管目录，得到 %s", result.Photo.StorageRelPath)
	}
}

func TestRenameMedia_PreservesUUIDAndExtension(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	result, err := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(createJPEGBytes(80, 60)),
		OriginalName: "before.jpg",
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})
	if err != nil {
		t.Fatalf("上传测试媒体失败: %v", err)
	}
	oldPath := svc.MediaPath(result.Photo)
	beforeBytes, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("读取重命名前媒体失败: %v", err)
	}
	beforeInfo, err := os.Stat(oldPath)
	if err != nil {
		t.Fatalf("读取重命名前文件信息失败: %v", err)
	}
	expectedEXIF := &storage.PhotoEXIF{
		Make:         "Echo Camera",
		Model:        "Preservation Test",
		Orientation:  6,
		FNumber:      "f/2.8",
		ExposureTime: "1/125",
		ISOSpeed:     400,
		Latitude:     31.2304,
		Longitude:    121.4737,
		HasGPS:       true,
	}
	if err := svc.repo.UpdatePhotoEXIF(result.Photo.ID, 1, expectedEXIF); err != nil {
		t.Fatalf("设置测试 EXIF 失败: %v", err)
	}
	expectedEXIFValue := *expectedEXIF

	updated, err := svc.RenameMedia(result.Photo.ID, 1, "after")
	if err != nil {
		t.Fatalf("重命名媒体失败: %v", err)
	}
	if updated.UUID != result.Photo.UUID || updated.OriginalName != "after.jpg" {
		t.Fatalf("重命名不应改变 UUID 或扩展名: before=%+v after=%+v", result.Photo, updated)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("旧文件仍存在或检查失败: %v", err)
	}
	newPath := svc.MediaPath(updated)
	afterInfo, err := os.Stat(newPath)
	if err != nil {
		t.Fatalf("新文件不存在: %v", err)
	}
	if !os.SameFile(beforeInfo, afterInfo) {
		t.Fatal("重命名应保留原文件本身，而不是复制或重建媒体文件")
	}
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) || beforeInfo.Mode() != afterInfo.Mode() {
		t.Fatalf("重命名不应改变文件修改时间或权限: before=%v/%v after=%v/%v", beforeInfo.ModTime(), beforeInfo.Mode(), afterInfo.ModTime(), afterInfo.Mode())
	}
	afterBytes, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("读取重命名后媒体失败: %v", err)
	}
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatal("重命名不应改变媒体文件字节，EXIF 等内嵌信息必须原样保留")
	}
	if updated.EXIF == nil || !reflect.DeepEqual(*updated.EXIF, expectedEXIFValue) {
		t.Fatalf("重命名不应改变数据库中的 EXIF 信息: got=%+v want=%+v", updated.EXIF, expectedEXIFValue)
	}
}

func TestUpload_FileWrittenToDisk(t *testing.T) {
	svc, dir := newTestPhotoService(t)
	data := createJPEGBytes(100, 100)

	result, err := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(data),
		OriginalName: "photo.jpg",
		Size:         int64(len(data)),
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 验证文件已写入磁盘
	path := svc.PhotoPath(result.Photo)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("图片文件应该存在于 %s", path)
	}
	if !strings.Contains(path, "EchoGallery Uploads") {
		t.Fatalf("图片应写入资源库上传目录，得到 %s", path)
	}
	_ = dir
}

func TestUpload_UnsupportedFormat(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	_, err := svc.Upload(UploadInput{
		Reader:       bytes.NewReader([]byte("not an image")),
		OriginalName: "file.pdf",
		Size:         100,
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})
	if err == nil {
		t.Error("不支持的格式应该返回错误")
	}
}

func TestThumbnailExistsForTier_CleansLegacyPreviewFilesWhenFullExists(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	photo := &storage.Photo{UUID: "thumb-1", MediaKind: storage.MediaKindImage}

	fullPath := svc.ThumbnailPath(photo)
	previewPath := svc.legacyThumbnailPreviewPath(photo)
	buildPreviewPath := svc.legacyThumbnailBuildPreviewPath(photo)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("full"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previewPath, []byte("preview"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(buildPreviewPath, []byte("build-preview"), 0644); err != nil {
		t.Fatal(err)
	}

	if !svc.thumbnailExistsForTier(photo, thumbnailTierFull) {
		t.Fatal("full 缩略图应当满足当前请求")
	}
	if _, err := os.Stat(previewPath); !os.IsNotExist(err) {
		t.Fatalf("存在 full 时应删除 preview，得到 err=%v", err)
	}
	if _, err := os.Stat(buildPreviewPath); !os.IsNotExist(err) {
		t.Fatalf("存在 full 时应删除 build-preview，得到 err=%v", err)
	}
}

func TestThumbnailExistsForTier_DoesNotUseLegacyPreviewOnly(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	photo := &storage.Photo{UUID: "thumb-2", MediaKind: storage.MediaKindImage}

	previewPath := svc.legacyThumbnailPreviewPath(photo)
	buildPreviewPath := svc.legacyThumbnailBuildPreviewPath(photo)

	if err := os.MkdirAll(filepath.Dir(previewPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previewPath, []byte("preview"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(buildPreviewPath, []byte("build-preview"), 0644); err != nil {
		t.Fatal(err)
	}

	if svc.thumbnailExistsForTier(photo, thumbnailTierFull) {
		t.Fatal("旧 preview/build-preview 不应再满足当前标准缩略图请求")
	}
	if _, err := os.Stat(previewPath); err != nil {
		t.Fatalf("旧 preview 文件应暂时保留，等待 full 生成后清理: %v", err)
	}
}

func TestThumbnailExistsForTier_ReadsLegacyFlatWebPWithoutMoving(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	photo := &storage.Photo{UUID: "thumb-legacy-1", MediaKind: storage.MediaKindImage}

	legacyPath := svc.legacyFlatThumbnailPath(photo)
	shardedPath := svc.ThumbnailPath(photo)

	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy-webp"), 0644); err != nil {
		t.Fatal(err)
	}

	if !svc.thumbnailExistsForTier(photo, thumbnailTierFull) {
		t.Fatal("旧平铺 webp 缩略图应继续复用")
	}
	if _, err := os.Stat(shardedPath); !os.IsNotExist(err) {
		t.Fatalf("读取旧缩略图不应生成新的分片文件，得到 err=%v", err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("旧平铺缩略图应保留: %v", err)
	}
}

func TestThumbnailExistsForTier_ReadsLegacySharedShardWithoutMoving(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	svc.SetThumbnailLibraryID("lib_a")
	photo := &storage.Photo{UUID: "thumb-legacy-library-1", MediaKind: storage.MediaKindImage}

	legacyPath := svc.legacyShardedThumbnailPath(photo)
	scopedPath := svc.ThumbnailPath(photo)

	if legacyPath == scopedPath {
		t.Fatal("设置 library_id 后旧分片路径与新资源库路径不应相同")
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy-sharded-webp"), 0644); err != nil {
		t.Fatal(err)
	}

	if !svc.thumbnailExistsForTier(photo, thumbnailTierFull) {
		t.Fatal("旧共享分片缩略图应继续复用")
	}
	if _, err := os.Stat(scopedPath); !os.IsNotExist(err) {
		t.Fatalf("读取旧缩略图不应生成新的资源库文件，得到 err=%v", err)
	}
	if !strings.Contains(scopedPath, filepath.Join("lib_a")) {
		t.Fatalf("期望新缩略图路径包含资源库 ID 目录，得到 %s", scopedPath)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("旧共享分片缩略图应保留: %v", err)
	}
}

func TestMaintainThumbnailsContext_MovesLegacyAndCleansLegacyFiles(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	repo := svc.repo.(*mockRepo)
	photo := &storage.Photo{UUID: "thumb-maint-1", MediaKind: storage.MediaKindImage, UploadedBy: 1}
	if err := repo.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}

	legacyPath := svc.legacyFlatThumbnailPath(photo)
	shardedPath := svc.ThumbnailPath(photo)
	previewPath := svc.legacyThumbnailPreviewPath(photo)
	buildPreviewPath := svc.legacyThumbnailBuildPreviewPath(photo)
	legacyJPEGPath := svc.LegacyThumbnailPath(photo)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy-webp"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previewPath, []byte("preview"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(buildPreviewPath, []byte("build-preview"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyJPEGPath, []byte("legacy-jpg"), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.MaintainThumbnailsContext(context.Background(), 1, ThumbnailMaintenanceOptions{
		MoveLegacyThumbnails: true,
		CleanThumbnailFiles:  true,
	}, nil)
	if err != nil {
		t.Fatalf("整理缩略图失败: %v", err)
	}
	if summary.Moved != 1 {
		t.Fatalf("期望移动 1 个旧缩略图，得到 %d", summary.Moved)
	}
	if summary.Cleaned != 4 {
		t.Fatalf("期望清理 4 个旧文件，得到 %d", summary.Cleaned)
	}
	if _, err := os.Stat(shardedPath); err != nil {
		t.Fatalf("迁移后的分片缩略图应存在: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("旧平铺 webp 应被移走，得到 err=%v", err)
	}
	if _, err := os.Stat(previewPath); !os.IsNotExist(err) {
		t.Fatalf("preview 应被清理，得到 err=%v", err)
	}
	if _, err := os.Stat(buildPreviewPath); !os.IsNotExist(err) {
		t.Fatalf("build-preview 应被清理，得到 err=%v", err)
	}
	if _, err := os.Stat(legacyJPEGPath); !os.IsNotExist(err) {
		t.Fatalf("旧 jpg 应被清理，得到 err=%v", err)
	}
}

func TestMaintainThumbnailsContext_CleanOnlyKeepsLegacyFlatWebP(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	repo := svc.repo.(*mockRepo)
	photo := &storage.Photo{UUID: "thumb-maint-2", MediaKind: storage.MediaKindImage, UploadedBy: 1}
	if err := repo.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}

	legacyPath := svc.legacyFlatThumbnailPath(photo)
	previewPath := svc.legacyThumbnailPreviewPath(photo)
	buildPreviewPath := svc.legacyThumbnailBuildPreviewPath(photo)
	legacyJPEGPath := svc.LegacyThumbnailPath(photo)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy-webp"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previewPath, []byte("preview"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(buildPreviewPath, []byte("build-preview"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyJPEGPath, []byte("legacy-jpg"), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.MaintainThumbnailsContext(context.Background(), 1, ThumbnailMaintenanceOptions{
		CleanThumbnailFiles: true,
	}, nil)
	if err != nil {
		t.Fatalf("整理缩略图失败: %v", err)
	}
	if summary.Moved != 0 {
		t.Fatalf("未启用移动时不应迁移旧平铺 webp，得到 %d", summary.Moved)
	}
	if summary.Cleaned != 3 {
		t.Fatalf("期望清理 3 个旧文件，得到 %d", summary.Cleaned)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("未启用移动时旧平铺 webp 应保留: %v", err)
	}
	if _, err := os.Stat(previewPath); !os.IsNotExist(err) {
		t.Fatalf("preview 应被清理，得到 err=%v", err)
	}
	if _, err := os.Stat(buildPreviewPath); !os.IsNotExist(err) {
		t.Fatalf("build-preview 应被清理，得到 err=%v", err)
	}
	if _, err := os.Stat(legacyJPEGPath); !os.IsNotExist(err) {
		t.Fatalf("旧 jpg 应被清理，得到 err=%v", err)
	}
}

func TestMaintainThumbnailsContext_CleanNeverRemovesLibraryIDDirs(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	svc.SetThumbnailLibraryID("lib_keep")
	keepDir := filepath.Join(svc.thumbnailPath, "lib_keep")
	staleDir := filepath.Join(svc.thumbnailPath, "lib_stale")
	legacyShardDir := filepath.Join(svc.thumbnailPath, "ab")
	for _, dir := range []string{keepDir, staleDir, legacyShardDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "thumb.webp"), []byte("thumb"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := svc.MaintainThumbnailsContext(context.Background(), 1, ThumbnailMaintenanceOptions{
		CleanThumbnailFiles: true,
	}, nil)
	if err != nil {
		t.Fatalf("清理缩略图失败: %v", err)
	}
	if summary.Cleaned != 0 {
		t.Fatalf("没有媒体文件时不应清理任何缩略图文件或目录，得到 %d", summary.Cleaned)
	}
	if _, err := os.Stat(keepDir); err != nil {
		t.Fatalf("有效 library_id 目录应保留: %v", err)
	}
	if _, err := os.Stat(staleDir); err != nil {
		t.Fatalf("清理文件不应删除其它 library_id 目录: %v", err)
	}
	if _, err := os.Stat(legacyShardDir); err != nil {
		t.Fatalf("旧分片目录也不应被当作 library_id 删除: %v", err)
	}
}

func TestUpload_FallbackTime(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	data := createJPEGBytes(100, 100)
	fallback := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)

	result, err := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(data),
		OriginalName: "test.jpg",
		Size:         int64(len(data)),
		UploadedBy:   1,
		FileModTime:  fallback,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Photo.TakenAt.Equal(fallback) {
		t.Errorf("期望使用 fallback 时间 %v，得到 %v", fallback, result.Photo.TakenAt)
	}
}

func TestRegisterUploadedVideo_Success(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	result, err := svc.RegisterUploadedVideo(RegisterUploadedVideoInput{
		UUID:         "video-uuid",
		OriginalName: "demo.mp4",
		MimeType:     "video/mp4",
		Size:         4096,
		UploadedBy:   1,
		TakenAt:      time.Now(),
		Meta: &media.VideoMeta{
			Width:      1920,
			Height:     1080,
			DurationMS: 54321,
			FormatName: "mp4",
			CodecName:  "h264",
			FrameRate:  29.97,
		},
	})
	if err != nil {
		t.Fatalf("注册视频失败: %v", err)
	}
	if result.ID == 0 {
		t.Fatal("注册后应生成 ID")
	}
	if result.MediaKind != storage.MediaKindVideo {
		t.Fatalf("期望视频类型，得到 %s", result.MediaKind)
	}
	if result.DurationMS != 54321 {
		t.Fatalf("期望时长 54321，得到 %d", result.DurationMS)
	}
	if result.EXIF == nil || result.EXIF.VideoCodec != "h264" || result.EXIF.VideoFrameRate != 29.97 {
		t.Fatalf("期望写入视频元数据，得到 %+v", result.EXIF)
	}
}

func TestThumbnailPath_VideoUsesSharedThumbnailDirectory(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	photo := &storage.Photo{
		UUID:         "video-uuid",
		OriginalName: "demo.mp4",
		MediaKind:    storage.MediaKindVideo,
	}

	got := svc.ThumbnailPath(photo)
	want := svc.PosterPath(photo)
	if got != want {
		t.Fatalf("期望视频缩略图路径与海报路径一致，got=%s want=%s", got, want)
	}
	if want := imgpkg.ThumbnailShardPath(svc.thumbnailPath, photo.UUID); got != want {
		t.Fatalf("期望视频缩略图使用分片目录，got=%s want=%s", got, want)
	}
	if !strings.HasSuffix(got, filepath.Join("vi", "de", "video-uuid.webp")) {
		t.Fatalf("期望视频缩略图为 webp 文件，得到 %s", got)
	}
}

// --- GetTimeline / GetTrash / DeletePhoto / RestorePhoto / EmptyTrash 测试 ---

func TestGetTimeline(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	data := createJPEGBytes(100, 100)

	for i := 0; i < 3; i++ {
		svc.Upload(UploadInput{
			Reader:       bytes.NewReader(data),
			OriginalName: "test.jpg",
			Size:         int64(len(data)),
			UploadedBy:   1,
			FileModTime:  time.Now(),
		})
	}

	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("获取时间线失败: %v", err)
	}
	if len(page.Photos) != 3 {
		t.Errorf("期望 3 张，得到 %d", len(page.Photos))
	}
}

func TestMissingSourceFileHiddenFromLibrary(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	data := createJPEGBytes(100, 100)

	result, err := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(data),
		OriginalName: "missing.jpg",
		Size:         int64(len(data)),
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if err := os.Remove(svc.PhotoPath(result.Photo)); err != nil {
		t.Fatalf("删除源文件失败: %v", err)
	}

	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("获取时间线失败: %v", err)
	}
	if len(page.Photos) != 0 {
		t.Fatalf("缺失源文件不应展示在时间线，得到 %d 条", len(page.Photos))
	}

	photo, err := svc.GetPhoto(result.Photo.ID, 1)
	if err != nil {
		t.Fatalf("获取单张媒体失败: %v", err)
	}
	if photo != nil {
		t.Fatalf("缺失源文件不应返回单张媒体详情")
	}
}

func TestDeleteAndRestorePhoto(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	data := createJPEGBytes(100, 100)

	result, _ := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(data),
		OriginalName: "test.jpg",
		Size:         int64(len(data)),
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})

	// 删除
	if err := svc.DeletePhoto(result.Photo.ID, 1); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 时间线上应该消失
	page, _ := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(page.Photos) != 0 {
		t.Error("删除后时间线应该为空")
	}

	// 回收站应该有
	trash, _ := svc.GetTrash(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(trash.Photos) != 1 {
		t.Error("删除后回收站应该有 1 张")
	}
	links := readTrashLinks(t, svc)
	if len(links) != 1 {
		t.Fatalf("删除后 trash-links 应有 1 条，got=%d", len(links))
	}
	if links[0] != filepath.ToSlash(result.Photo.SourceRelPath) {
		t.Fatalf("回收站相对路径不正确: got=%q want=%q", links[0], result.Photo.SourceRelPath)
	}

	// 恢复
	if err := svc.RestorePhoto(result.Photo.ID, 1); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	page, _ = svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(page.Photos) != 1 {
		t.Error("恢复后时间线应该有 1 张")
	}
	if links := readTrashLinks(t, svc); len(links) != 0 {
		t.Fatalf("恢复后 trash-links 应为空，got=%v", links)
	}
}

func TestPermanentlyDeletePhoto_MovesManagedFilesToTrashDir(t *testing.T) {
	sourceDir := t.TempDir()
	dataDir := t.TempDir()
	trashDir := t.TempDir()
	repo := newMockRepo()
	svc := newPhotoServiceSync(repo, sourceDir, dataDir, trashDir)
	data := createJPEGBytes(100, 100)

	result, err := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(data),
		OriginalName: "trash-me.jpg",
		Size:         int64(len(data)),
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if err := svc.DeletePhoto(result.Photo.ID, 1); err != nil {
		t.Fatalf("软删除失败: %v", err)
	}
	if links := readTrashLinks(t, svc); len(links) != 1 {
		t.Fatalf("软删除后 trash-links 应有 1 条，got=%v", links)
	}

	mediaPath := svc.MediaPath(result.Photo)
	thumbPath := svc.ThumbnailPath(result.Photo)
	if err := svc.PermanentlyDeletePhoto(result.Photo.ID, 1); err != nil {
		t.Fatalf("永久删除失败: %v", err)
	}
	if _, err := os.Stat(mediaPath); !os.IsNotExist(err) {
		t.Fatalf("原媒体文件应已移走，stat err=%v", err)
	}
	if _, err := os.Stat(thumbPath); !os.IsNotExist(err) {
		t.Fatalf("原缩略图文件应已移走，stat err=%v", err)
	}

	if _, err := os.Stat(filepath.Join(sourceDir, ".echogallery", ".trash", filepath.Base(mediaPath))); err != nil {
		t.Fatalf("媒体文件应移动到回收站目录: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceDir, ".echogallery", ".trash", filepath.Base(thumbPath))); !os.IsNotExist(err) {
		t.Fatalf("缩略图不应移动到回收站，应该被清理，stat err=%v", err)
	}
	if links := readTrashLinks(t, svc); len(links) != 0 {
		t.Fatalf("永久删除后 trash-links 应为空，got=%v", links)
	}
}

func TestMoveFileToTrash_FallsBackWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.jpg")
	dest := filepath.Join(dir, "trash", "source.jpg")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatalf("创建目标目录失败: %v", err)
	}
	content := []byte("echo-gallery")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("写入源文件失败: %v", err)
	}

	prevRename := moveFileRename
	moveFileRename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	}
	t.Cleanup(func() {
		moveFileRename = prevRename
	})

	if err := moveFileToTrash(src, dest); err != nil {
		t.Fatalf("跨卷回退移动失败: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("源文件应已移除，stat err=%v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("目标文件内容不匹配: got=%q want=%q", string(got), string(content))
	}
}

func TestEmptyTrash(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	data := createJPEGBytes(100, 100)

	result, _ := svc.Upload(UploadInput{
		Reader:       bytes.NewReader(data),
		OriginalName: "test.jpg",
		Size:         int64(len(data)),
		UploadedBy:   1,
		FileModTime:  time.Now(),
	})
	svc.DeletePhoto(result.Photo.ID, 1)
	if links := readTrashLinks(t, svc); len(links) != 1 {
		t.Fatalf("清空前 trash-links 应有 1 条，got=%v", links)
	}

	if err := svc.EmptyTrash(1); err != nil {
		t.Fatalf("清空回收站失败: %v", err)
	}

	trash, _ := svc.GetTrash(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if len(trash.Photos) != 0 {
		t.Error("清空后回收站应该为空")
	}
	if links := readTrashLinks(t, svc); len(links) != 0 {
		t.Fatalf("清空后 trash-links 应为空，got=%v", links)
	}
}
