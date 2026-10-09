package service

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/uuid"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

const (
	thumbnailTierFull = "full"
)

type thumbnailWorkItem struct {
	photo *storage.Photo
	tier  string
}

// PhotoService 图片业务逻辑
type PhotoService struct {
	lifecycleMu          sync.Mutex
	closing              bool
	workerStop           chan struct{}
	workerWG             sync.WaitGroup
	repo                 storage.Repository
	sourcePath           string
	dataPath             string
	thumbnailPath        string
	thumbnailLibraryID   string
	legacyThumbnailRoot  string
	legacyReadOnly       bool
	thumbnailBuildActive atomic.Bool
	thumbnailSize        int
	trashPath            string
	syncThumbnail        bool // 测试用：同步生成缩略图
	thumbPriorityHeavy   chan thumbnailWorkItem
	thumbJobsHeavy       chan thumbnailWorkItem
	thumbPriorityImage   chan thumbnailWorkItem
	thumbJobsImage       chan thumbnailWorkItem
	thumbPriorityVideo   chan thumbnailWorkItem
	thumbJobsVideo       chan thumbnailWorkItem
	thumbJobsMu          sync.Mutex
	scanMu               sync.Mutex
	thumbPending         map[string]struct{}
	thumbUrgent          map[string]struct{}
	thumbBuildMu         sync.Mutex
	thumbBuild           *thumbnailBuildTask
	playbackBuildMu      sync.Mutex
	playbackBuild        *playbackCacheBuildTask
	videoThumbRefresh    *videoThumbnailRefreshTask
	exifBackfillMu       sync.Mutex
	exifBackfill         *exifBackfillTask
}

func (s *PhotoService) LibraryUserID(fallback int64) int64 {
	if provider, ok := s.repo.(interface{ LibraryUserID(int64) int64 }); ok {
		return provider.LibraryUserID(fallback)
	}
	return fallback
}

// NewPhotoService 创建图片服务
func NewPhotoService(repo storage.Repository, sourcePath string, dataPath string, _ string) *PhotoService {
	return newPhotoService(repo, sourcePath, dataPath, true)
}

func NewPhotoServiceWithoutWarmup(repo storage.Repository, sourcePath string, dataPath string, _ string) *PhotoService {
	return newPhotoService(repo, sourcePath, dataPath, false)
}

func newPhotoService(repo storage.Repository, sourcePath string, dataPath string, startWorkers bool) *PhotoService {
	svc := &PhotoService{
		workerStop:    make(chan struct{}),
		repo:          repo,
		sourcePath:    sourcePath,
		dataPath:      dataPath,
		thumbnailPath: dataPath,
		thumbnailSize: imgpkg.DefaultThumbnailLongEdge,
		trashPath:     filepath.Join(sourcePath, ".echogallery", ".trash"),
		thumbPending:  make(map[string]struct{}),
		thumbUrgent:   make(map[string]struct{}),
	}
	if startWorkers {
		svc.startThumbnailWorkers()
	}
	return svc
}

// newPhotoServiceSync 创建同步模式图片服务（仅用于测试）
func newPhotoServiceSync(repo storage.Repository, sourcePath string, dataPath string, _ string) *PhotoService {
	return &PhotoService{
		repo:          repo,
		sourcePath:    sourcePath,
		dataPath:      dataPath,
		thumbnailPath: dataPath,
		thumbnailSize: imgpkg.DefaultThumbnailLongEdge,
		trashPath:     filepath.Join(sourcePath, ".echogallery", ".trash"),
		syncThumbnail: true,
		thumbPending:  make(map[string]struct{}),
		thumbUrgent:   make(map[string]struct{}),
	}
}

func imageThumbnailWarmupWorkerCount() int {
	if lowResourceModeEnabled() {
		return 1
	}
	n := runtime.GOMAXPROCS(0) / 5
	if n < 1 {
		return 1
	}
	if n > 3 {
		return 3
	}
	return n
}

func heavyImageThumbnailWarmupWorkerCount() int {
	return 1
}

func videoThumbnailWarmupWorkerCount() int {
	if lowResourceModeEnabled() {
		return 1
	}
	n := runtime.GOMAXPROCS(0) / 8
	if n < 1 {
		return 1
	}
	if n > 2 {
		return 2
	}
	return n
}

