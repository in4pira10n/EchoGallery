package service

import (
	"os"
	"path/filepath"
	"testing"

	"echogallery/internal/storage"
)

func saveTestVideo(t *testing.T, svc *PhotoService, repo *mockRepo, userID int64, name string) *storage.Photo {
	t.Helper()
	if err := os.WriteFile(filepath.Join(svc.sourcePath, name), []byte("video"), 0644); err != nil {
		t.Fatalf("写入测试视频失败: %v", err)
	}
	photo := &storage.Photo{
		UUID:          "video-1",
		OriginalName:  name,
		MediaKind:     storage.MediaKindVideo,
		MimeType:      "video/mp4",
		SourceRelPath: name,
		UploadedBy:    userID,
	}
	if err := repo.SavePhoto(photo); err != nil {
		t.Fatalf("保存测试视频失败: %v", err)
	}
	return photo
}

func TestVideoPlaybackPreference_UsesLibraryVolumeMeta(t *testing.T) {
	sourceDir := t.TempDir()
	repo := newMockRepo()
	svc := newPhotoServiceSync(repo, sourceDir, t.TempDir(), t.TempDir())
	photo := saveTestVideo(t, svc, repo, 1, "clip.mp4")
	repo.playbackPrefs["1:1"] = &storage.VideoPlaybackPreference{
		PhotoID:    photo.ID,
		Volume:     0.25,
		Muted:      true,
		ResumeTime: 42,
	}
	repo.libraryVideoVolume = 0.8
	repo.libraryVideoVolumeSet = true

	pref, err := svc.GetVideoPlaybackPreference(photo.ID, 1)
	if err != nil {
		t.Fatalf("读取播放偏好失败: %v", err)
	}
	if pref.Volume != 0.8 {
		t.Fatalf("期望使用资源库共享音量 0.8，得到 %.2f", pref.Volume)
	}
	if !pref.Muted || pref.ResumeTime != 42 {
		t.Fatalf("期望保留媒体级静音/续播，得到 %+v", pref)
	}
}

func TestSaveVideoPlaybackPreference_PersistsLibraryVolumeAndMediaState(t *testing.T) {
	sourceDir := t.TempDir()
	repo := newMockRepo()
	svc := newPhotoServiceSync(repo, sourceDir, t.TempDir(), t.TempDir())
	photo := saveTestVideo(t, svc, repo, 1, "clip.mp4")

	pref, err := svc.SaveVideoPlaybackPreference(photo.ID, 1, 0.35, true, 99, []storage.VideoPlaybackBookmark{{Slot: 1, Time: 12}})
	if err != nil {
		t.Fatalf("保存播放偏好失败: %v", err)
	}
	if !repo.libraryVideoVolumeSet || repo.libraryVideoVolume != 0.35 {
		t.Fatalf("期望音量保存为资源库共享 meta，得到 set=%v volume=%.2f", repo.libraryVideoVolumeSet, repo.libraryVideoVolume)
	}
	if pref.Volume != 0.35 || !pref.Muted || pref.ResumeTime != 99 || len(pref.Bookmarks) != 1 {
		t.Fatalf("期望保存媒体级播放状态，得到 %+v", pref)
	}
}
