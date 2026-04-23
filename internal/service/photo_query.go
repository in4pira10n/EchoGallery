package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"echogallery/internal/storage"
)

var (
	execCommand = exec.Command
	currentOS   = runtime.GOOS
)

// GetTimeline 获取时间线图片（游标分页）
func (s *PhotoService) GetTimeline(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	return s.repo.ListPhotos(params)
}

// GetTrash 获取回收站图片（游标分页）
func (s *PhotoService) GetTrash(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	return s.repo.ListTrashedPhotos(params)
}

// GetFavorites 获取个人收藏（游标分页）
func (s *PhotoService) GetFavorites(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	return s.repo.ListFavoritePhotos(params)
}

// GetAlbumMedia 获取相册内媒体（游标分页）。
func (s *PhotoService) GetAlbumMedia(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error) {
	return s.repo.ListAlbumPhotos(params)
}

// GetAlbum 获取单个相册。
func (s *PhotoService) GetAlbum(id int64, userID int64) (*storage.Album, error) {
	return s.repo.GetAlbumByID(id, userID)
}

// ListAlbums 获取用户所有相册。
func (s *PhotoService) ListAlbums(userID int64) ([]*storage.Album, error) {
	return s.repo.ListAlbums(userID)
}

// ListAlbumsForPhoto 获取包含指定媒体的相册。
func (s *PhotoService) ListAlbumsForPhoto(photoID int64, userID int64) ([]*storage.Album, error) {
	return s.repo.ListAlbumsForPhoto(photoID, userID)
}

// ListShares 获取用户所有分享链接。
func (s *PhotoService) ListShares(userID int64) ([]*storage.ShareLink, error) {
	return s.repo.ListShareLinks(userID)
}

// GetShareByToken 通过 token 获取分享链接。
func (s *PhotoService) GetShareByToken(token string) (*storage.ShareLink, error) {
	return s.repo.GetShareLinkByToken(token)
}

// CreateShare 创建分享链接。
func (s *PhotoService) CreateShare(input CreateShareInput) (*storage.ShareLink, error) {
	if input.Type != storage.ShareTypePhoto && input.Type != storage.ShareTypeAlbum {
		return nil, fmt.Errorf("无效的分享类型: %s", input.Type)
	}

	token, err := generateToken(16)
	if err != nil {
		return nil, fmt.Errorf("生成 token 失败: %w", err)
	}

	link := &storage.ShareLink{
		Token:     token,
		Type:      input.Type,
		TargetID:  input.TargetID,
		CreatedBy: input.UserID,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: time.Now(),
	}
	if err := s.repo.CreateShareLink(link); err != nil {
		return nil, err
	}
	return link, nil
}

// DeleteShare 删除分享链接。
func (s *PhotoService) DeleteShare(id int64, userID int64) error {
	return s.repo.DeleteShareLink(id, userID)
}

// CreateAlbum 创建相册。
func (s *PhotoService) CreateAlbum(name, description string, userID int64) (*storage.Album, error) {
	if name == "" {
		return nil, fmt.Errorf("相册名称不能为空")
	}
	album := &storage.Album{
		Name:        name,
		Description: description,
		CreatedBy:   userID,
		CreatedAt:   time.Now(),
	}
	if err := s.repo.CreateAlbum(album); err != nil {
		return nil, err
	}
	return album, nil
}

// UpdateAlbum 更新相册。
func (s *PhotoService) UpdateAlbum(id int64, name, description string, coverPhotoID *int64, userID int64) (*storage.Album, error) {
	album, err := s.repo.GetAlbumByID(id, userID)
	if err != nil {
		return nil, err
	}
	if album == nil {
		return nil, fmt.Errorf("相册不存在")
	}
	if name != "" {
		album.Name = name
	}
	album.Description = description
	album.CoverPhotoID = coverPhotoID
	if err := s.repo.UpdateAlbum(album); err != nil {
		return nil, err
	}
	return album, nil
}

// DeleteAlbum 删除相册。
func (s *PhotoService) DeleteAlbum(id int64, userID int64) error {
	return s.repo.DeleteAlbum(id, userID)
}

// GetAlbumDownloadEntries 获取相册下载条目。
func (s *PhotoService) GetAlbumDownloadEntries(albumID int64, userID int64) (string, []DownloadEntry, error) {
	album, err := s.repo.GetAlbumByID(albumID, userID)
	if err != nil {
		return "", nil, err
	}
	if album == nil {
		return "", nil, fmt.Errorf("相册不存在")
	}

	page, err := s.repo.ListAlbumPhotos(storage.ListAlbumPhotosParams{
		AlbumID: albumID,
		UserID:  userID,
		Limit:   10000,
	})
	if err != nil {
		return "", nil, err
	}

	entries := make([]DownloadEntry, 0, len(page.Photos))
	usedNames := map[string]int{}
	for _, photo := range page.Photos {
		name := makeUniqueDownloadName(photo.OriginalName, usedNames)
		entries = append(entries, DownloadEntry{
			FileName: name,
			Path:     s.MediaPath(photo),
			MimeType: photo.MimeType,
		})
	}

	albumName := album.Name
	if albumName == "" {
		albumName = "album"
	}
	return albumName, entries, nil
}

// AddPhoto 将媒体添加到相册。
func (s *PhotoService) AddPhoto(albumID int64, photoID int64, userID int64) error {
	return s.repo.AddPhotoToAlbum(albumID, photoID, userID)
}

// RemovePhoto 将媒体从相册移除。
func (s *PhotoService) RemovePhoto(albumID int64, photoID int64, userID int64) error {
	return s.repo.RemovePhotoFromAlbum(albumID, photoID, userID)
}

