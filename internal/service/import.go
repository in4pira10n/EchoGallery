package service

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
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
	folderAlbumDescription     = "自动从文件夹导入"
	folderAlbumSourceKind      = "folder"
	rootFolderAlbumKey         = "__echogallery_root__"
	rootFolderAlbumName        = "根目录"
	libraryScanSnapshotVersion = 2
)

var errSnapshotMismatch = errors.New("library scan snapshot mismatch")

// ImportSummary 汇总一次启动扫描导入的结果。
type ImportSummary struct {
	// Changed covers moves and metadata updates as well as additions/deletions.
	Changed  bool
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

type libraryDirectorySnapshot struct {
	mediaEntries         []scanMediaEntry
	rootInfo             fs.FileInfo
	directoryModTimes    map[string]int64
	directoryEntryHashes map[string]string
	directoryMediaCounts map[string]int
	directoryMediaHashes map[string]string
	changedDirectories   []string
	dirCount             int
	dirModHash           string
	mediaFileHash        string
	mediaFileCount       int
}

// Background checks share the disk with interactive media reads. Keep their
// metadata import serial; explicit batch builds retain their configured workers.
type backgroundLibraryScanKey struct{}

func WithBackgroundLibraryScan(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundLibraryScanKey{}, true)
}

func backgroundLibraryScan(ctx context.Context) bool {
	background, _ := ctx.Value(backgroundLibraryScanKey{}).(bool)
	return background
}

// ImportExistingPhotos 扫描 storagePath 中现有的图片文件并导入数据库。
// 导入时只建立索引，不在启动阶段预生成缩略图；缩略图在访问时按需生成。
func (s *PhotoService) ImportExistingPhotos(uploadedBy int64, progress func(done, total int)) (*ImportSummary, error) {
	return s.ImportExistingPhotosContext(context.Background(), uploadedBy, progress)
}