func (s *PhotoService) startThumbnailWorkers() {
	if s == nil || s.syncThumbnail || s.thumbJobsImage != nil || s.thumbJobsVideo != nil {
		return
	}
	s.thumbPriorityHeavy = make(chan thumbnailWorkItem, 12)
	s.thumbJobsHeavy = make(chan thumbnailWorkItem, 24)
	s.thumbPriorityImage = make(chan thumbnailWorkItem, 96)
	s.thumbJobsImage = make(chan thumbnailWorkItem, 192)
	s.thumbPriorityVideo = make(chan thumbnailWorkItem, 48)
	s.thumbJobsVideo = make(chan thumbnailWorkItem, 128)
	for i := 0; i < heavyImageThumbnailWarmupWorkerCount(); i++ {
		s.background(func() {
			for {
				job, ok := s.nextThumbnailJob(s.thumbPriorityHeavy, s.thumbJobsHeavy)
				if !ok {
					return
				}
				s.processThumbnailJob(job)
			}
		})
	}
	for i := 0; i < imageThumbnailWarmupWorkerCount(); i++ {
		s.background(func() {
			for {
				job, ok := s.nextThumbnailJob(s.thumbPriorityImage, s.thumbJobsImage)
				if !ok {
					return
				}
				s.processThumbnailJob(job)
			}
		})
	}
	for i := 0; i < videoThumbnailWarmupWorkerCount(); i++ {
		s.background(func() {
			for {
				job, ok := s.nextThumbnailJob(s.thumbPriorityVideo, s.thumbJobsVideo)
				if !ok {
					return
				}
				s.processThumbnailJob(job)
			}
		})
	}
}

func (s *PhotoService) processThumbnailJob(job thumbnailWorkItem) {
	_ = s.generateThumbnailForPhotoTier(job.photo, job.tier)
	if job.photo == nil {
		return
	}
	key := s.thumbnailQueueKey(job.photo, job.tier)
	s.thumbJobsMu.Lock()
	delete(s.thumbPending, key)
	delete(s.thumbUrgent, key)
	s.thumbJobsMu.Unlock()
}

func (s *PhotoService) nextThumbnailJob(priority <-chan thumbnailWorkItem, normal <-chan thumbnailWorkItem) (thumbnailWorkItem, bool) {
	if s == nil {
		return thumbnailWorkItem{}, false
	}
	select {
	case <-s.workerStop:
		return thumbnailWorkItem{}, false
	case job, ok := <-priority:
		return job, ok
	default:
	}
	select {
	case <-s.workerStop:
		return thumbnailWorkItem{}, false
	case job, ok := <-priority:
		return job, ok
	case job, ok := <-normal:
		return job, ok
	}
}

func (s *PhotoService) thumbnailQueueKey(photo *storage.Photo, tier string) string {
	if photo == nil {
		return ""
	}
	return photo.UUID + ":" + tier
}

func (s *PhotoService) enqueueThumbnailTier(photo *storage.Photo, tier string, priority bool) {
	if s == nil || photo == nil || photo.UUID == "" {
		return
	}
	if s.syncThumbnail {
		_ = s.generateThumbnailForPhotoTier(photo, tier)
		return
	}
	if s.thumbnailExistsForTier(photo, tier) {
		return
	}
	key := s.thumbnailQueueKey(photo, tier)
	s.thumbJobsMu.Lock()
	if priority {
		if _, exists := s.thumbUrgent[key]; exists {
			s.thumbJobsMu.Unlock()
			return
		}
		s.thumbUrgent[key] = struct{}{}
		s.thumbPending[key] = struct{}{}
		s.thumbJobsMu.Unlock()
		s.enqueueThumbnailJob(thumbnailWorkItem{photo: photo, tier: tier}, true)
		return
	}
	if _, exists := s.thumbPending[key]; exists {
		s.thumbJobsMu.Unlock()
		return
	}
	s.thumbPending[key] = struct{}{}
	s.thumbJobsMu.Unlock()
	s.enqueueThumbnailJob(thumbnailWorkItem{photo: photo, tier: tier}, false)
}

