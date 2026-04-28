package service

import (
	"context"
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
	path          string
	originalName  string
	sourceRelPath string
	info          fs.FileInfo
}

// ImportExistingPhotos 扫描 storagePath 中现有的图片文件并导入数据库。
// 导入时只建立索引，不在启动阶段预生成缩略图；缩略图在访问时按需生成。
func (s *PhotoService) ImportExistingPhotos(uploadedBy int64, progress func(done, total int)) (*ImportSummary, error) {
	return s.ImportExistingPhotosContext(context.Background(), uploadedBy, progress)
}

// ImportExistingPhotosContext 扫描 storagePath 中现有的图片文件并导入数据库。
// ctx 用于让启动扫描在 Ctrl+C 或网页退出时尽快停下，避免大资源库后台任务拖住进程。
func (s *PhotoService) ImportExistingPhotosContext(ctx context.Context, uploadedBy int64, progress func(done, total int)) (*ImportSummary, error) {
	summary := &ImportSummary{}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	albumCache, err := s.loadAlbumCache(uploadedBy)
	if err != nil {
		return summary, err
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	var jobs []importJob
	sourceIndex, err := s.repo.ListSourceMediaIndex(uploadedBy)
	if err != nil {
		return summary, err
	}
	totalCandidates := 0
	indexSkippedCandidates := 0

	err = filepath.WalkDir(s.sourcePath, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		sourceRelPath, err := filepath.Rel(s.sourcePath, path)
		if err != nil {
			return err
		}
		sourceRelPath = filepath.Clean(sourceRelPath)
		totalCandidates++

		var info fs.FileInfo
		if existing, ok := sourceIndex[sourceRelPath]; ok {
			if existing.SourceModUnix == 0 {
				summary.Skipped++
				indexSkippedCandidates++
				return nil
			}
			info, err = d.Info()
			if err != nil {
				return err
			}
			if sourceMediaUnchanged(existing, info) {
				summary.Skipped++
				indexSkippedCandidates++
				return nil
			}
		}
		jobs = append(jobs, importJob{path: path, originalName: name, sourceRelPath: sourceRelPath, info: info})
		return nil
	})
	if err != nil {
		return summary, err
	}

	if len(jobs) == 0 {
		if progress != nil {
			progress(totalCandidates, totalCandidates)
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
				if ctx.Err() != nil {
					return
				}
				photo, imported, jobErr := s.importExistingPhotoFile(job, uploadedBy)
				if jobErr != nil {
					atomic.AddInt64(&skippedCount, 1)
					fmt.Fprintf(os.Stderr, "警告: 跳过导入失败的文件 %s: %v\n", job.path, jobErr)
					if progress != nil {
						done := int(atomic.AddInt64(&processedCount, 1))
						if done == len(jobs) || done%10 == 0 {
							progressMu.Lock()
							progress(indexSkippedCandidates+done, totalCandidates)
							progressMu.Unlock()
						}
					}
					continue
				}
				if ctx.Err() != nil {
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
						progress(indexSkippedCandidates+done, totalCandidates)
						progressMu.Unlock()
					}
				}
			}
		}()
	}
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			close(jobCh)
			wg.Wait()
			return summary, ctx.Err()
		case err := <-errCh:
			close(jobCh)
			wg.Wait()
			return summary, err
		case jobCh <- job:
		}
	}
	close(jobCh)
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return summary, err
	}
	select {
	case err := <-errCh:
		return summary, err
	default:
	}

	summary.Imported += int(importedCount)
	summary.Skipped += int(skippedCount)
	return summary, nil
}

func sourceMediaUnchanged(existing storage.SourceMediaInfo, info fs.FileInfo) bool {
	// 旧数据库没有 source_mod_unix；只要路径已存在，仍按旧逻辑快速跳过，避免升级后首次启动退化成逐文件查询。
	if existing.SourceModUnix == 0 {
		return true
	}
	return existing.Size == info.Size() && existing.SourceModUnix == info.ModTime().UnixNano()
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

func (s *PhotoService) importExistingPhotoFile(job importJob, uploadedBy int64) (*storage.Photo, bool, error) {
	if existing, err := s.repo.GetPhotoBySourceRelPath(job.sourceRelPath, uploadedBy); err != nil {
		return nil, false, err
	} else if existing != nil {
		return existing, false, nil
	}

	info := job.info
	if info == nil {
		var err error
		info, err = os.Stat(job.path)
		if err != nil {
			return nil, false, err
		}
	}

	file, err := os.Open(job.path)
	if err != nil {
		return nil, false, err
	}
	mimeType := imgpkg.DetectMimeType(job.originalName)
	if imgpkg.SupportedMimeTypes[mimeType] {
		meta, err := imgpkg.ExtractMeta(file, job.originalName, info.ModTime())
		_ = file.Close()
		if err != nil {
			return nil, false, fmt.Errorf("解析图片失败 %s: %w", job.path, err)
		}

		photoUUID := uuid.New().String()
		photo := &storage.Photo{
			UUID:          photoUUID,
			OriginalName:  job.originalName,
			MediaKind:     storage.MediaKindImage,
			MimeType:      meta.MimeType,
			Size:          info.Size(),
			Width:         meta.Width,
			Height:        meta.Height,
			SourceRelPath: job.sourceRelPath,
			SourceModUnix: info.ModTime().UnixNano(),
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

	videoMeta, err := media.ProbeVideo(job.path)
	if err != nil {
		videoMeta = &media.VideoMeta{FormatName: strings.TrimPrefix(filepath.Ext(job.originalName), ".")}
	}
	photoUUID := uuid.New().String()
	photo := &storage.Photo{
		UUID:          photoUUID,
		OriginalName:  job.originalName,
		MediaKind:     storage.MediaKindVideo,
		MimeType:      media.DetectVideoMimeType(job.originalName),
		Size:          info.Size(),
		Width:         videoMeta.Width,
		Height:        videoMeta.Height,
		DurationMS:    videoMeta.DurationMS,
		SourceRelPath: job.sourceRelPath,
		SourceModUnix: info.ModTime().UnixNano(),
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
