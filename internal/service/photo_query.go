package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"echogallery/internal/storage"
)

// folderAlbumTrashRepository is intentionally optional. The service keeps the
// general storage.Repository small so lightweight test repositories and future
// backends can continue to support personal albums without implementing the
// folder-trash feature immediately.
type folderAlbumTrashRepository interface {
	SoftDeleteFolderAlbumTree(id, userID, deletedBy int64, groupID string) (*storage.FolderAlbumTrashEntry, error)
	ListTrashedFolderAlbumPaths(userID int64) (map[string]bool, error)
	ListTrashedFolderAlbums(userID int64) ([]*storage.FolderAlbumTrashEntry, error)
	RestoreFolderAlbumTrash(groupID string, userID int64) error
	HardDeleteFolderAlbumTrash(groupID string, userID int64) ([]*storage.Photo, error)
	HardDeleteTrashedFolderAlbums(userID int64) error
}

var (
	execCommand    = exec.Command
	currentOS      = runtime.GOOS
	moveFileRename = os.Rename
)

// GetTimeline 获取时间线图片（游标分页）
func (s *PhotoService) GetTimeline(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	page, err := s.repo.ListPhotos(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
}

func (s *PhotoService) LocateTimelineWindow(params storage.LocateTimelineParams) (*storage.TimelineLocateResult, error) {
	result, err := s.repo.LocateTimelineWindow(params)
	if err != nil || result == nil {
		return result, err
	}
	page := s.filterExistingMediaPage(&storage.PhotoPage{Photos: result.Photos})
	result.Photos = page.Photos
	if result.TargetIndex >= len(result.Photos) {
		result.TargetIndex = len(result.Photos) - 1
	}
	if result.TargetIndex < 0 {
		result.TargetIndex = 0
	}
	return result, nil
}

func (s *PhotoService) GetTimelineBefore(params storage.LocateTimelineParams) (*storage.PhotoPage, error) {
	page, err := s.repo.ListPhotosBefore(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
}

func (s *PhotoService) LocateAlbumWindow(params storage.LocateAlbumParams) (*storage.TimelineLocateResult, error) {
	result, err := s.repo.LocateAlbumWindow(params)
	if err != nil || result == nil {
		return result, err
	}
	page := s.filterExistingMediaPage(&storage.PhotoPage{Photos: result.Photos})
	result.Photos = page.Photos
	if result.TargetIndex >= len(result.Photos) {
		result.TargetIndex = len(result.Photos) - 1
	}
	if result.TargetIndex < 0 {
		result.TargetIndex = 0
	}
	return result, nil
}

func (s *PhotoService) GetAlbumBefore(params storage.LocateAlbumParams) (*storage.PhotoPage, error) {
	page, err := s.repo.ListAlbumPhotosBefore(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
}

// GetTrash 获取回收站图片（游标分页）
func (s *PhotoService) GetTrash(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	page, err := s.repo.ListTrashedPhotos(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
}

// GetTrashAlbums returns folder deletion batches, one entry per deleted root
// folder. Child folders are represented by the same group and are not emitted
// as separate recycle-bin cards.
func (s *PhotoService) GetTrashAlbums(userID int64) ([]*storage.FolderAlbumTrashEntry, error) {
	repo, ok := s.repo.(folderAlbumTrashRepository)
	if !ok {
		return nil, fmt.Errorf("当前数据库不支持文件夹相册回收站")
	}
	return repo.ListTrashedFolderAlbums(userID)
}

// GetFavorites 获取个人收藏（游标分页）
func (s *PhotoService) GetFavorites(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	page, err := s.repo.ListFavoritePhotos(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
}

// SearchMedia 搜索用户媒体（游标分页）。
func (s *PhotoService) SearchMedia(params storage.SearchPhotosParams) (*storage.PhotoPage, error) {
	page, err := s.repo.SearchPhotos(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
}

// GetRandomMedia 获取乱序相册媒体（游标分页）。
func (s *PhotoService) GetRandomMedia(params storage.RandomPhotosParams) (*storage.PhotoPage, error) {
	page, err := s.repo.ListRandomPhotos(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
}

// GetAlbumMedia 获取相册内媒体（游标分页）。
func (s *PhotoService) GetAlbumMedia(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error) {
	page, err := s.repo.ListAlbumPhotos(params)
	if err != nil {
		return nil, err
	}
	return s.filterExistingMediaPage(page), nil
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

func (s *PhotoService) syncPortableDatabase() error {
	if s == nil || s.repo == nil {
		return nil
	}
	if syncer, ok := s.repo.(interface{ SyncPortable() error }); ok {
		return syncer.SyncPortable()
	}
	return nil
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
	if err := s.syncPortableDatabase(); err != nil {
		return nil, fmt.Errorf("保存相册到资源库数据库失败: %w", err)
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
	if err := s.syncPortableDatabase(); err != nil {
		return nil, fmt.Errorf("保存相册到资源库数据库失败: %w", err)
	}
	return album, nil
}

// DeleteAlbum 删除相册。
func (s *PhotoService) DeleteAlbum(id int64, userID int64) error {
	album, err := s.repo.GetAlbumByID(id, userID)
	if err != nil {
		return err
	}
	if album == nil {
		return fmt.Errorf("相册不存在")
	}
	if isAutoFolderAlbum(album) {
		trashRepo, ok := s.repo.(folderAlbumTrashRepository)
		if !ok {
			return fmt.Errorf("当前数据库不支持文件夹相册回收站")
		}
		if _, err := trashRepo.SoftDeleteFolderAlbumTree(id, userID, userID, uuid.NewString()); err != nil {
			return err
		}
		return s.syncPortableDatabase()
	}
	if err := s.repo.DeleteAlbum(id, userID); err != nil {
		return err
	}
	return s.syncPortableDatabase()
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
	page = s.filterExistingMediaPage(page)

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
	if err := s.repo.AddPhotoToAlbum(albumID, photoID, userID); err != nil {
		return err
	}
	return s.syncPortableDatabase()
}

// RemovePhoto 将媒体从相册移除。
func (s *PhotoService) RemovePhoto(albumID int64, photoID int64, userID int64) error {
	if err := s.repo.RemovePhotoFromAlbum(albumID, photoID, userID); err != nil {
		return err
	}
	return s.syncPortableDatabase()
}

// GetPhoto 获取单张图片
func (s *PhotoService) GetPhoto(id int64, userID int64) (*storage.Photo, error) {
	photo, err := s.repo.GetPhotoByID(id, userID)
	if err != nil || photo == nil {
		return photo, err
	}
	if !s.mediaFileExists(photo) {
		return nil, nil
	}
	return photo, nil
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
	if err := s.repo.SoftDeletePhoto(id, userID, userID); err != nil {
		return err
	}
	return nil
}

// RestorePhoto 从回收站恢复图片
func (s *PhotoService) RestorePhoto(id int64, userID int64) error {
	if err := s.repo.RestorePhoto(id, userID); err != nil {
		return err
	}
	return nil
}

// RestoreFolderAlbum restores an entire folder deletion batch, including its
// child folder albums and media records.
func (s *PhotoService) RestoreFolderAlbum(groupID string, userID int64) error {
	repo, ok := s.repo.(folderAlbumTrashRepository)
	if !ok {
		return fmt.Errorf("当前数据库不支持文件夹相册回收站")
	}
	if err := repo.RestoreFolderAlbumTrash(groupID, userID); err != nil {
		return err
	}
	return s.syncPortableDatabase()
}

// SetPhotoFavorite 设置收藏状态
func (s *PhotoService) SetPhotoFavorite(id int64, userID int64, favorite bool, superFavorite bool) error {
	return s.repo.SetPhotoFavorite(id, userID, favorite, superFavorite)
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

	s.moveMediaToTrash(s.existingMediaPath(photo))
	s.removeThumbnailFiles(photo)
	return nil
}

// PermanentlyDeleteFolderAlbum removes one folder deletion batch from the
// database after moving the entire source directory tree to
// .echogallery/.trash. Thumbnails remain managed separately because they do
// not live inside the source folder.
func (s *PhotoService) PermanentlyDeleteFolderAlbum(groupID string, userID int64) error {
	repo, ok := s.repo.(folderAlbumTrashRepository)
	if !ok {
		return fmt.Errorf("当前数据库不支持文件夹相册回收站")
	}
	entry, err := s.trashedFolderAlbumEntry(groupID, userID)
	if err != nil {
		return err
	}
	// Folder batches are moved as one directory so nested folders and
	// unsupported files keep their original layout in .trash.
	if err := s.moveFolderAlbumToTrash(entry); err != nil {
		return fmt.Errorf("移动文件夹到 .trash 失败: %w", err)
	}
	photos, err := repo.HardDeleteFolderAlbumTrash(groupID, userID)
	if err != nil {
		return err
	}
	for _, photo := range photos {
		s.removeThumbnailFiles(photo)
	}
	return s.syncPortableDatabase()
}

// EmptyTrash 清空回收站，同时删除磁盘文件
func (s *PhotoService) EmptyTrash(userID int64) error {
	var folderEntries []*storage.FolderAlbumTrashEntry
	var folderRepo folderAlbumTrashRepository
	if candidate, ok := s.repo.(folderAlbumTrashRepository); ok {
		entries, err := candidate.ListTrashedFolderAlbums(userID)
		if err != nil {
			return fmt.Errorf("读取文件夹回收站失败: %w", err)
		}
		folderEntries = entries
		folderRepo = candidate
		for _, entry := range folderEntries {
			if err := s.moveFolderAlbumToTrash(entry); err != nil {
				return fmt.Errorf("移动文件夹到 .trash 失败: %w", err)
			}
		}
	}
	photos, err := s.repo.HardDeleteTrashedPhotos(userID)
	if err != nil {
		return fmt.Errorf("清空回收站失败: %w", err)
	}

	for _, photo := range photos {
		s.moveMediaToTrash(s.existingMediaPath(photo))
		s.removeThumbnailFiles(photo)
	}
	if folderRepo != nil {
		if err := folderRepo.HardDeleteTrashedFolderAlbums(userID); err != nil {
			return fmt.Errorf("清空文件夹相册回收站失败: %w", err)
		}
	}
	if err := s.syncPortableDatabase(); err != nil {
		return fmt.Errorf("同步资源库数据库失败: %w", err)
	}
	return nil
}

func (s *PhotoService) trashedFolderAlbumEntry(groupID string, userID int64) (*storage.FolderAlbumTrashEntry, error) {
	repo, ok := s.repo.(folderAlbumTrashRepository)
	if !ok {
		return nil, fmt.Errorf("当前数据库不支持文件夹相册回收站")
	}
	key := strings.TrimSpace(groupID)
	if key == "" {
		return nil, fmt.Errorf("文件夹回收站批次无效")
	}
	entries, err := repo.ListTrashedFolderAlbums(userID)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry != nil && strings.TrimSpace(entry.GroupID) == key {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("文件夹回收站批次不存在")
}

func (s *PhotoService) moveFolderAlbumToTrash(entry *storage.FolderAlbumTrashEntry) error {
	if entry == nil || strings.TrimSpace(s.sourcePath) == "" {
		return nil
	}
	relPath, err := safeLibraryRelativePath(entry.SourceRelPath)
	if err != nil {
		return err
	}
	groupID, err := safeTrashGroupID(entry.GroupID)
	if err != nil {
		return err
	}
	sourcePath := filepath.Join(s.sourcePath, relPath)
	destination := filepath.Join(s.trashBaseDir(), "folders", groupID, relPath)

	info, sourceErr := os.Stat(sourcePath)
	if sourceErr != nil {
		if os.IsNotExist(sourceErr) {
			// A previous attempt may already have moved the folder, or the
			// source folder may have been removed outside EchoGallery.
			if destinationInfo, destinationErr := os.Stat(destination); destinationErr == nil && destinationInfo.IsDir() {
				return nil
			}
			return nil
		}
		return sourceErr
	}
	if !info.IsDir() {
		return fmt.Errorf("源路径不是文件夹: %s", sourcePath)
	}
	if destinationInfo, destinationErr := os.Stat(destination); destinationErr == nil {
		if destinationInfo.IsDir() {
			return fmt.Errorf(".trash 中已存在同名文件夹: %s", destination)
		}
		return fmt.Errorf(".trash 中已存在同名文件: %s", destination)
	} else if !os.IsNotExist(destinationErr) {
		return destinationErr
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	return moveDirectoryToTrash(sourcePath, destination)
}

func safeTrashGroupID(value string) (string, error) {
	groupID := strings.TrimSpace(value)
	if groupID == "" || groupID == "." || groupID == ".." || filepath.Base(groupID) != groupID || filepath.VolumeName(groupID) != "" {
		return "", fmt.Errorf("文件夹回收站批次无效")
	}
	return groupID, nil
}

func safeLibraryRelativePath(value string) (string, error) {
	normalized := strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/")
	if normalized == "" {
		return "", fmt.Errorf("文件夹相对路径为空")
	}
	clean := filepath.Clean(filepath.FromSlash(normalized))
	if clean == "." || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("文件夹相对路径无效: %s", value)
	}
	return clean, nil
}

func moveDirectoryToTrash(src, dest string) error {
	if err := moveFileRename(src, dest); err == nil {
		return nil
	}
	if err := copyDirectory(src, dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	return nil
}

func copyDirectory(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := dest
		if rel != "." {
			target = filepath.Join(dest, rel)
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		return nil
	})
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
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

// RevealAlbumInFinder 在系统文件管理器中定位文件夹相册。
func (s *PhotoService) RevealAlbumInFinder(id int64, userID int64) error {
	album, err := s.repo.GetAlbumByID(id, userID)
	if err != nil {
		return err
	}
	if album == nil || !isAutoFolderAlbum(album) {
		return fmt.Errorf("相册不存在")
	}
	relPath := autoFolderAlbumPath(album)
	if relPath == "" {
		return fmt.Errorf("相册没有可打开的文件夹路径")
	}
	cleanRel := filepath.Clean(relPath)
	if filepath.IsAbs(cleanRel) || cleanRel == "." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return fmt.Errorf("相册路径无效")
	}
	target := filepath.Join(s.sourcePath, cleanRel)
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("文件夹不存在")
	}
	if !info.IsDir() {
		return fmt.Errorf("相册路径不是文件夹")
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
	if err := runFileManagerCommand(name, args...); err != nil {
		return err
	}
	if currentOS == "windows" {
		name, args = windowsFileManagerFocusCommand()
		if err := execCommand(name, args...).Run(); err != nil {
			return fmt.Errorf("激活资源管理器窗口失败: %w", err)
		}
	}
	return nil
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

func windowsFileManagerFocusCommand() (string, []string) {
	script := `$ErrorActionPreference = 'Stop'; ` +
		`Start-Sleep -Milliseconds 300; ` +
		`$explorer = Get-Process explorer -ErrorAction SilentlyContinue | ` +
		`Where-Object { $_.MainWindowHandle -ne 0 } | ` +
		`Sort-Object StartTime -Descending | ` +
		`Select-Object -First 1; ` +
		`if (-not $explorer) { exit 0 }; ` +
		`$shell = New-Object -ComObject WScript.Shell; ` +
		`if (-not $shell.AppActivate($explorer.Id)) { exit 1 }`
	return "powershell", []string{"-NoProfile", "-WindowStyle", "Hidden", "-Command", script}
}

func runFileManagerCommand(name string, args ...string) error {
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

func (s *PhotoService) mediaFileExists(photo *storage.Photo) bool {
	if photo == nil {
		return false
	}
	for _, path := range s.mediaFileCandidates(photo) {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func (s *PhotoService) mediaFileCandidates(photo *storage.Photo) []string {
	if photo == nil {
		return nil
	}
	candidates := make([]string, 0, 3)
	add := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		for _, existing := range candidates {
			if existing == clean {
				return
			}
		}
		candidates = append(candidates, clean)
	}
	if photo.SourceRelPath != "" {
		add(filepath.Join(s.sourcePath, photo.SourceRelPath))
	}
	if photo.StorageRelPath != "" {
		add(filepath.Join(s.dataPath, photo.StorageRelPath))
	}
	if photo.UUID != "" && photo.OriginalName != "" {
		add(filepath.Join(s.dataPath, photo.UUID+filepath.Ext(photo.OriginalName)))
	}
	return candidates
}

func (s *PhotoService) filterExistingMediaPage(page *storage.PhotoPage) *storage.PhotoPage {
	if page == nil || len(page.Photos) == 0 {
		return page
	}
	filtered := page.Photos[:0]
	removed := 0
	for _, photo := range page.Photos {
		if s.mediaFileExists(photo) {
			filtered = append(filtered, photo)
			continue
		}
		removed++
	}
	page.Photos = filtered
	if removed > 0 && page.Total >= removed {
		page.Total -= removed
	}
	return page
}

func (s *PhotoService) collectTrashLinks(userID int64) ([]string, error) {
	seen := make(map[string]struct{})
	links := make([]string, 0)
	cursor := ""
	for {
		page, err := s.repo.ListTrashedPhotos(storage.ListPhotosParams{
			UserID: userID,
			Cursor: cursor,
			Limit:  500,
		})
		if err != nil {
			return nil, err
		}
		for _, photo := range page.Photos {
			target := s.trashLinkTarget(photo)
			if target == "" {
				continue
			}
			if _, ok := seen[target]; ok {
				continue
			}
			seen[target] = struct{}{}
			links = append(links, target)
		}
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	sort.Strings(links)
	return links, nil
}

func (s *PhotoService) trashBaseDir() string {
	if strings.TrimSpace(s.sourcePath) != "" {
		return filepath.Clean(filepath.Join(s.sourcePath, ".echogallery", ".trash"))
	}
	if s.trashPath != "" {
		return filepath.Clean(s.trashPath)
	}
	return filepath.Join(s.dataPath, "trash")
}

func (s *PhotoService) trashLinkTarget(photo *storage.Photo) string {
	if photo == nil {
		return ""
	}
	return filepath.ToSlash(photo.SourceRelPath)
}

func (s *PhotoService) existingMediaPath(photo *storage.Photo) string {
	for _, path := range s.mediaFileCandidates(photo) {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	return s.MediaPath(photo)
}

func (s *PhotoService) moveMediaToTrash(path string) {
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}

	baseDir := s.trashBaseDir()
	dest := filepath.Join(baseDir, filepath.Base(path))
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return
	}
	_ = moveFileToTrash(path, uniqueTrashPath(dest))
}

func (s *PhotoService) removeThumbnailFiles(photo *storage.Photo) {
	for _, path := range s.thumbnailCleanupPaths(photo) {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			_ = os.Remove(path)
		}
	}
}

func moveFileToTrash(src, dest string) error {
	if err := moveFileRename(src, dest); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if err := os.Chmod(dest, info.Mode()); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if err := os.Chtimes(dest, info.ModTime(), info.ModTime()); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if err := os.Remove(src); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
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