func (s *PhotoService) enqueueThumbnailJob(job thumbnailWorkItem, priority bool) {
	if job.photo == nil {
		return
	}
	queue := s.thumbJobsImage
	if priority {
		queue = s.thumbPriorityImage
	}
	if job.photo.MediaKind == storage.MediaKindVideo {
		queue = s.thumbJobsVideo
		if priority {
			queue = s.thumbPriorityVideo
		}
	} else if isMemoryHeavyThumbnail(job.photo) {
		queue = s.thumbJobsHeavy
		if priority {
			queue = s.thumbPriorityHeavy
		}
	}
	select {
	case <-s.workerStop:
	case queue <- job:
	}
}

func (s *PhotoService) enqueueThumbnailGeneration(photo *storage.Photo, priority bool) {
	if s == nil || photo == nil || photo.UUID == "" {
		return
	}
	if s.syncThumbnail {
		_ = s.generateThumbnailForPhoto(photo)
		return
	}
	if s.thumbJobsImage == nil || s.thumbJobsVideo == nil {
		return
	}
	s.enqueueThumbnailTier(photo, thumbnailTierFull, priority)
}

func isVideoMediaKind(photo *storage.Photo) bool {
	return photo != nil && photo.MediaKind == storage.MediaKindVideo
}

func isMemoryHeavyThumbnail(photo *storage.Photo) bool {
	if photo == nil || photo.MediaKind == storage.MediaKindVideo {
		return false
	}
	width := int64(photo.Width)
	height := int64(photo.Height)
	pixels := width * height
	if pixels >= 24_000_000 {
		return true
	}
	return photo.Size >= 18*1024*1024
}

func (s *PhotoService) warmImportedThumbnails(photos []*storage.Photo) {
	if s == nil || s.syncThumbnail || len(photos) == 0 {
		return
	}
	sort.SliceStable(photos, func(i, j int) bool {
		ti := photos[i].TakenAt
		tj := photos[j].TakenAt
		if ti.Equal(tj) {
			return photos[i].UUID > photos[j].UUID
		}
		return ti.After(tj)
	})
	const firstScreenWarmCount = 180
	s.background(func() {
		for index, photo := range photos {
			s.enqueueThumbnailGeneration(photo, index < firstScreenWarmCount)
		}
	})
}

func (s *PhotoService) WarmThumbnailsByUUIDs(uuids []string, userID int64) (int, error) {
	if s == nil || len(uuids) == 0 {
		return 0, nil
	}
	seen := make(map[string]struct{}, len(uuids))
	warmed := 0
	for _, raw := range uuids {
		uuid := strings.TrimSpace(raw)
		if uuid == "" {
			continue
		}
		if _, exists := seen[uuid]; exists {
			continue
		}
		seen[uuid] = struct{}{}
		photo, err := s.repo.GetPhotoByUUIDAny(uuid, userID)
		if err != nil || photo == nil {
			continue
		}
		s.enqueueThumbnailTier(photo, thumbnailTierFull, true)
		warmed++
		if warmed >= 160 {
			break
		}
	}
	return warmed, nil
}

