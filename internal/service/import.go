package service

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

// ImportSummary 汇总一次启动扫描导入的结果。
type ImportSummary struct {
	Imported int
	Skipped  int
}

type importJob struct {
	path         string
	originalName string
}

// ImportExistingPhotos 扫描 storagePath 中现有的图片文件并导入数据库。
// 导入时只建立索引，不在启动阶段预生成缩略图；缩略图在访问时按需生成。
func (s *PhotoService) ImportExistingPhotos(uploadedBy int64, progress func(done, total int)) (*ImportSummary, error) {
	summary := &ImportSummary{}
	albumCache, err := s.loadAlbumCache(uploadedBy)
	if err != nil {
		return summary, err
	}
	var jobs []importJob

	err = filepath.WalkDir(s.sourcePath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == s.sourcePath {
			return nil
		}

		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if strings.HasPrefix(name, ".") {
			summary.Skipped++
			return nil
		}
		if !imgpkg.SupportedMimeTypes[imgpkg.DetectMimeType(name)] && !media.IsSupportedVideoFilename(name) {
			summary.Skipped++
			return nil
		}
		jobs = append(jobs, importJob{path: path, originalName: name})
		return nil
	})
	if err != nil {
		return summary, err
	}

	if len(jobs) == 0 {
		if progress != nil {
			progress(0, 0)
		}
		return summary, nil
	}

	var (
		importedCount  int64
		skippedCount   int64
		processedCount int64
		cacheMu        sync.Mutex
		progressMu     sync.Mutex
		errCh          = make(chan error, 1)
		jobCh          = make(chan importJob)
		wg             sync.WaitGroup
	)

	workerCount := importWorkerCount()
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for job := range jobCh {
				photo, imported, jobErr := s.importExistingPhotoFile(job.path, job.originalName, uploadedBy)
				if jobErr != nil {
					select {
					case errCh <- jobErr:
					default:
					}
					return
				}
				cacheMu.Lock()
				jobErr = s.attachImportedPhotoToFolderAlbum(photo, uploadedBy, albumCache)
				cacheMu.Unlock()
				if jobErr != nil {
					select {
					case errCh <- jobErr:
					default:
					}
					return
				}
				if imported {
					atomic.AddInt64(&importedCount, 1)
				} else {
					atomic.AddInt64(&skippedCount, 1)
				}
				if progress != nil {
					done := int(atomic.AddInt64(&processedCount, 1))
					if done == len(jobs) || done%10 == 0 {
						progressMu.Lock()
						progress(done, len(jobs))
						progressMu.Unlock()
					}
				}
			}
		}()
	}
	for _, job := range jobs {
		select {
		case err := <-errCh:
			close(jobCh)
			wg.Wait()
			return summary, err
		case jobCh <- job:
		}
	}
	close(jobCh)
	wg.Wait()

	select {
	case err := <-errCh:
		return summary, err
	default:
	}

	summary.Imported += int(importedCount)
	summary.Skipped += int(skippedCount)
	return summary, nil
}

func importWorkerCount() int {
	n := runtime.GOMAXPROCS(0)
	if n < 2 {
		return 2
	}
	if n > 8 {
		return 8
	}
	return n
}

func (s *PhotoService) importExistingPhotoFile(path, originalName string, uploadedBy int64) (*storage.Photo, bool, error) {
	sourceRelPath, err := filepath.Rel(s.sourcePath, path)
	if err != nil {
		return nil, false, err
	}
	sourceRelPath = filepath.Clean(sourceRelPath)

	if existing, err := s.repo.GetPhotoBySourceRelPath(sourceRelPath, uploadedBy); err != nil {
		return nil, false, err
	} else if existing != nil {
		return existing, false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	mimeType := imgpkg.DetectMimeType(originalName)
	if imgpkg.SupportedMimeTypes[mimeType] {
		meta, err := imgpkg.ExtractMeta(file, originalName, info.ModTime())
		_ = file.Close()
		if err != nil {
			return nil, false, fmt.Errorf("解析图片失败 %s: %w", path, err)
		}

		photoUUID := uuid.New().String()
		photo := &storage.Photo{
			UUID:          photoUUID,
			OriginalName:  originalName,
			MediaKind:     storage.MediaKindImage,
			MimeType:      meta.MimeType,
			Size:          info.Size(),
			Width:         meta.Width,
			Height:        meta.Height,
			SourceRelPath: sourceRelPath,
			TakenAt:       meta.TakenAt,
			UploadedAt:    time.Now(),
			UploadedBy:    uploadedBy,
		}

		if err := s.repo.SavePhoto(photo); err != nil {
			return nil, false, fmt.Errorf("保存图片记录失败: %w", err)
		}

		return photo, true, nil
	}
	_ = file.Close()

	videoMeta, err := media.ProbeVideo(path)
	if err != nil {
		videoMeta = &media.VideoMeta{FormatName: strings.TrimPrefix(filepath.Ext(originalName), ".")}
	}
	photoUUID := uuid.New().String()
	photo := &storage.Photo{
		UUID:          photoUUID,
		OriginalName:  originalName,
		MediaKind:     storage.MediaKindVideo,
		MimeType:      media.DetectVideoMimeType(originalName),
		Size:          info.Size(),
		Width:         videoMeta.Width,
		Height:        videoMeta.Height,
		DurationMS:    videoMeta.DurationMS,
		SourceRelPath: sourceRelPath,
		TakenAt:       info.ModTime(),
		UploadedAt:    time.Now(),
		UploadedBy:    uploadedBy,
	}

	if err := s.repo.SavePhoto(photo); err != nil {
		return nil, false, fmt.Errorf("保存视频记录失败: %w", err)
	}
	return photo, true, nil
}

func (s *PhotoService) loadAlbumCache(uploadedBy int64) (map[string]int64, error) {
	albums, err := s.repo.ListAlbums(uploadedBy)
	if err != nil {
		return nil, err
	}
	cache := make(map[string]int64, len(albums))
	for _, album := range albums {
		cache[album.Name] = album.ID
	}
	return cache, nil
}

func (s *PhotoService) attachImportedPhotoToFolderAlbum(photo *storage.Photo, uploadedBy int64, albumCache map[string]int64) error {
	if photo == nil || photo.SourceRelPath == "" {
		return nil
	}
	dir := filepath.Dir(photo.SourceRelPath)
	if dir == "." || dir == "" {
		return nil
	}

	albumName := filepath.ToSlash(dir)
	albumID, ok := albumCache[albumName]
	if !ok {
		album := &storage.Album{
			Name:        albumName,
			Description: "自动从文件夹导入",
			CreatedBy:   uploadedBy,
			CreatedAt:   time.Now(),
		}
		if err := s.repo.CreateAlbum(album); err != nil {
			return err
		}
		albumID = album.ID
		albumCache[albumName] = albumID
	}
	return s.repo.AddPhotoToAlbum(albumID, photo.ID, uploadedBy)
}