// GetPhoto 获取单张图片
func (s *PhotoService) GetPhoto(id int64, userID int64) (*storage.Photo, error) {
	return s.repo.GetPhotoByID(id, userID)
}

// GetPhotoByUUID ��过 UUID 获取单张图片（不含软删除）
func (s *PhotoService) GetPhotoByUUID(uuid string, userID int64) (*storage.Photo, error) {
	return s.repo.GetPhotoByUUID(uuid, userID)
}

// GetPhotoByUUIDAny 通过 UUID 获取图片，包含软删除（用于文件服务）
func (s *PhotoService) GetPhotoByUUIDAny(uuid string, userID int64) (*storage.Photo, error) {
	return s.repo.GetPhotoByUUIDAny(uuid, userID)
}

// DeletePhoto 软删除图片（移入回收站）
func (s *PhotoService) DeletePhoto(id int64, userID int64) error {
	return s.repo.SoftDeletePhoto(id, userID, userID)
}

// RestorePhoto 从回收站恢复图片
func (s *PhotoService) RestorePhoto(id int64, userID int64) error {
	return s.repo.RestorePhoto(id, userID)
}

// SetPhotoFavorite 设置收藏状态
func (s *PhotoService) SetPhotoFavorite(id int64, userID int64, favorite bool) error {
	return s.repo.SetPhotoFavorite(id, userID, favorite)
}

// PermanentlyDeletePhoto 彻底删除单张回收站图片，并清理磁盘文件。
func (s *PhotoService) PermanentlyDeletePhoto(id int64, userID int64) error {
	photo, err := s.repo.GetPhotoByIDAny(id, userID)
	if err != nil {
		return err
	}
	if photo == nil || photo.DeletedAt == nil {
		return fmt.Errorf("照片/视频不在回收站中")
	}
	if err := s.repo.HardDeletePhoto(id, userID); err != nil {
		return err
	}

	s.moveManagedFileToTrash(s.MediaPath(photo))
	for _, thumbPath := range s.ThumbnailCandidates(photo) {
		s.moveManagedFileToTrash(thumbPath)
	}
	return nil
}

// EmptyTrash 清空回收站，同时删除磁盘文件
func (s *PhotoService) EmptyTrash(userID int64) error {
	photos, err := s.repo.HardDeleteTrashedPhotos(userID)
	if err != nil {
		return fmt.Errorf("清空回收站失败: %w", err)
	}

	for _, photo := range photos {
		s.moveManagedFileToTrash(s.MediaPath(photo))
		for _, thumbPath := range s.ThumbnailCandidates(photo) {
			s.moveManagedFileToTrash(thumbPath)
		}
	}
	return nil
}

// RevealInFinder 在系统文件管理器中定位媒体文件。
func (s *PhotoService) RevealInFinder(id int64, userID int64) error {
	photo, err := s.repo.GetPhotoByIDAny(id, userID)
	if err != nil {
		return err
	}
	if photo == nil {
		return fmt.Errorf("照片/视频不存在")
	}

	target := s.resolveFinderPath(photo)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("文件不存在")
	}
	if err := revealInFileManager(target); err != nil {
		return fmt.Errorf("在文件管理器中打开失败: %w", err)
	}
	return nil
}

// PlayWithSystemPlayer 使用系统默认播放器打开媒体文件。
func (s *PhotoService) PlayWithSystemPlayer(id int64, userID int64) error {
	photo, err := s.repo.GetPhotoByIDAny(id, userID)
	if err != nil {
		return err
	}
	if photo == nil {
		return fmt.Errorf("照片/视频不存在")
	}

	target := s.resolveFinderPath(photo)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("文件不存在")
	}
	if err := openWithSystemDefault(target); err != nil {
		return fmt.Errorf("使用系统播放器打开失败: %w", err)
	}
	return nil
}

func revealInFileManager(target string) error {
	name, args := fileManagerRevealCommand(target)
	err := execCommand(name, args...).Run()
	if err == nil {
		return nil
	}
	if currentOS == "windows" {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil
		}
	}
	return err
}

func openWithSystemDefault(target string) error {
	name, args := systemOpenCommand(target)
	return execCommand(name, args...).Run()
}

func fileManagerRevealCommand(target string) (string, []string) {
	clean := filepath.Clean(target)
	switch currentOS {
	case "darwin":
		return "open", []string{"-R", clean}
	case "windows":
		return "explorer", []string{"/select,", clean}
	default:
		dir := filepath.Dir(clean)
		if dir == "" || dir == "." {
			dir = clean
		}
		return "xdg-open", []string{dir}
	}
}

func systemOpenCommand(target string) (string, []string) {
	clean := filepath.Clean(target)
	switch currentOS {
	case "darwin":
		return "open", []string{clean}
	case "windows":
		return "cmd", []string{"/c", "start", "", clean}
	default:
		return "xdg-open", []string{clean}
	}
}

func (s *PhotoService) resolveFinderPath(photo *storage.Photo) string {
	if photo.SourceRelPath != "" {
		sourcePath := filepath.Join(s.sourcePath, photo.SourceRelPath)
		if _, err := os.Stat(sourcePath); err == nil {
			return sourcePath
		}
	}
	return s.MediaPath(photo)
}

func (s *PhotoService) moveManagedFileToTrash(path string) {
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}

	rel, err := filepath.Rel(s.dataPath, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	baseDir := s.trashPath
	if baseDir == "" {
		baseDir = filepath.Join(s.dataPath, "Trash")
	}
	dest := filepath.Join(baseDir, rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return
	}
	if err := os.Rename(path, uniqueTrashPath(dest)); err != nil {
		return
	}
}

func uniqueTrashPath(dest string) string {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(dest)
	base := dest[:len(dest)-len(ext)]
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