func (s *PhotoService) RenameMedia(id int64, userID int64, name string) (*storage.Photo, error) {
	photo, err := s.GetPhoto(id, userID)
	if err != nil {
		return nil, err
	}
	if photo == nil {
		return nil, fmt.Errorf("媒体不存在")
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") {
		return nil, fmt.Errorf("文件名无效")
	}
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`/\\:*?"<>|`, r) {
			return nil, fmt.Errorf("文件名包含不支持的字符")
		}
	}
	upper := strings.ToUpper(strings.TrimRight(name, " ."))
	base := upper
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return nil, fmt.Errorf("文件名是系统保留名称")
	}
	if filepath.Ext(name) != "" {
		return nil, fmt.Errorf("请只输入文件名，不要输入扩展名")
	}
	oldPath := s.MediaPath(photo)
	if _, err := os.Stat(oldPath); err != nil {
		return nil, fmt.Errorf("读取原媒体失败: %w", err)
	}
	usesSourcePath := photo.SourceRelPath != ""
	if usesSourcePath {
		if _, err := os.Stat(filepath.Join(s.sourcePath, filepath.FromSlash(photo.SourceRelPath))); err != nil {
			usesSourcePath = false
		}
	}
	ext := filepath.Ext(photo.OriginalName)
	newName := name + ext
	newPath := filepath.Join(filepath.Dir(oldPath), newName)
	if filepath.Clean(newPath) == filepath.Clean(oldPath) {
		return photo, nil
	}
	if _, err := os.Lstat(newPath); err == nil {
		return nil, fmt.Errorf("目标文件已存在")
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("检查目标文件失败: %w", err)
	}
	// Rename the directory entry only; never decode or rewrite media bytes and embedded metadata.
	if err := os.Rename(oldPath, newPath); err != nil {
		return nil, fmt.Errorf("重命名媒体文件失败: %w", err)
	}
	newInfo, err := os.Stat(newPath)
	if err != nil {
		_ = os.Rename(newPath, oldPath)
		return nil, fmt.Errorf("读取重命名后的媒体失败: %w", err)
	}
	sourceRelPath := photo.SourceRelPath
	storageRelPath := photo.StorageRelPath
	if usesSourcePath {
		sourceRelPath, err = filepath.Rel(s.sourcePath, newPath)
		sourceRelPath = filepath.ToSlash(sourceRelPath)
	} else if storageRelPath != "" {
		storageRelPath, err = filepath.Rel(s.dataPath, newPath)
		storageRelPath = filepath.ToSlash(storageRelPath)
		sourceRelPath = ""
	}
	if err != nil {
		_ = os.Rename(newPath, oldPath)
		return nil, fmt.Errorf("媒体路径无法转换为相对路径: %w", err)
	}
	if err := s.repo.UpdatePhotoMediaPath(photo.ID, userID, sourceRelPath, storageRelPath, newName, newInfo.Size(), newInfo.ModTime().UnixNano()); err != nil {
		if rollbackErr := os.Rename(newPath, oldPath); rollbackErr != nil {
			return nil, fmt.Errorf("更新媒体记录失败: %v；恢复原文件名也失败: %w", err, rollbackErr)
		}
		return nil, fmt.Errorf("更新媒体记录失败: %w", err)
	}
	photo.SourceRelPath = sourceRelPath
	photo.StorageRelPath = storageRelPath
	photo.OriginalName = newName
	photo.Size = newInfo.Size()
	photo.SourceModUnix = newInfo.ModTime().UnixNano()
	if err := s.syncPortableDatabase(); err != nil {
		return photo, fmt.Errorf("媒体已重命名，但同步资源库数据库失败: %w", err)
	}
	return photo, nil
}

func (s *PhotoService) thumbnailExistsForTier(photo *storage.Photo, tier string) bool {
	if photo == nil {
		return false
	}
	targetPath := s.ThumbnailPath(photo)
	if resolveManagedFile(targetPath) {
		s.cleanupRedundantThumbnailFiles(photo)
		return true
	}
	for _, legacyPath := range s.legacyThumbnailCandidates(photo) {
		if legacyPath == "" || legacyPath == targetPath || !resolveManagedFile(legacyPath) {
			continue
		}
		return true
	}
	return false
}

func (s *PhotoService) thumbnailTargetExists(photo *storage.Photo) bool {
	return photo != nil && resolveManagedFile(s.ThumbnailPath(photo))
}

