package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"echogallery/internal/config"
	"echogallery/internal/danmaku"
	"echogallery/internal/storage"
	"echogallery/internal/storage/sqlite"
)

func cleanupService(t *testing.T) (*PhotoService, *sqlite.DB) {
	t.Helper()
	source, data := t.TempDir(), t.TempDir()
	repo, err := sqlite.New(filepath.Join(data, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return NewPhotoServiceWithoutWarmup(repo, source, data, ""), repo
}

func putCleanupFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("retained data"), 0644); err != nil {
		t.Fatal(err)
	}
}

func saveCleanupPhoto(t *testing.T, svc *PhotoService, name string) *storage.Photo {
	t.Helper()
	path := filepath.Join(svc.sourcePath, name)
	putCleanupFile(t, path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	p := &storage.Photo{UUID: uuid.NewString(), OriginalName: filepath.Base(name),
		SourceRelPath: filepath.ToSlash(name), Size: info.Size(), SourceModUnix: info.ModTime().UnixNano(),
		MediaKind: storage.MediaKindVideo, MimeType: "video/mp4", UploadedBy: 1, TakenAt: time.Now(), UploadedAt: time.Now()}
	if err := svc.repo.SavePhoto(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCleanMissingMediaRemovesOwnedArtifactsAndPreservesOtherMedia(t *testing.T) {
	svc, repo := cleanupService(t)
	missing := saveCleanupPhoto(t, svc, "deleted.mp4")
	live := saveCleanupPhoto(t, svc, "live.mp4")
	trashed := saveCleanupPhoto(t, svc, "trash.mp4")
	if err := repo.SoftDeletePhoto(trashed.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(svc.MediaPath(trashed)); err != nil {
		t.Fatal(err)
	}
	root := config.LibraryDataRoot(svc.sourcePath)
	removed := []string{
		svc.ThumbnailPath(missing),
		filepath.Join(filepath.Dir(svc.ThumbnailPath(missing)), missing.UUID+".heif-v3-1-2.jpg"),
		filepath.Join(svc.dataPath, "playback", missing.UUID+"-old.mp4"),
		filepath.Join(root, "soundtracks", missing.UUID+"-previous.mp4"),
		filepath.Join(root, "subtitles", missing.UUID+"-previous.vtt"),
	}
	for folder, key := range map[string]string{
		"soundtracks": fmt.Sprintf("aac-v1:%s:%s:%d:%d:0", missing.UUID, missing.SourceRelPath, missing.Size, missing.SourceModUnix),
		"subtitles":   fmt.Sprintf("srt-v2-positioned:%s:%s:%d:%d:-1", missing.UUID, missing.SourceRelPath, missing.Size, missing.SourceModUnix),
	} {
		ext := ".mp4"
		if folder == "subtitles" {
			ext = ".vtt"
		}
		removed = append(removed, filepath.Join(root, folder, fmt.Sprintf("%x%s", sha256.Sum256([]byte(key)), ext)))
	}
	kept := []string{svc.MediaPath(live), filepath.Join(svc.sourcePath, "deleted.srt"),
		svc.ThumbnailPath(live), svc.ThumbnailPath(trashed),
		filepath.Join(root, "subtitles", live.UUID+"-other.vtt"),
		filepath.Join(root, "soundtracks", "unattributable-old-cache.mp4")}
	for _, path := range append(append([]string{}, removed...), kept...) {
		putCleanupFile(t, path)
	}
	for bvid, mediaUUID := range map[string]string{
		"BV1SopR6wE2x": missing.UUID, "BV1SopR6wE2y": live.UUID, "BV1SopR6wE2z": "",
	} {
		if err := danmaku.WriteMetadata(danmaku.Root(svc.sourcePath), danmaku.Metadata{Video: danmaku.Video{BVID: bvid}, MediaUUID: mediaUUID}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(svc.MediaPath(missing)); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		count, err := svc.CleanMissingMediaContext(context.Background(), 1, nil)
		if err != nil || count != 1-attempt {
			t.Fatalf("cleanup attempt %d: removed=%d err=%v", attempt, count, err)
		}
	}
	for _, path := range removed {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("generated artifact survived: %s (%v)", path, err)
		}
	}
	for _, path := range kept {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("unrelated or user-owned file changed: %s (%v)", path, err)
		}
	}
	if p, err := repo.GetPhotoByIDAny(missing.ID, 1); err != nil || p != nil {
		t.Fatalf("missing media record survived: %+v %v", p, err)
	}
	if p, err := repo.GetPhotoByIDAny(trashed.ID, 1); err != nil || p == nil {
		t.Fatalf("trash was removed: %+v %v", p, err)
	}
	for _, bvid := range []string{"BV1SopR6wE2y", "BV1SopR6wE2z"} {
		if _, err := danmaku.ReadMetadata(danmaku.Root(svc.sourcePath), bvid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := danmaku.ReadMetadata(danmaku.Root(svc.sourcePath), "BV1SopR6wE2x"); !os.IsNotExist(err) {
		t.Fatalf("deleted media danmaku survived: %v", err)
	}
}

func TestCleanMissingMediaPreflightFailureOrCancellationPreservesRecords(t *testing.T) {
	for _, mode := range []string{"cancelled", "unreachable", "io_error", "unreadable_link"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo := cleanupService(t)
			missing := saveCleanupPhoto(t, svc, "a.mp4")
			_ = os.Remove(svc.MediaPath(missing))
			putCleanupFile(t, svc.ThumbnailPath(missing))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var progress func(int, int)
			switch mode {
			case "cancelled":
				progress = func(done, _ int) {
					if done == 1 {
						cancel()
					}
				}
			case "unreachable":
				if err := os.Rename(svc.sourcePath, svc.sourcePath+"-offline"); err != nil {
					t.Fatal(err)
				}
			case "io_error":
				saveCleanupPhoto(t, svc, filepath.Join("z", "file.mp4"))
				if err := os.RemoveAll(filepath.Join(svc.sourcePath, "z")); err != nil {
					t.Fatal(err)
				}
				putCleanupFile(t, filepath.Join(svc.sourcePath, "z")) // ENOTDIR, never a deletion signal
			case "unreadable_link":
				if err := os.Symlink(filepath.Join(t.TempDir(), "unreachable.mp4"), filepath.Join(svc.sourcePath, missing.SourceRelPath)); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			count, err := svc.CleanMissingMediaContext(ctx, 1, progress)
			if err == nil || count != 0 {
				t.Fatalf("unsafe cleanup: count=%d err=%v", count, err)
			}
			if p, err := repo.GetPhotoByIDAny(missing.ID, 1); err != nil || p == nil {
				t.Fatalf("record lost: %v", err)
			}
			if _, err := os.Stat(svc.ThumbnailPath(missing)); err != nil {
				t.Fatalf("cache lost: %v", err)
			}
		})
	}
}

func TestIdentifiedMediaCacheRetainsLegacyUntilSourceChanges(t *testing.T) {
	svc, _ := cleanupService(t)
	p := saveCleanupPhoto(t, svc, "clip.mp4")
	path, err := svc.soundtrackCachePath(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path)[:37] != p.UUID+"-" {
		t.Fatalf("new cache has no owner: %s", path)
	}
	legacy := filepath.Join(filepath.Dir(path), filepath.Base(path)[37:])
	putCleanupFile(t, legacy)
	if got, err := svc.soundtrackCachePath(p, 0); err != nil || got != legacy {
		t.Fatalf("legacy cache is not reused: %s %v", got, err)
	}
	putCleanupFile(t, path)
	if got, err := svc.soundtrackCachePath(p, 0); err != nil || got != path {
		t.Fatalf("owned cache should win: %s %v", got, err)
	}
}

func TestCleanMissingMediaArtifactFailureCanBeRetried(t *testing.T) {
	svc, repo := cleanupService(t)
	p := saveCleanupPhoto(t, svc, "deleted.mp4")
	if err := os.Remove(svc.MediaPath(p)); err != nil {
		t.Fatal(err)
	}
	cache := svc.ThumbnailPath(p)
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	if count, err := svc.CleanMissingMediaContext(context.Background(), 1, nil); err == nil || count != 0 {
		t.Fatalf("artifact error must stop cleanup: %d %v", count, err)
	}
	if got, err := repo.GetPhotoByID(p.ID, 1); err != nil || got == nil {
		t.Fatalf("failed cleanup lost record: %+v %v", got, err)
	}
	if err := os.Remove(cache); err != nil {
		t.Fatal(err)
	}
	if count, err := svc.CleanMissingMediaContext(context.Background(), 1, nil); err != nil || count != 1 {
		t.Fatalf("retry failed: %d %v", count, err)
	}
}

func TestCleanMissingMediaRevalidatesBeforeRemovingArtifacts(t *testing.T) {
	for _, mode := range []string{"reappeared", "dangling_file", "linked_parent", "replaced_root", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo := cleanupService(t)
			p := saveCleanupPhoto(t, svc, "folder/deleted.mp4")
			path := svc.MediaPath(p)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			thumb := svc.ThumbnailPath(p)
			putCleanupFile(t, thumb)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			preflightDone, changed := false, false
			progress := func(done, total int) {
				if done == total && done > 0 {
					preflightDone = true
				}
				if !preflightDone || changed || done != 0 {
					return
				}
				changed = true
				switch mode {
				case "reappeared":
					putCleanupFile(t, path)
				case "dangling_file":
					if err := os.Symlink(filepath.Join(t.TempDir(), "offline.mp4"), path); err != nil {
						t.Fatal(err)
					}
				case "linked_parent":
					if err := os.Remove(filepath.Dir(path)); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(t.TempDir(), filepath.Dir(path)); err != nil {
						t.Fatal(err)
					}
				case "replaced_root":
					if err := os.Rename(svc.sourcePath, svc.sourcePath+"-offline"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(svc.sourcePath, 0755); err != nil {
						t.Fatal(err)
					}
				case "cancelled":
					cancel()
				}
			}
			count, err := svc.CleanMissingMediaContext(ctx, 1, progress)
			if count != 0 || !changed {
				t.Fatalf("cleanup count=%d mutation=%v error=%v", count, changed, err)
			}
			if mode == "reappeared" && err != nil {
				t.Fatal(err)
			}
			if mode != "reappeared" && err == nil {
				t.Fatal("ambiguous absence should stop cleanup")
			}
			if photo, err := repo.GetPhotoByIDAny(p.ID, 1); err != nil || photo == nil {
				t.Fatalf("lost record: %v", err)
			}
			if _, err := os.Stat(thumb); err != nil {
				t.Fatalf("lost cache: %v", err)
			}
		})
	}
}

func TestCleanMissingMediaPreservesAmbiguousFilenameVariants(t *testing.T) {
	for _, variant := range []string{"CAFÉ.mp4", "cafe\u0301.mp4"} {
		t.Run(variant, func(t *testing.T) {
			svc, repo := cleanupService(t)
			p := saveCleanupPhoto(t, svc, "café.mp4")
			if err := os.Remove(svc.MediaPath(p)); err != nil {
				t.Fatal(err)
			}
			putCleanupFile(t, filepath.Join(svc.sourcePath, variant))
			putCleanupFile(t, svc.ThumbnailPath(p))
			count, _ := svc.CleanMissingMediaContext(context.Background(), 1, nil)
			if count != 0 {
				t.Fatal("ambiguous filename removed")
			}
			if photo, err := repo.GetPhotoByIDAny(p.ID, 1); err != nil || photo == nil {
				t.Fatalf("lost record: %v", err)
			}
		})
	}
}
