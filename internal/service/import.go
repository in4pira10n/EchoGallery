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

const (
	folderAlbumDescription = "自动从文件夹导入"
	folderAlbumSourceKind  = "folder"
)

// ImportSummary 汇总一次启动扫描导入的结果。
type ImportSummary struct {
	Imported int
	Skipped  int
	Pruned   int
}

type importJob struct {
	path          string
	originalName  string
	sourceRelPath string
	info          fs.FileInfo
}

type sourceMoveJob struct {
	existing      storage.SourceMediaInfo
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
	var pendingJobs []importJob
	var relocatedPhotos []*storage.Photo
	sourceIndex, err := s.repo.ListSourceMediaIndex(uploadedBy)
	if err != nil {
		return summary, err
	}
	totalCandidates := 0
	indexSkippedCandidates := 0
	relocatedCandidates := 0
	seenSourcePaths := make(map[string]bool)
	seenFolderAlbums := make(map[string]bool)

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
		sourceRelPath = filepath.ToSlash(filepath.Clean(sourceRelPath))
		seenSourcePaths[sourceRelPath] = true
		for _, albumPath := range folderAlbumPathsForSourceRelPath(sourceRelPath) {
			seenFolderAlbums[albumPath] = true
		}
		totalCandidates++
		if progress != nil && (totalCandidates <= 12 || totalCandidates%32 == 0) {
			progress(totalCandidates, 0)
		}

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
		pendingJobs = append(pendingJobs, importJob{path: path, originalName: name, sourceRelPath: sourceRelPath, info: info})
		return nil
	})
	if err != nil {
		return summary, err
	}
	if progress != nil && totalCandidates > 0 {
		progress(totalCandidates, 0)
	}

	moveCandidates := buildMissingSourceFingerprintIndex(sourceIndex, seenSourcePaths)
	jobs := make([]importJob, 0, len(pendingJobs))
	for _, job := range pendingJobs {
		if _, ok := sourceIndex[job.sourceRelPath]; ok {
			jobs = append(jobs, job)
			continue
		}
		info := job.info
		if info == nil {
			info, err = os.Stat(job.path)
			if err != nil {
				return summary, err
			}
			job.info = info
		}
		key := sourceMediaFingerprint(info.Size(), info.ModTime().UnixNano())
		candidate, ok := pickMovedSourceCandidate(moveCandidates[key], job.originalName)
		if !ok {
			jobs = append(jobs, job)
			continue
		}
		photo, moveErr := s.relocateExistingSourceMedia(sourceMoveJob{
			existing:      candidate,
			path:          job.path,
			originalName:  job.originalName,
			sourceRelPath: job.sourceRelPath,
			info:          info,
		}, uploadedBy)
		if moveErr != nil {
			return summary, moveErr
		}
		relocatedPhotos = append(relocatedPhotos, photo)
		relocatedCandidates++
		delete(sourceIndex, candidate.SourceRelPath)
		sourceIndex[job.sourceRelPath] = storage.SourceMediaInfo{
			ID:            photo.ID,
			SourceRelPath: job.sourceRelPath,
			Size:          info.Size(),
			SourceModUnix: info.ModTime().UnixNano(),
		}
		remaining := moveCandidates[key]
		for i := range remaining {
			if remaining[i].ID != candidate.ID {
				continue
			}
			moveCandidates[key] = append(remaining[:i], remaining[i+1:]...)
			break
		}
	}

	pruned, err := s.pruneMissingSourceMedia(ctx, uploadedBy, sourceIndex, seenSourcePaths)
	if err != nil {
		return summary, err
	}
	summary.Pruned += pruned
	if err := s.ensureFolderAlbums(uploadedBy, seenFolderAlbums, albumCache); err != nil {
		return summary, err
	}
	if err := s.pruneMissingFolderAlbums(uploadedBy, seenFolderAlbums); err != nil {
		return summary, err
	}
	for _, photo := range relocatedPhotos {
		if err := s.attachImportedPhotoToFolderAlbum(photo, uploadedBy, albumCache); err != nil {
			return summary, err
		}
	}
	if err := s.repo.RefreshFolderAlbumCovers(uploadedBy); err != nil {
		return summary, err
	}
	summary.Skipped += relocatedCandidates

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
		importedPhotos []*storage.Photo
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
					cacheMu.Lock()
					importedPhotos = append(importedPhotos, photo)
					cacheMu.Unlock()
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
	if err := s.repo.RefreshFolderAlbumCovers(uploadedBy); err != nil {
		return summary, err
	}
	s.warmImportedThumbnails(importedPhotos)
	return summary, nil
}

