package service

import (
	"os"
	"path/filepath"
	"testing"

	"echogallery/internal/storage"
)

func TestImportExistingPhotos_Success(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	srcPath := filepath.Join(svc.sourcePath, "IMG_0001.JPG")
	if err := os.WriteFile(srcPath, createJPEGBytes(640, 480), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.sourcePath, ".hidden.jpg"), createJPEGBytes(10, 10), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.sourcePath, "note.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.ImportExistingPhotos(1, nil)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("期望导入 1 张，得到 %d", summary.Imported)
	}

	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("获取时间线失败: %v", err)
	}
	if len(page.Photos) != 1 {
		t.Fatalf("期望时间线有 1 张，得到 %d", len(page.Photos))
	}

	photo := page.Photos[0]
	if photo.OriginalName != "IMG_0001.JPG" {
		t.Fatalf("期望保留原文件名，得到 %s", photo.OriginalName)
	}
	if _, err := os.Stat(srcPath); err != nil {
		t.Fatalf("原始文件应被保留: %v", err)
	}
	if got := svc.PhotoPath(photo); got != srcPath {
		t.Fatalf("导入后的媒体路径应直接指向源文件，得到 %s", got)
	}
	if photo.SourceRelPath != "IMG_0001.JPG" {
		t.Fatalf("期望记录导入源路径，得到 %s", photo.SourceRelPath)
	}
	if photo.StorageRelPath != "" {
		t.Fatalf("导入历史图片不应再生成内部原图副本，得到 %s", photo.StorageRelPath)
	}
	if _, err := os.Stat(svc.ThumbnailPath(photo)); !os.IsNotExist(err) {
		t.Fatalf("启动导入阶段不应预生成缩略图，得到 err=%v", err)
	}
}

func TestImportExistingPhotos_Idempotent(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	if err := os.WriteFile(filepath.Join(svc.sourcePath, "IMG_0002.jpg"), createJPEGBytes(320, 240), 0644); err != nil {
		t.Fatal(err)
	}

	first, err := svc.ImportExistingPhotos(1, nil)
	if err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	second, err := svc.ImportExistingPhotos(1, nil)
	if err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}

	if first.Imported != 1 {
		t.Fatalf("首次导入数量错误: %d", first.Imported)
	}
	if second.Imported != 0 {
		t.Fatalf("二次导入不应重复导入，得到 %d", second.Imported)
	}

	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("获取时间线失败: %v", err)
	}
	if len(page.Photos) != 1 {
		t.Fatalf("期望最终只有 1 条记录，得到 %d", len(page.Photos))
	}
}

func TestImportExistingPhotos_CreatesAlbumsFromFolders(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	if err := os.MkdirAll(filepath.Join(svc.sourcePath, "旅行", "第一天"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.sourcePath, "旅行", "第一天", "IMG_1001.jpg"), createJPEGBytes(200, 120), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.ImportExistingPhotos(1, nil)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("期望导入 1 张，得到 %d", summary.Imported)
	}

	albums, err := svc.ListAlbums(1)
	if err != nil {
		t.Fatalf("查询相册失败: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("期望生成 1 个相册，得到 %d", len(albums))
	}
	if albums[0].Name != "旅行/第一天" {
		t.Fatalf("期望相册名为 旅行/第一天，得到 %s", albums[0].Name)
	}

	page, err := svc.GetAlbumMedia(storage.ListAlbumPhotosParams{AlbumID: albums[0].ID, UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查询相册媒体失败: %v", err)
	}
	if len(page.Photos) != 1 {
		t.Fatalf("期望相册内有 1 张，得到 %d", len(page.Photos))
	}
}

func TestImportExistingPhotos_SkipsBrokenSupportedFiles(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	if err := os.WriteFile(filepath.Join(svc.sourcePath, "broken.png"), []byte("not-a-real-png"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.sourcePath, "ok.jpg"), createJPEGBytes(320, 200), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.ImportExistingPhotos(1, nil)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("期望导入 1 张有效媒体，得到 %d", summary.Imported)
	}
	if summary.Skipped != 1 {
		t.Fatalf("期望跳过 1 个损坏文件，得到 %d", summary.Skipped)
	}

	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 10})
	if err != nil {
		t.Fatalf("获取时间线失败: %v", err)
	}
	if len(page.Photos) != 1 {
		t.Fatalf("期望时间线中只有 1 条有效记录，得到 %d", len(page.Photos))
	}
	if page.Photos[0].OriginalName != "ok.jpg" {
		t.Fatalf("期望保留有效文件，得到 %s", page.Photos[0].OriginalName)
	}
}