func resolveManagedFile(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (s *PhotoService) generateThumbnailForPhoto(photo *storage.Photo) error {
	return s.generateThumbnailForPhotoTier(photo, thumbnailTierFull)
}

func (s *PhotoService) generateThumbnailForPhotoTier(photo *storage.Photo, tier string) error {
	if photo == nil {
		return nil
	}
	if tier != thumbnailTierFull {
		tier = thumbnailTierFull
	}
	if s.thumbnailExistsForTier(photo, tier) && !s.thumbnailBuildIsActive() {
		return nil
	}
	if s.legacyReadOnly && !s.thumbnailBuildIsActive() {
		return fmt.Errorf("旧版缩略图仅可读取，请从设置页启动缩略图构建")
	}
	return s.writeThumbnailForPhoto(photo)
}

func (s *PhotoService) writeThumbnailForPhoto(photo *storage.Photo) error {
	maxEdge := s.thumbnailLongEdge()
	destPath := s.ThumbnailPath(photo)
	if photo.MediaKind == storage.MediaKindVideo {
		if err := media.GeneratePoster(s.resolveFinderPath(photo), destPath, maxEdge); err != nil {
			return err
		}
		s.cleanupRedundantThumbnailFiles(photo)
		return nil
	}
	srcPath := s.resolveFinderPath(photo)
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := imgpkg.GenerateThumbnail(file, photo.MimeType, destPath, maxEdge); err != nil {
		return err
	}
	s.cleanupRedundantThumbnailFiles(photo)
	return nil
}

func (s *PhotoService) thumbnailLongEdge() int {
	if s.thumbnailSize <= 0 {
		return imgpkg.DefaultThumbnailLongEdge
	}
	return s.thumbnailSize
}

func (s *PhotoService) SetThumbnailRoot(path string) {
	if path = filepath.Clean(path); path != "" && path != "." {
		s.thumbnailPath = path
	}
}

func (s *PhotoService) SetThumbnailLibraryID(id string) {
	s.thumbnailLibraryID = normalizeThumbnailLibraryID(id)
}

// SetLegacyThumbnailRoot enables read-only fallback to the old server-local layout.
func (s *PhotoService) SetLegacyThumbnailRoot(root string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	s.legacyThumbnailRoot = filepath.Clean(root)
	s.legacyReadOnly = true
}

func (s *PhotoService) legacyManagedThumbnailRoot() string {
	if s == nil || s.legacyThumbnailRoot == "" {
		return ""
	}
	if s.thumbnailLibraryID == "" {
		return s.legacyThumbnailRoot
	}
	return filepath.Join(s.legacyThumbnailRoot, s.thumbnailLibraryID)
}

func (s *PhotoService) thumbnailBuildIsActive() bool {
	return s != nil && s.thumbnailBuildActive.Load()
}

func (s *PhotoService) LegacyThumbnailReadOnly() bool {
	return s != nil && s.legacyReadOnly
}

func (s *PhotoService) SetThumbnailSize(size int) {
	if size >= 96 && size <= 1024 {
		s.thumbnailSize = size
	}
}

// UploadInput 上传图片的输入参数
type UploadInput struct {
	Reader       io.ReadSeeker
	OriginalName string
	Size         int64
	UploadedBy   int64
	FileModTime  time.Time // 文件修改时间，作为 EXIF 缺失时的后备
	RelPath      string    // 上传文件在资源库中的相对路径；为空时自动生成
}

// UploadResult 上传结果
type UploadResult struct {
	Photo *storage.Photo
}

// Upload 处理图片上传：提取元数据、存储文件、写数据库
func (s *PhotoService) Upload(input UploadInput) (*UploadResult, error) {
	// 1. 提取图片元数据（EXIF、尺寸、类型）
	fallbackTime := input.FileModTime
	if fallbackTime.IsZero() {
		fallbackTime = time.Now()
	}

	meta, err := imgpkg.ExtractMeta(input.Reader, input.OriginalName, fallbackTime)
	if err != nil {
		return nil, fmt.Errorf("解析图片失败: %w", err)
	}

	// 2. 生成 UUID 文件名
	photoUUID := uuid.New().String()
	sourceRelPath := input.RelPath
	if strings.TrimSpace(sourceRelPath) == "" {
		sourceRelPath = UploadedMediaRelPath(photoUUID, input.OriginalName, time.Now())
	}

	// 3. 重置读取位置，写入磁盘
	if _, err := input.Reader.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek 失败: %w", err)
	}

	destPath := filepath.Join(s.sourcePath, sourceRelPath)
	if err := writeFile(destPath, input.Reader); err != nil {
		return nil, fmt.Errorf("保存图片文件失败: %w", err)
	}
	if !fallbackTime.IsZero() {
		_ = os.Chtimes(destPath, fallbackTime, fallbackTime)
	}

	// 4. 写数据库
	photo := &storage.Photo{
		UUID:          photoUUID,
		OriginalName:  input.OriginalName,
		MediaKind:     storage.MediaKindImage,
		MimeType:      meta.MimeType,
		Size:          input.Size,
		Width:         meta.Width,
		Height:        meta.Height,
		DurationMS:    0,
		EXIF:          copyPhotoEXIF(meta.EXIF),
		SourceRelPath: sourceRelPath,
		SourceModUnix: fallbackTime.UnixNano(),
		TakenAt:       meta.TakenAt,
		UploadedAt:    time.Now(),
		UploadedBy:    input.UploadedBy,
	}

	if err := s.repo.SavePhoto(photo); err != nil {
		// 数据库写入失败，清理已写入的文件
		os.Remove(destPath)
		return nil, fmt.Errorf("保存图片记录失败: %w", err)
	}

	// 5. 生成缩略图（生产环境异步，测试环境同步）
	if s.syncThumbnail {
		s.generateThumbnail(photo, destPath, meta.MimeType)
	} else {
		s.background(func() { s.enqueueThumbnailGeneration(photo, true) })
	}

	return &UploadResult{Photo: photo}, nil
}