func folderAlbumNameForSourceRelPath(sourceRelPath string) string {
	paths := folderAlbumPathsForSourceRelPath(sourceRelPath)
	if len(paths) == 0 {
		return ""
	}
	return paths[len(paths)-1]
}

func folderAlbumPathsForSourceRelPath(sourceRelPath string) []string {
	if sourceRelPath == "" {
		return nil
	}
	dir := filepath.Dir(sourceRelPath)
	if dir == "." || dir == "" {
		return nil
	}
	dir = filepath.ToSlash(filepath.Clean(dir))
	if dir == "." || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	paths := make([]string, 0, len(parts))
	for index := range parts {
		part := strings.TrimSpace(parts[index])
		if part == "" || part == "." {
			continue
		}
		paths = append(paths, strings.Join(parts[:index+1], "/"))
	}
	return paths
}

func (s *PhotoService) pruneMissingSourceMedia(ctx context.Context, uploadedBy int64, sourceIndex map[string]storage.SourceMediaInfo, seenSourcePaths map[string]bool) (int, error) {
	pruned := 0
	for sourceRelPath, existing := range sourceIndex {
		if err := ctx.Err(); err != nil {
			return pruned, err
		}
		if sourceRelPath == "" || seenSourcePaths[filepath.ToSlash(filepath.Clean(sourceRelPath))] {
			continue
		}
		if err := s.repo.HardDeletePhoto(existing.ID, uploadedBy); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}

func sourceMediaUnchanged(existing storage.SourceMediaInfo, info fs.FileInfo) bool {
	// 旧数据库没有 source_mod_unix；只要路径已存在，仍按旧逻辑快速跳过，避免升级后首次启动退化成逐文件查询。
	if existing.SourceModUnix == 0 {
		return true
	}
	return existing.Size == info.Size() && existing.SourceModUnix == info.ModTime().UnixNano()
}

func sourceMediaFingerprint(size int64, modUnix int64) string {
	return fmt.Sprintf("%d:%d", size, modUnix)
}

func buildMissingSourceFingerprintIndex(sourceIndex map[string]storage.SourceMediaInfo, seenSourcePaths map[string]bool) map[string][]storage.SourceMediaInfo {
	index := make(map[string][]storage.SourceMediaInfo)
	for sourceRelPath, item := range sourceIndex {
		if sourceRelPath == "" || seenSourcePaths[filepath.Clean(sourceRelPath)] {
			continue
		}
		if item.SourceModUnix == 0 {
			continue
		}
		key := sourceMediaFingerprint(item.Size, item.SourceModUnix)
		index[key] = append(index[key], item)
	}
	return index
}

func pickMovedSourceCandidate(candidates []storage.SourceMediaInfo, originalName string) (storage.SourceMediaInfo, bool) {
	if len(candidates) == 0 {
		return storage.SourceMediaInfo{}, false
	}
	cleanName := strings.TrimSpace(originalName)
	if cleanName != "" {
		for _, candidate := range candidates {
			if filepath.Base(candidate.SourceRelPath) == cleanName {
				return candidate, true
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return storage.SourceMediaInfo{}, false
}

func importWorkerCount() int {
	if lowResourceModeEnabled() {
		return 1
	}
	if batchAggressiveModeEnabled() {
		n := (runtime.GOMAXPROCS(0) * 3) / 4
		if n < 2 {
			return 2
		}
		if n > 8 {
			return 8
		}
		return n
	}
	n := runtime.GOMAXPROCS(0) / 2
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}

func (s *PhotoService) relocateExistingSourceMedia(job sourceMoveJob, uploadedBy int64) (*storage.Photo, error) {
	photo, err := s.repo.GetPhotoByIDAny(job.existing.ID, uploadedBy)
	if err != nil {
		return nil, err
	}
	if photo == nil {
		return nil, fmt.Errorf("待迁移媒体不存在: %s", job.existing.SourceRelPath)
	}
	info := job.info
	if info == nil {
		info, err = os.Stat(job.path)
		if err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdatePhotoSourceMedia(photo.ID, uploadedBy, job.sourceRelPath, job.originalName, info.Size(), info.ModTime().UnixNano()); err != nil {
		return nil, err
	}
	photo.SourceRelPath = job.sourceRelPath
	photo.OriginalName = job.originalName
	photo.Size = info.Size()
	photo.SourceModUnix = info.ModTime().UnixNano()
	return photo, nil
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
			EXIF:          copyPhotoEXIF(meta.EXIF),
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
		EXIF:          videoMetaEXIF(videoMeta),
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
		if album == nil {
			continue
		}
		if path := autoFolderAlbumPath(album); path != "" {
			cache[path] = album.ID
			continue
		}
		cache[album.Name] = album.ID
	}
	return cache, nil
}

func (s *PhotoService) attachImportedPhotoToFolderAlbum(photo *storage.Photo, uploadedBy int64, albumCache map[string]int64) error {
	if photo == nil || photo.SourceRelPath == "" {
		return nil
	}
	albumPaths := folderAlbumPathsForSourceRelPath(photo.SourceRelPath)
	if len(albumPaths) == 0 {
		return nil
	}
	leafAlbumID := int64(0)
	for _, albumPath := range albumPaths {
		albumID, ok := albumCache[albumPath]
		if !ok {
			album := &storage.Album{
				Name:          albumPath,
				Description:   folderAlbumDescription,
				SourceKind:    folderAlbumSourceKind,
				SourceRelPath: albumPath,
				CreatedBy:     uploadedBy,
				CreatedAt:     time.Now(),
			}
			if err := s.repo.CreateAlbum(album); err != nil {
				return err
			}
			albumID = album.ID
			albumCache[albumPath] = albumID
		}
		leafAlbumID = albumID
	}
	if leafAlbumID == 0 {
		return nil
	}
	albums, err := s.repo.ListAlbumsForPhoto(photo.ID, uploadedBy)
	if err != nil {
		return err
	}
	leafPath := albumPaths[len(albumPaths)-1]
	for _, album := range albums {
		if !isAutoFolderAlbum(album) {
			continue
		}
		if autoFolderAlbumPath(album) == leafPath {
			continue
		}
		if err := s.repo.RemovePhotoFromAlbum(album.ID, photo.ID, uploadedBy); err != nil {
			return err
		}
	}
	return s.repo.AddPhotoToAlbum(leafAlbumID, photo.ID, uploadedBy)
}

func (s *PhotoService) ensureFolderAlbums(uploadedBy int64, seenFolderAlbums map[string]bool, albumCache map[string]int64) error {
	for albumPath := range seenFolderAlbums {
		albumPath = filepath.ToSlash(filepath.Clean(albumPath))
		if albumPath == "." || albumPath == "" {
			continue
		}
		if _, ok := albumCache[albumPath]; ok {
			continue
		}
		album := &storage.Album{
			Name:          albumPath,
			Description:   folderAlbumDescription,
			SourceKind:    folderAlbumSourceKind,
			SourceRelPath: albumPath,
			CreatedBy:     uploadedBy,
			CreatedAt:     time.Now(),
		}
		if err := s.repo.CreateAlbum(album); err != nil {
			return err
		}
		albumCache[albumPath] = album.ID
	}
	return nil
}

func (s *PhotoService) pruneMissingFolderAlbums(uploadedBy int64, seenFolderAlbums map[string]bool) error {
	albums, err := s.repo.ListAlbums(uploadedBy)
	if err != nil {
		return err
	}
	for _, album := range albums {
		if !isAutoFolderAlbum(album) {
			if err := s.repo.DeleteAlbum(album.ID, uploadedBy); err != nil {
				return err
			}
			continue
		}
		albumPath := autoFolderAlbumPath(album)
		if albumPath == "" || seenFolderAlbums[albumPath] {
			continue
		}
		if err := s.repo.DeleteAlbum(album.ID, uploadedBy); err != nil {
			return err
		}
	}
	return nil
}

func isAutoFolderAlbum(album *storage.Album) bool {
	if album == nil {
		return false
	}
	if album.SourceKind == folderAlbumSourceKind {
		return true
	}
	return album.SourceKind == "" && album.Description == folderAlbumDescription
}

func autoFolderAlbumPath(album *storage.Album) string {
	if album == nil {
		return ""
	}
	if album.SourceRelPath != "" {
		return filepath.ToSlash(filepath.Clean(album.SourceRelPath))
	}
	if album.Description == folderAlbumDescription {
		return filepath.ToSlash(filepath.Clean(album.Name))
	}
	return ""
}