// ImportExistingPhotosContext 扫描 storagePath 中现有的图片文件并导入数据库。
// ctx 用于让启动扫描在 Ctrl+C 或网页退出时尽快停下，避免大资源库后台任务拖住进程。
func (s *PhotoService) ImportExistingPhotosContext(ctx context.Context, uploadedBy int64, progress func(done, total int)) (*ImportSummary, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-s.workerStop:
			cancel()
		case <-ctx.Done():
		}
	}()
	reportScan(ctx, "waiting", "scan_lock", s.sourcePath, 0, 0, 0)
	for !s.scanMu.TryLock() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
	}
	defer s.scanMu.Unlock()
	s.lifecycleMu.Lock()
	closing := s.closing
	s.lifecycleMu.Unlock()
	if closing {
		return nil, fmt.Errorf("资源库正在关闭，请重新进入")
	}
	uploadedBy = s.LibraryUserID(uploadedBy)
	summary := &ImportSummary{}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	reportScan(ctx, "checking", "metadata", s.sourcePath, 0, 0, 0)
	rootInfo, err := scanStat(ctx, s.sourcePath)
	if err != nil {
		return summary, err
	}
	if !rootInfo.IsDir() {
		return summary, fmt.Errorf("资源库路径不是目录: %s", s.sourcePath)
	}
	previousSnapshot, snapshotErr := s.repo.GetLibraryScanSnapshot()
	if snapshotErr == nil && !s.IsSMBLibrary() && quickLibraryScanSnapshotMatches(ctx, previousSnapshot, s.sourcePath, rootInfo) {
		summary.Skipped = previousSnapshot.FileCount
		if progress != nil {
			progress(previousSnapshot.FileCount, previousSnapshot.FileCount)
		}
		return summary, nil
	} else if snapshotErr != nil {
		return summary, snapshotErr
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	reportScan(ctx, "discovering", "metadata", s.sourcePath, 0, 0, 0)
	var dirSnapshot libraryDirectorySnapshot
	if s.IsSMBLibrary() {
		dirSnapshot, err = collectSMBLibrarySnapshot(ctx, s.sourcePath, previousSnapshot)
	} else {
		dirSnapshot, err = collectLibraryDirectorySnapshot(ctx, s.sourcePath, previousSnapshot)
	}
	if err != nil {
		return summary, err
	}
	reportScan(ctx, "indexing", "database", s.sourcePath, -1, 0, 0)
	albumCache, err := s.loadAlbumCache(uploadedBy)
	if err != nil {
		return summary, err
	}
	deletedFolderPaths := make(map[string]bool)
	if trashRepo, ok := s.repo.(folderAlbumTrashRepository); ok {
		deletedFolderPaths, err = trashRepo.ListTrashedFolderAlbumPaths(uploadedBy)
		if err != nil {
			return summary, err
		}
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
	changedDirs := changedLibraryDirectories(previousSnapshot, dirSnapshot)
	summary.Changed = len(changedDirs) > 0
	for sourceRelPath := range sourceIndex {
		parent := filepath.ToSlash(filepath.Dir(sourceRelPath))
		if parent == "" {
			parent = "."
		}
		if _, exists := dirSnapshot.directoryModTimes[parent]; !exists {
			continue
		}
		if changedDirs[parent] {
			continue
		}
		seenSourcePaths[sourceRelPath] = true
		for _, albumPath := range folderAlbumPathsForSourceRelPath(sourceRelPath) {
			seenFolderAlbums[albumPath] = true
		}
		if len(folderAlbumPathsForSourceRelPath(sourceRelPath)) == 0 {
			seenFolderAlbums[rootFolderAlbumKey] = true
		}
		indexSkippedCandidates++
	}
	totalCandidates = indexSkippedCandidates
	summary.Skipped = indexSkippedCandidates

	visitMedia := func(path string, d fs.DirEntry, walkErr error) error {
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
			if path != s.sourcePath {
				rel, relErr := filepath.Rel(s.sourcePath, path)
				if relErr != nil {
					return relErr
				}
				rel = filepath.ToSlash(filepath.Clean(rel))
				if isUnderDeletedFolderPath(rel, deletedFolderPaths) {
					return filepath.SkipDir
				}
			}
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
		parentRel := filepath.ToSlash(filepath.Dir(sourceRelPath))
		if parentRel == "" {
			parentRel = "."
		}
		if !changedDirs[parentRel] {
			if _, known := sourceIndex[sourceRelPath]; !known {
				// A stable directory can still contain a media record that was
				// missing from SQLite after an interrupted or failed import.
				// Recheck unknown paths instead of trusting the directory cache.
			} else {
				return nil
			}
		}
		if isUnderDeletedFolderPath(sourceRelPath, deletedFolderPaths) {
			summary.Skipped++
			return nil
		}
		seenSourcePaths[sourceRelPath] = true
		for _, albumPath := range folderAlbumPathsForSourceRelPath(sourceRelPath) {
			seenFolderAlbums[albumPath] = true
		}
		if len(folderAlbumPathsForSourceRelPath(sourceRelPath)) == 0 {
			seenFolderAlbums[rootFolderAlbumKey] = true
		}
		totalCandidates++
		if progress != nil && (totalCandidates <= 12 || totalCandidates%32 == 0) {
			progress(totalCandidates, 0)
		}

		var info fs.FileInfo
		if existing, ok := sourceIndex[sourceRelPath]; ok {
			if existing.SourceModUnix == 0 && !existing.Deleted {
				summary.Skipped++
				indexSkippedCandidates++
				return nil
			}
			info, err = d.Info()
			if err != nil {
				return err
			}
			if existing.SourceModUnix == 0 {
				if existing.Deleted && sourceFileChangedAfterDeletion(info, existing.DeletedAtUnixNano) {
					pendingJobs = append(pendingJobs, importJob{path: path, originalName: name, sourceRelPath: sourceRelPath, info: info})
					return nil
				}
				summary.Skipped++
				indexSkippedCandidates++
				return nil
			}
			if sourceMediaUnchanged(existing, info) {
				if existing.Deleted && sourceFileChangedAfterDeletion(info, existing.DeletedAtUnixNano) {
					// A file deleted in the app may still be present on disk. Seeing it
					// again during a library scan means it has been restored externally.
					pendingJobs = append(pendingJobs, importJob{path: path, originalName: name, sourceRelPath: sourceRelPath, info: info})
					return nil
				}
				summary.Skipped++
				indexSkippedCandidates++
				return nil
			}
		}
		pendingJobs = append(pendingJobs, importJob{path: path, originalName: name, sourceRelPath: sourceRelPath, info: info})
		return nil
	}
	if s.IsSMBLibrary() {
		for _, entry := range dirSnapshot.mediaEntries {
			if err = visitMedia(entry.path, entry.entry, nil); err != nil {
				break
			}
		}
		dirSnapshot.mediaEntries = nil
	} else {
		err = walkScanDir(ctx, s.sourcePath, visitMedia)
	}
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
			info, err = scanStat(ctx, job.path)
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
	if err := s.ensureFolderAlbums(uploadedBy, seenFolderAlbums, albumCache, deletedFolderPaths); err != nil {
		return summary, err
	}
	if err := s.pruneMissingFolderAlbums(uploadedBy, seenFolderAlbums); err != nil {
		return summary, err
	}
	for _, photo := range relocatedPhotos {
		if err := s.attachImportedPhotoToFolderAlbum(photo, uploadedBy, albumCache, deletedFolderPaths); err != nil {
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
		dirSnapshot.changedDirectories = changedDirectoryList(changedDirs)
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		reportScan(ctx, "saving", "save", s.sourcePath, -1, 0, 0)
		if err := s.saveLibraryScanSnapshot(dirSnapshot, totalCandidates); err != nil {
			return summary, err
		}
		return summary, nil
	}

	reportScan(ctx, "importing", "import_media", s.sourcePath, -1, 0, 0)
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
	if backgroundLibraryScan(ctx) {
		workerCount = 1
	}
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for job := range jobCh {
				if ctx.Err() != nil {
					return
				}
				reportScan(ctx, "importing", "import_media", job.path, -1, 0, 0)
				photo, imported, jobErr := s.importExistingPhotoFileContext(ctx, job, uploadedBy)
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
				jobErr = s.attachImportedPhotoToFolderAlbum(photo, uploadedBy, albumCache, deletedFolderPaths)
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
	dirSnapshot.changedDirectories = changedDirectoryList(changedDirs)
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	reportScan(ctx, "saving", "save", s.sourcePath, -1, 0, 0)
	if err := s.saveLibraryScanSnapshot(dirSnapshot, totalCandidates); err != nil {
		return summary, err
	}
	s.warmImportedThumbnails(importedPhotos)
	return summary, nil
}

// NeedsLibraryBuild reports whether the directory snapshot requires a scan.
// It keeps normal application startup from showing a build screen when the
// library has not changed since the last successful scan.
func (s *PhotoService) NeedsLibraryBuild(ctx context.Context) (bool, error) {
	if s == nil || s.repo == nil {
		return false, fmt.Errorf("媒体服务不可用")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	reportScan(ctx, "checking", "metadata", s.sourcePath, 0, 0, 0)
	rootInfo, err := scanStat(ctx, s.sourcePath)
	if err != nil {
		return false, err
	}
	if !rootInfo.IsDir() {
		return false, fmt.Errorf("资源库路径不是目录: %s", s.sourcePath)
	}
	snapshot, err := s.repo.GetLibraryScanSnapshot()
	if err != nil {
		return false, err
	}
	return !quickLibraryScanSnapshotMatches(ctx, snapshot, s.sourcePath, rootInfo), nil
}

func (s *PhotoService) saveLibraryScanSnapshot(snapshot libraryDirectorySnapshot, fileCount int) error {
	if snapshot.rootInfo == nil {
		return nil
	}
	return s.repo.SaveLibraryScanSnapshot(storage.LibraryScanSnapshot{
		Version:              libraryScanSnapshotVersion,
		RootPath:             filepath.Clean(s.sourcePath),
		RootModUnixNano:      snapshot.rootInfo.ModTime().UnixNano(),
		DirectoryModTimes:    snapshot.directoryModTimes,
		DirectoryEntryHashes: snapshot.directoryEntryHashes,
		DirectoryMediaCounts: snapshot.directoryMediaCounts,
		DirectoryMediaHashes: snapshot.directoryMediaHashes,
		ChangedDirectories:   snapshot.changedDirectories,
		DirectoryCount:       snapshot.dirCount,
		DirectoryModHash:     snapshot.dirModHash,
		MediaFileHash:        snapshot.mediaFileHash,
		FileCount:            fileCount,
		CompletedAt:          time.Now(),
	})
}

func collectLibraryDirectorySnapshot(ctx context.Context, sourcePath string, previous *storage.LibraryScanSnapshot) (libraryDirectorySnapshot, error) {
	snapshot := libraryDirectorySnapshot{
		directoryModTimes:    make(map[string]int64),
		directoryEntryHashes: make(map[string]string),
		directoryMediaCounts: make(map[string]int),
		directoryMediaHashes: make(map[string]string),
	}
	rootInfo, err := scanStat(ctx, sourcePath)
	if err != nil {
		return snapshot, err
	}
	snapshot.rootInfo = rootInfo
	if !rootInfo.IsDir() {
		return snapshot, nil
	}
	dirHasher := fnv.New64a()
	mediaHasher := fnv.New64a()
	stableDirs := stableLibraryDirectories(ctx, sourcePath, previous)
	entryHashers := make(map[string]hash.Hash64)
	mediaHashers := make(map[string]hash.Hash64)
	err = walkScanDir(ctx, sourcePath, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != sourcePath && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			relPath := "."
			if path != sourcePath {
				rel, err := filepath.Rel(sourcePath, path)
				if err != nil {
					return err
				}
				relPath = filepath.ToSlash(filepath.Clean(rel))
				parentRel := filepath.ToSlash(filepath.Dir(relPath))
				if parentRel == "" {
					parentRel = "."
				}
				if hasher, ok := entryHashers[parentRel]; ok {
					_, _ = fmt.Fprintf(hasher, "d:%s\x00", d.Name())
				}
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			snapshot.dirCount++
			snapshot.directoryModTimes[relPath] = info.ModTime().UnixNano()
			if count, ok := previousDirectoryMediaCount(previous, relPath); ok && stableDirs[relPath] {
				snapshot.directoryMediaCounts[relPath] = count
				snapshot.mediaFileCount += count
			}
			if stableDirs[relPath] {
				if previous != nil {
					snapshot.directoryEntryHashes[relPath] = previous.DirectoryEntryHashes[relPath]
					snapshot.directoryMediaHashes[relPath] = previous.DirectoryMediaHashes[relPath]
				}
				// A directory's own entries may be stable while a descendant changed.
				// Keep visiting directories, but skip metadata reads for its media below.
				return nil
			}
			entryHashers[relPath] = fnv.New64a()
			mediaHashers[relPath] = fnv.New64a()
			_, _ = fmt.Fprintf(dirHasher, "%s\x00%d\x00", relPath, info.ModTime().UnixNano())
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || !d.Type().IsRegular() {
			return nil
		}
		if !imgpkg.SupportedMimeTypes[imgpkg.DetectMimeType(name)] && !media.IsSupportedVideoFilename(name) {
			return nil
		}
		parentRel, err := filepath.Rel(sourcePath, filepath.Dir(path))
		if err != nil {
			return err
		}
		parentRel = filepath.ToSlash(filepath.Clean(parentRel))
		if stableDirs[parentRel] {
			return nil
		}
		_, _ = fmt.Fprintf(entryHashers[parentRel], "f:%s\x00", name)
		info, err := d.Info()
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(filepath.Clean(relPath))
		snapshot.mediaFileCount++
		snapshot.directoryMediaCounts[parentRel]++
		_, _ = fmt.Fprintf(mediaHasher, "%s\x00%d\x00%d\x00", relPath, info.Size(), info.ModTime().UnixNano())
		_, _ = fmt.Fprintf(mediaHashers[parentRel], "%s\x00%d\x00%d\x00", name, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return snapshot, err
	}
	snapshot.dirModHash = fmt.Sprintf("%016x", dirHasher.Sum64())
	snapshot.mediaFileHash = fmt.Sprintf("%016x", mediaHasher.Sum64())
	for relPath, hasher := range entryHashers {
		snapshot.directoryEntryHashes[relPath] = fmt.Sprintf("%016x", hasher.Sum64())
	}
	for relPath, hasher := range mediaHashers {
		if _, stable := stableDirs[relPath]; stable {
			continue
		}
		snapshot.directoryMediaHashes[relPath] = fmt.Sprintf("%016x", hasher.Sum64())
	}
	return snapshot, nil
}

func previousDirectoryMediaCount(snapshot *storage.LibraryScanSnapshot, relPath string) (int, bool) {
	if snapshot == nil || snapshot.DirectoryMediaCounts == nil {
		return 0, false
	}
	count, ok := snapshot.DirectoryMediaCounts[relPath]
	return count, ok
}

func stableLibraryDirectories(ctx context.Context, sourcePath string, previous *storage.LibraryScanSnapshot) map[string]bool {
	stable := make(map[string]bool)
	if previous == nil || previous.Version != libraryScanSnapshotVersion || len(previous.DirectoryModTimes) == 0 || len(previous.DirectoryEntryHashes) == 0 {
		return stable
	}
	_ = walkLibraryDirectories(ctx, sourcePath, func(path string) ([]fs.DirEntry, error) { return readScanDir(ctx, path) }, func(path string, d fs.DirEntry, entries []fs.DirEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != sourcePath && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		relPath := "."
		if path != sourcePath {
			rel, err := filepath.Rel(sourcePath, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(filepath.Clean(rel))
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if previous.DirectoryModTimes[relPath] != info.ModTime().UnixNano() {
			return nil
		}
		if libraryDirectoryEntriesMatch(previous, relPath, entries) {
			stable[relPath] = true
		}
		return nil
	})
	return stable
}

// changedLibraryDirectories identifies directories whose direct entries or
// modification time changed since the previous scan. Files in stable
// directories can be trusted to have the same source paths and are skipped
// without opening or stat'ing each media file.
func changedLibraryDirectories(previous *storage.LibraryScanSnapshot, current libraryDirectorySnapshot) map[string]bool {
	changed := make(map[string]bool, len(current.directoryModTimes))
	if previous == nil || previous.Version != libraryScanSnapshotVersion {
		for relPath := range current.directoryModTimes {
			changed[relPath] = true
		}
		return changed
	}
	for relPath, modTime := range current.directoryModTimes {
		if previous.DirectoryModTimes[relPath] != modTime || previous.DirectoryEntryHashes[relPath] != current.directoryEntryHashes[relPath] {
			changed[relPath] = true
		}
	}
	for relPath := range previous.DirectoryModTimes {
		if _, ok := current.directoryModTimes[relPath]; ok {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(relPath))
		if parent == "" {
			parent = "."
		}
		changed[parent] = true
	}
	return changed
}

func changedDirectoryList(changed map[string]bool) []string {
	result := make([]string, 0, len(changed))
	for path, value := range changed {
		if value {
			result = append(result, filepath.ToSlash(filepath.Clean(path)))
		}
	}
	sort.Strings(result)
	return result
}

// Enumerate each directory once, reusing its entries for validation and child
// traversal. WalkDir plus an extra ReadDir doubled HDD directory enumeration.
// Iteration is serial and retains only pending directory entries, not media.
func walkLibraryDirectories(ctx context.Context, root string, readDir func(string) ([]fs.DirEntry, error), visit func(string, fs.DirEntry, []fs.DirEntry) error) error {
	info, err := scanStat(ctx, root)
	if err != nil {
		return err
	}
	type directory struct {
		path  string
		entry fs.DirEntry
	}
	pending := []directory{{root, fs.FileInfoToDirEntry(info)}}
	visited := 0
	mediaCount := 0
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		reportScan(ctx, "", "read_directory", next.path, visited, len(pending)+1, mediaCount)
		entries, err := readDir(next.path)
		if err != nil {
			return err
		}
		if err := visit(next.path, next.entry, entries); err != nil {
			if err == filepath.SkipDir {
				continue
			}
			return err
		}
		for _, entry := range entries {
			if isScanMedia(entry) {
				mediaCount++
			}
		}
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				pending = append(pending, directory{filepath.Join(next.path, entry.Name()), entry})
			}
		}
		visited++
		reportScan(ctx, "", "directory_complete", next.path, visited, len(pending), mediaCount)
		if backgroundLibraryScan(ctx) && visited%32 == 0 {
			// Give foreground thumbnail/original reads a chance between directories.
			timer := time.NewTimer(2 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}

func quickLibraryScanSnapshotMatches(ctx context.Context, snapshot *storage.LibraryScanSnapshot, sourcePath string, rootInfo fs.FileInfo) bool {
	if snapshot == nil || snapshot.Version != libraryScanSnapshotVersion || rootInfo == nil || len(snapshot.DirectoryModTimes) == 0 ||
		len(snapshot.DirectoryEntryHashes) != len(snapshot.DirectoryModTimes) {
		return false
	}
	if filepath.Clean(snapshot.RootPath) != filepath.Clean(sourcePath) {
		return false
	}
	checked := 0
	err := walkLibraryDirectories(ctx, sourcePath, func(path string) ([]fs.DirEntry, error) { return readScanDir(ctx, path) }, func(path string, d fs.DirEntry, entries []fs.DirEntry) error {
		checked++
		relPath := "."
		if path != sourcePath {
			rel, err := filepath.Rel(sourcePath, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(filepath.Clean(rel))
		}
		expectedModTime, ok := snapshot.DirectoryModTimes[relPath]
		if !ok {
			return errSnapshotMismatch
		}
		info, err := d.Info()
		if err != nil || info.ModTime().UnixNano() != expectedModTime || !libraryDirectoryEntriesMatch(snapshot, relPath, entries) {
			return errSnapshotMismatch
		}
		return nil
	})
	matched := err == nil && checked == len(snapshot.DirectoryModTimes)
	return matched
}

// Older v2 incremental scans omitted stable child directories from their
// parent's entry hash. Accept that exact legacy representation only when all
// child paths are already present in the snapshot. The caller still validates
// each child's timestamp and entries, so additions/deletions are not hidden.
// This avoids forcing users to rescan an otherwise unchanged existing library.
func libraryDirectoryEntriesMatch(snapshot *storage.LibraryScanSnapshot, relPath string, entries []fs.DirEntry) bool {
	expected := snapshot.DirectoryEntryHashes[relPath]
	if expected == hashDirectoryEntries(entries) {
		return true
	}
	legacyEntries := make([]fs.DirEntry, 0, len(entries))
	changed := make(map[string]bool, len(snapshot.ChangedDirectories))
	for _, dir := range snapshot.ChangedDirectories {
		changed[dir] = true
	}
	legacyWithChanged := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			child := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
			if _, known := snapshot.DirectoryModTimes[child]; !known {
				return false
			}
			if changed[child] {
				legacyWithChanged = append(legacyWithChanged, entry)
			}
			continue
		}
		legacyEntries = append(legacyEntries, entry)
		legacyWithChanged = append(legacyWithChanged, entry)
	}
	return expected == hashDirectoryEntries(legacyEntries) || expected == hashDirectoryEntries(legacyWithChanged)
}

func hashDirectoryEntries(entries []fs.DirEntry) string {
	hasher := fnv.New64a()
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			_, _ = fmt.Fprintf(hasher, "d:%s\x00", name)
			continue
		}
		if entry.Type().IsRegular() && (imgpkg.SupportedMimeTypes[imgpkg.DetectMimeType(name)] || media.IsSupportedVideoFilename(name)) {
			_, _ = fmt.Fprintf(hasher, "f:%s\x00", name)
		}
	}
	return fmt.Sprintf("%016x", hasher.Sum64())
}

func libraryScanSnapshotMatches(snapshot *storage.LibraryScanSnapshot, sourcePath string, current libraryDirectorySnapshot) bool {
	if snapshot == nil || snapshot.Version != libraryScanSnapshotVersion || current.rootInfo == nil {
		return false
	}
	return filepath.Clean(snapshot.RootPath) == filepath.Clean(sourcePath) &&
		snapshot.RootModUnixNano > 0 &&
		snapshot.RootModUnixNano == current.rootInfo.ModTime().UnixNano() &&
		snapshot.DirectoryCount > 0 &&
		snapshot.DirectoryCount == current.dirCount &&
		snapshot.DirectoryModHash != "" &&
		snapshot.DirectoryModHash == current.dirModHash &&
		snapshot.FileCount == current.mediaFileCount
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
	for sourceRelPath := range sourceIndex {
		if err := ctx.Err(); err != nil {
			return pruned, err
		}
		if sourceRelPath == "" || seenSourcePaths[filepath.ToSlash(filepath.Clean(sourceRelPath))] {
			continue
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

func sourceFileChangedAfterDeletion(info fs.FileInfo, deletedAtUnixNano int64) bool {
	if info == nil || deletedAtUnixNano <= 0 {
		return false
	}
	changedAt := sourceFileChangeTime(info)
	return !changedAt.IsZero() && changedAt.UnixNano() > deletedAtUnixNano
}

func sourceFileChangeTime(info fs.FileInfo) time.Time {
	if info == nil || info.Sys() == nil {
		return time.Time{}
	}
	value := reflect.ValueOf(info.Sys())
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return time.Time{}
	}
	for _, fieldName := range []string{"Ctim", "Ctimespec", "ChangeTime"} {
		if field := value.FieldByName(fieldName); field.IsValid() {
			if changedAt := reflectedTimespec(field); !changedAt.IsZero() {
				return changedAt
			}
		}
	}
	if field := value.FieldByName("CreationTime"); field.IsValid() {
		filetime := field
		for filetime.Kind() == reflect.Pointer || filetime.Kind() == reflect.Interface {
			if filetime.IsNil() {
				return time.Time{}
			}
			filetime = filetime.Elem()
		}
		if filetime.Kind() == reflect.Struct {
			high, okHigh := reflectedUint(filetime.FieldByName("HighDateTime"))
			low, okLow := reflectedUint(filetime.FieldByName("LowDateTime"))
			if okHigh && okLow {
				ticks := (high << 32) | low
				const windowsToUnixEpoch100ns = uint64(116444736000000000)
				if ticks >= windowsToUnixEpoch100ns {
					return time.Unix(0, int64(ticks-windowsToUnixEpoch100ns)*100)
				}
			}
		}
	}
	return time.Time{}
}

func reflectedTimespec(value reflect.Value) time.Time {
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return time.Time{}
	}
	sec, okSec := reflectedInt(value.FieldByName("Sec"))
	nsec, okNsec := reflectedInt(value.FieldByName("Nsec"))
	if !okSec || !okNsec || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, nsec)
}

func reflectedInt(value reflect.Value) (int64, bool) {
	if !value.IsValid() {
		return 0, false
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(value.Uint()), true
	default:
		return 0, false
	}
}

func reflectedUint(value reflect.Value) (uint64, bool) {
	if !value.IsValid() {
		return 0, false
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint(), true
	default:
		return 0, false
	}
}

func buildMissingSourceFingerprintIndex(sourceIndex map[string]storage.SourceMediaInfo, seenSourcePaths map[string]bool) map[string][]storage.SourceMediaInfo {
	index := make(map[string][]storage.SourceMediaInfo)
	for sourceRelPath, item := range sourceIndex {
		if sourceRelPath == "" || seenSourcePaths[filepath.Clean(sourceRelPath)] {
			continue
		}
		if item.Deleted {
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

func (s *PhotoService) importExistingPhotoFileContext(ctx context.Context, job importJob, uploadedBy int64) (*storage.Photo, bool, error) {
	info := job.info
	if info == nil {
		var err error
		info, err = scanStat(ctx, job.path)
		if err != nil {
			return nil, false, err
		}
	}
	if existing, err := s.repo.GetPhotoBySourceRelPath(job.sourceRelPath, uploadedBy); err != nil {
		return nil, false, err
	} else if existing != nil {
		wasDeleted := existing.DeletedAt != nil
		sourceChanged := existing.Size != info.Size() || existing.SourceModUnix != info.ModTime().UnixNano()
		if wasDeleted {
			if !sourceFileChangedAfterDeletion(info, existing.DeletedAt.UnixNano()) {
				return existing, false, nil
			}
			if err := s.repo.RestorePhoto(existing.ID, uploadedBy); err != nil {
				return nil, false, err
			}
			existing.DeletedAt = nil
			existing.DeletedBy = nil
			existing.DeletedGroupID = ""
		}
		if err := s.refreshExistingImportedPhotoMetadata(ctx, existing, job, uploadedBy, sourceChanged); err != nil {
			return nil, false, err
		}
		if sourceChanged {
			if err := s.repo.UpdatePhotoSourceMedia(existing.ID, uploadedBy, job.sourceRelPath, job.originalName, info.Size(), info.ModTime().UnixNano()); err != nil {
				return nil, false, fmt.Errorf("更新媒体源文件信息失败: %w", err)
			}
			existing.SourceRelPath = job.sourceRelPath
			existing.OriginalName = job.originalName
			existing.Size = info.Size()
			existing.SourceModUnix = info.ModTime().UnixNano()
			s.removeThumbnailFiles(existing)
			s.enqueueThumbnailGeneration(existing, true)
		}
		return existing, wasDeleted, nil
	}

	mimeType := imgpkg.DetectMimeType(job.originalName)
	if imgpkg.SupportedMimeTypes[mimeType] {
		meta, err := scanIO(ctx, job.path, func() (*imgpkg.Meta, error) {
			return imgpkg.ExtractMetaFromPathContext(ctx, job.path, job.originalName, info.ModTime())
		})
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

		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if err := s.repo.SavePhoto(photo); err != nil {
			return nil, false, fmt.Errorf("保存图片记录失败: %w", err)
		}

		return photo, true, nil
	}

	videoMeta, err := media.ProbeVideoContext(ctx, job.path)
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if err != nil {
		videoMeta = &media.VideoMeta{FormatName: strings.TrimPrefix(filepath.Ext(job.originalName), ".")}
	}
	takenAt := info.ModTime()
	if videoMeta != nil && !videoMeta.TakenAt.IsZero() {
		takenAt = videoMeta.TakenAt
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
		TakenAt:       takenAt,
		UploadedAt:    time.Now(),
		UploadedBy:    uploadedBy,
	}

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := s.repo.SavePhoto(photo); err != nil {
		return nil, false, fmt.Errorf("保存视频记录失败: %w", err)
	}
	return photo, true, nil
}

func (s *PhotoService) refreshExistingImportedPhotoMetadata(ctx context.Context, existing *storage.Photo, job importJob, uploadedBy int64, force bool) error {
	if existing == nil || (!force && !shouldRefreshCapturedMetadata(existing)) {
		return nil
	}
	info := job.info
	if info == nil {
		var err error
		info, err = scanStat(ctx, job.path)
		if err != nil {
			return err
		}
	}
	mimeType := imgpkg.DetectMimeType(job.originalName)
	if imgpkg.SupportedMimeTypes[mimeType] {
		meta, err := scanIO(ctx, job.path, func() (*imgpkg.Meta, error) {
			return imgpkg.ExtractMetaFromPathContext(ctx, job.path, job.originalName, info.ModTime())
		})
		if err != nil {
			return nil
		}
		exif := copyPhotoEXIF(meta.EXIF)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.repo.UpdatePhotoCapturedMetadata(existing.ID, uploadedBy, meta.TakenAt, exif, meta.Width, meta.Height, existing.DurationMS); err != nil {
			return err
		}
		existing.TakenAt = meta.TakenAt
		existing.EXIF = exif
		existing.Width = meta.Width
		existing.Height = meta.Height
		return nil
	}

	videoMeta, err := media.ProbeVideoContext(ctx, job.path)
	if err != nil || videoMeta == nil {
		return nil
	}
	takenAt := info.ModTime()
	if !videoMeta.TakenAt.IsZero() {
		takenAt = videoMeta.TakenAt
	}
	exif := videoMetaEXIF(videoMeta)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.repo.UpdatePhotoCapturedMetadata(existing.ID, uploadedBy, takenAt, exif, videoMeta.Width, videoMeta.Height, videoMeta.DurationMS); err != nil {
		return err
	}
	existing.TakenAt = takenAt
	existing.EXIF = exif
	existing.Width = videoMeta.Width
	existing.Height = videoMeta.Height
	existing.DurationMS = videoMeta.DurationMS
	return nil
}

func shouldRefreshCapturedMetadata(photo *storage.Photo) bool {
	if photo == nil {
		return false
	}
	if photo.TakenAt.IsZero() {
		return true
	}
	if photo.MediaKind == storage.MediaKindImage && photo.EXIF == nil {
		return true
	}
	if photo.MediaKind == storage.MediaKindVideo && (photo.EXIF == nil || photo.EXIF.TakenAt.IsZero()) {
		return true
	}
	if photo.SourceModUnix > 0 {
		sourceMod := time.Unix(0, photo.SourceModUnix)
		diff := photo.TakenAt.Sub(sourceMod)
		if diff < 0 {
			diff = -diff
		}
		if diff <= 2*time.Second {
			return true
		}
	}
	return false
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
		if album.SourceKind == folderAlbumSourceKind && strings.TrimSpace(album.Name) == rootFolderAlbumName && strings.TrimSpace(album.SourceRelPath) == "" {
			cache[rootFolderAlbumKey] = album.ID
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

func (s *PhotoService) attachImportedPhotoToFolderAlbum(photo *storage.Photo, uploadedBy int64, albumCache map[string]int64, deletedFolderPaths map[string]bool) error {
	if photo == nil || photo.SourceRelPath == "" {
		return nil
	}
	if isUnderDeletedFolderPath(photo.SourceRelPath, deletedFolderPaths) {
		return nil
	}
	albumPaths := folderAlbumPathsForSourceRelPath(photo.SourceRelPath)
	if len(albumPaths) == 0 {
		if albumID := albumCache[rootFolderAlbumKey]; albumID != 0 {
			return s.repo.AddPhotoToAlbum(albumID, photo.ID, uploadedBy)
		}
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

func (s *PhotoService) ensureFolderAlbums(uploadedBy int64, seenFolderAlbums map[string]bool, albumCache map[string]int64, deletedFolderPaths map[string]bool) error {
	if seenFolderAlbums[rootFolderAlbumKey] {
		if _, ok := albumCache[rootFolderAlbumKey]; !ok {
			album := &storage.Album{Name: rootFolderAlbumName, Description: folderAlbumDescription, SourceKind: folderAlbumSourceKind, CreatedBy: uploadedBy, CreatedAt: time.Now()}
			if err := s.repo.CreateAlbum(album); err != nil {
				return err
			}
			albumCache[rootFolderAlbumKey] = album.ID
		}
	}
	for albumPath := range seenFolderAlbums {
		if albumPath == rootFolderAlbumKey {
			continue
		}
		albumPath = filepath.ToSlash(filepath.Clean(albumPath))
		if albumPath == "." || albumPath == "" {
			continue
		}
		if isUnderDeletedFolderPath(albumPath, deletedFolderPaths) {
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
			// A scan only owns automatically generated folder albums. Personal
			// albums must survive even if they are not present on disk.
			continue
		}
		if strings.TrimSpace(album.SourceRelPath) == "" && album.Name == rootFolderAlbumName {
			if !seenFolderAlbums[rootFolderAlbumKey] {
				_ = s.repo.DeleteAlbum(album.ID, uploadedBy)
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

func isUnderDeletedFolderPath(sourceRelPath string, deletedFolderPaths map[string]bool) bool {
	if len(deletedFolderPaths) == 0 {
		return false
	}
	path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(sourceRelPath)))
	if path == "." || path == "" {
		return false
	}
	for current := path; current != "." && current != ""; {
		if deletedFolderPaths[current] {
			return true
		}
		parent := filepath.ToSlash(filepath.Dir(current))
		if parent == current {
			break
		}
		current = parent
	}
	return false
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