func copyPhotoEXIF(src *imgpkg.EXIFData) *storage.PhotoEXIF {
	if src == nil {
		return nil
	}
	return &storage.PhotoEXIF{
		Make:         src.Make,
		Model:        src.Model,
		Orientation:  src.Orientation,
		TakenAt:      src.TakenAt,
		Width:        src.Width,
		Height:       src.Height,
		FNumber:      src.FNumber,
		ExposureTime: src.ExposureTime,
		ISOSpeed:     src.ISOSpeed,
		FocalLength:  src.FocalLength,
		Latitude:     src.Latitude,
		Longitude:    src.Longitude,
		HasGPS:       src.HasGPS,
		Location:     src.Location,
	}
}

// generateThumbnail 生成缩略图（在后台 goroutine 中调用）
func (s *PhotoService) generateThumbnail(photo *storage.Photo, srcPath string, mimeType string) {
	f, err := os.Open(srcPath)
	if err != nil {
		return
	}
	defer f.Close()
	imgpkg.GenerateThumbnail(f, mimeType, s.ThumbnailPath(photo), s.thumbnailLongEdge()) //nolint:errcheck
	s.cleanupRedundantThumbnailFiles(photo)
}

// PhotoPath 返回图片原图的磁盘路径
func (s *PhotoService) PhotoPath(photo *storage.Photo) string {
	return s.MediaPath(photo)
}

// ThumbnailPath 返回图片缩略图的磁盘路径
func (s *PhotoService) ThumbnailPath(photo *storage.Photo) string {
	if photo != nil && photo.MediaKind == storage.MediaKindVideo {
		return s.PosterPath(photo)
	}
	return imgpkg.ThumbnailShardPath(s.managedThumbnailRoot(), photo.UUID)
}

func (s *PhotoService) managedThumbnailRoot() string {
	if s == nil || s.thumbnailLibraryID == "" {
		return s.thumbnailPath
	}
	return filepath.Join(s.thumbnailPath, s.thumbnailLibraryID)
}

func normalizeThumbnailLibraryID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func (s *PhotoService) legacyShardedThumbnailPath(photo *storage.Photo) string {
	if photo == nil {
		return ""
	}
	return imgpkg.ThumbnailShardPath(s.thumbnailPath, photo.UUID)
}

func (s *PhotoService) legacyFlatThumbnailPath(photo *storage.Photo) string {
	if photo == nil {
		return ""
	}
	return imgpkg.ThumbnailFlatPath(s.thumbnailPath, photo.UUID)
}

func (s *PhotoService) legacyThumbnailPreviewPath(photo *storage.Photo) string {
	if photo == nil {
		return ""
	}
	return filepath.Join(s.thumbnailPath, photo.UUID+".preview.webp")
}

func (s *PhotoService) legacyThumbnailBuildPreviewPath(photo *storage.Photo) string {
	if photo == nil {
		return ""
	}
	return filepath.Join(s.thumbnailPath, photo.UUID+".build-preview.webp")
}

func (s *PhotoService) LegacyThumbnailPath(photo *storage.Photo) string {
	if photo == nil {
		return ""
	}
	return filepath.Join(s.thumbnailPath, photo.UUID+".jpg")
}

func (s *PhotoService) ThumbnailCandidates(photo *storage.Photo) []string {
	if photo == nil {
		return nil
	}
	return append([]string{s.ThumbnailPath(photo)}, s.legacyThumbnailCandidates(photo)...)
}

func (s *PhotoService) legacyThumbnailCandidates(photo *storage.Photo) []string {
	if photo == nil {
		return nil
	}
	root := s.legacyManagedThumbnailRoot()
	if root == "" {
		return []string{s.legacyShardedThumbnailPath(photo), s.legacyFlatThumbnailPath(photo)}
	}
	return []string{
		imgpkg.ThumbnailShardPath(root, photo.UUID),
		imgpkg.ThumbnailFlatPath(root, photo.UUID),
		filepath.Join(root, photo.UUID+".preview.webp"),
		filepath.Join(root, photo.UUID+".build-preview.webp"),
		filepath.Join(root, photo.UUID+".jpg"),
		imgpkg.ThumbnailShardPath(s.legacyThumbnailRoot, photo.UUID),
		imgpkg.ThumbnailFlatPath(s.legacyThumbnailRoot, photo.UUID),
	}
}

