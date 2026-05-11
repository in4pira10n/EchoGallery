package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

// PhotoService 图片业务逻辑
type PhotoService struct {
	repo          storage.Repository
	sourcePath    string
	dataPath      string
	thumbnailPath string
	thumbnailSize int
	trashPath     string
	syncThumbnail bool // 测试用：同步生成缩略图
	thumbPriority chan *storage.Photo
	thumbJobs     chan *storage.Photo
	thumbJobsMu   sync.Mutex
	thumbPending  map[string]struct{}
	thumbUrgent   map[string]struct{}
	thumbBuildMu  sync.Mutex
	thumbBuild    *thumbnailBuildTask
}

// NewPhotoService 创建图片服务
func NewPhotoService(repo storage.Repository, sourcePath string, dataPath string, trashPath string) *PhotoService {
	svc := &PhotoService{
		repo:          repo,
		sourcePath:    sourcePath,
		dataPath:      dataPath,
		thumbnailPath: dataPath,
		thumbnailSize: imgpkg.DefaultThumbnailLongEdge,
		trashPath:     trashPath,
		thumbPending:  make(map[string]struct{}),
		thumbUrgent:   make(map[string]struct{}),
	}
	svc.startThumbnailWorkers()
	return svc
}

// newPhotoServiceSync 创建同步模式图片服务（仅用于测试）
func newPhotoServiceSync(repo storage.Repository, sourcePath string, dataPath string, trashPath string) *PhotoService {
	return &PhotoService{
		repo:          repo,
		sourcePath:    sourcePath,
		dataPath:      dataPath,
		thumbnailPath: dataPath,
		thumbnailSize: imgpkg.DefaultThumbnailLongEdge,
		trashPath:     trashPath,
		syncThumbnail: true,
		thumbPending:  make(map[string]struct{}),
		thumbUrgent:   make(map[string]struct{}),
	}
}

func thumbnailWarmupWorkerCount() int {
	n := runtime.GOMAXPROCS(0) / 3
	if n < 1 {
		return 1
	}
	if n > 3 {
		return 3
	}
	return n
}

func (s *PhotoService) startThumbnailWorkers() {
	if s == nil || s.syncThumbnail || s.thumbJobs != nil {
		return
	}
	s.thumbPriority = make(chan *storage.Photo, 192)
	s.thumbJobs = make(chan *storage.Photo, 512)
	for i := 0; i < thumbnailWarmupWorkerCount(); i++ {
		go func() {
			for {
				photo, ok := s.nextThumbnailJob()
				if !ok {
					return
				}
				s.generateThumbnailForPhoto(photo) //nolint:errcheck
				if photo == nil {
					continue
				}
				s.thumbJobsMu.Lock()
				delete(s.thumbPending, photo.UUID)
				delete(s.thumbUrgent, photo.UUID)
				s.thumbJobsMu.Unlock()
			}
		}()
	}
}

func (s *PhotoService) nextThumbnailJob() (*storage.Photo, bool) {
	if s == nil {
		return nil, false
	}
	select {
	case photo, ok := <-s.thumbPriority:
		return photo, ok
	default:
	}
	select {
	case photo, ok := <-s.thumbPriority:
		return photo, ok
	case photo, ok := <-s.thumbJobs:
		return photo, ok
	}
}

func (s *PhotoService) enqueueThumbnailGeneration(photo *storage.Photo, priority bool) {
	if s == nil || photo == nil || photo.UUID == "" {
		return
	}
	if s.syncThumbnail {
		s.generateThumbnailForPhoto(photo) //nolint:errcheck
		return
	}
	if s.thumbJobs == nil {
		return
	}
	if s.thumbnailAlreadyExists(photo) {
		return
	}
	s.thumbJobsMu.Lock()
	if priority {
		if _, exists := s.thumbUrgent[photo.UUID]; exists {
			s.thumbJobsMu.Unlock()
			return
		}
		s.thumbUrgent[photo.UUID] = struct{}{}
		s.thumbPending[photo.UUID] = struct{}{}
		s.thumbJobsMu.Unlock()
		s.thumbPriority <- photo
		return
	}
	if _, exists := s.thumbPending[photo.UUID]; exists {
		s.thumbJobsMu.Unlock()
		return
	}
	s.thumbPending[photo.UUID] = struct{}{}
	s.thumbJobsMu.Unlock()
	s.thumbJobs <- photo
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
	go func() {
		for index, photo := range photos {
			s.enqueueThumbnailGeneration(photo, index < firstScreenWarmCount)
		}
	}()
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
		s.enqueueThumbnailGeneration(photo, true)
		warmed++
		if warmed >= 160 {
			break
		}
	}
	return warmed, nil
}

func (s *PhotoService) thumbnailAlreadyExists(photo *storage.Photo) bool {
	for _, candidate := range s.ThumbnailCandidates(photo) {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}

func (s *PhotoService) generateThumbnailForPhoto(photo *storage.Photo) error {
	if photo == nil {
		return nil
	}
	if s.thumbnailAlreadyExists(photo) {
		return nil
	}
	if photo.MediaKind == storage.MediaKindVideo {
		return media.GeneratePoster(s.resolveFinderPath(photo), s.PosterPath(photo), s.thumbnailSize)
	}
	srcPath := s.resolveFinderPath(photo)
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return imgpkg.GenerateThumbnail(file, photo.MimeType, s.ThumbnailPath(photo), s.thumbnailSize)
}

func (s *PhotoService) SetThumbnailRoot(path string) {
	if path = filepath.Clean(path); path != "" && path != "." {
		s.thumbnailPath = path
	}
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
		go s.enqueueThumbnailGeneration(photo, true)
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
	}
}

// generateThumbnail 生成缩略图（在后台 goroutine 中调用）
func (s *PhotoService) generateThumbnail(photo *storage.Photo, srcPath string, mimeType string) {
	thumbPath := s.ThumbnailPath(photo)

	f, err := os.Open(srcPath)
	if err != nil {
		return
	}
	defer f.Close()

	imgpkg.GenerateThumbnail(f, mimeType, thumbPath, s.thumbnailSize) //nolint:errcheck
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
	return filepath.Join(s.thumbnailPath, photo.UUID+".webp")
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
	if photo.MediaKind == storage.MediaKindVideo {
		candidates := []string{s.PosterPath(photo)}
		legacy := filepath.Join(s.thumbnailPath, photo.UUID+".jpg")
		if !strings.EqualFold(legacy, candidates[0]) {
			candidates = append(candidates, legacy)
		}
		return candidates
	}
	return []string{s.ThumbnailPath(photo), s.LegacyThumbnailPath(photo)}
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