func (s *PhotoService) thumbnailCleanupPaths(photo *storage.Photo) []string {
	if photo == nil {
		return nil
	}
	return []string{
		s.ThumbnailPath(photo),
		s.legacyShardedThumbnailPath(photo),
		s.legacyFlatThumbnailPath(photo),
		s.legacyThumbnailPreviewPath(photo),
		s.legacyThumbnailBuildPreviewPath(photo),
		s.LegacyThumbnailPath(photo),
	}
}

func (s *PhotoService) thumbnailSatisfyingPaths(photo *storage.Photo, tier string) []string {
	if photo == nil {
		return nil
	}
	return []string{s.ThumbnailPath(photo), s.legacyShardedThumbnailPath(photo), s.legacyFlatThumbnailPath(photo)}
}

func (s *PhotoService) cleanupRedundantThumbnailFiles(photo *storage.Photo) {
	_ = s.cleanupThumbnailArtifacts(photo, resolveManagedFile(s.ThumbnailPath(photo)), false)
}

func (s *PhotoService) cleanupRedundantThumbnailFilesCount(photo *storage.Photo, fullExists bool) int {
	return s.cleanupThumbnailArtifacts(photo, fullExists, false)
}

func (s *PhotoService) cleanupThumbnailArtifacts(photo *storage.Photo, fullExists bool, forceLegacyCleanup bool) int {
	if photo == nil {
		return 0
	}
	fullPath := s.ThumbnailPath(photo)
	legacyShardedPath := s.legacyShardedThumbnailPath(photo)
	legacyFlatPath := s.legacyFlatThumbnailPath(photo)
	previewPath := s.legacyThumbnailPreviewPath(photo)
	buildPreviewPath := s.legacyThumbnailBuildPreviewPath(photo)
	legacyFullPath := s.LegacyThumbnailPath(photo)
	cleaned := 0
	if fullExists {
		if legacyShardedPath != "" && legacyShardedPath != fullPath {
			if removeThumbnailFileIfExists(legacyShardedPath) {
				cleaned++
			}
		}
		if legacyFlatPath != "" && legacyFlatPath != fullPath {
			if removeThumbnailFileIfExists(legacyFlatPath) {
				cleaned++
			}
		}
	}
	if fullExists || forceLegacyCleanup {
		if removeThumbnailFileIfExists(previewPath) {
			cleaned++
		}
		if removeThumbnailFileIfExists(buildPreviewPath) {
			cleaned++
		}
		if (fullExists || forceLegacyCleanup) && removeThumbnailFileIfExists(legacyFullPath) {
			cleaned++
		}
	}
	return cleaned
}

func copyManagedThumbnailFile(srcPath string, destPath string) error {
	_, err := copyManagedThumbnailFileWithChecksum(srcPath, destPath)
	return err
}

func copyManagedThumbnailFileWithChecksum(srcPath string, destPath string) ([32]byte, error) {
	var empty [32]byte
	if strings.TrimSpace(srcPath) == "" || strings.TrimSpace(destPath) == "" || srcPath == destPath {
		return empty, nil
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return empty, err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return empty, err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".echogallery-thumb-copy-*")
	if err != nil {
		return empty, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hash), src); err != nil {
		_ = tmp.Close()
		return empty, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return empty, err
	}
	if err := tmp.Close(); err != nil {
		return empty, err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return empty, err
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum, nil
}

// ManagedMediaRelPath 返回应用内部托管媒体文件的相对路径。
func ManagedMediaRelPath(photoUUID, originalName string) string {
	return filepath.Join(".library", photoUUID+filepath.Ext(originalName))
}

// UploadedMediaRelPath 返回用户上传媒体在资源库中的相对路径。
func UploadedMediaRelPath(photoUUID, originalName string, now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	return filepath.Join("EchoGallery Uploads", now.Format("2006-01-02"), photoUUID+filepath.Ext(originalName))
}

// writeFile 将 reader 内容写入目标路径，目标目录若不存在则自动创建
func writeFile(destPath string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
