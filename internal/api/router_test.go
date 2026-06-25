package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"echogallery/internal/config"
	imgpkg "echogallery/internal/image"
	"echogallery/internal/pagesession"
	"echogallery/internal/service"
	"echogallery/internal/sessionlock"
	"echogallery/internal/storage"
)

type stubRegistrar struct {
	addPhoto                       func(albumID int64, photoID int64, userID int64) error
	createAlbum                    func(name, description string, userID int64) (*storage.Album, error)
	createShare                    func(input service.CreateShareInput) (*storage.ShareLink, error)
	deleteAlbum                    func(id int64, userID int64) error
	deleteShare                    func(id int64, userID int64) error
	getAlbum                       func(id int64, userID int64) (*storage.Album, error)
	getAlbumDownloadEntries        func(albumID int64, userID int64) (string, []service.DownloadEntry, error)
	getShareByToken                func(token string) (*storage.ShareLink, error)
	listAlbums                     func(userID int64) ([]*storage.Album, error)
	listAlbumsForPhoto             func(photoID int64, userID int64) ([]*storage.Album, error)
	listShares                     func(userID int64) ([]*storage.ShareLink, error)
	removePhoto                    func(albumID int64, photoID int64, userID int64) error
	updateAlbum                    func(id int64, name, description string, coverPhotoID *int64, userID int64) (*storage.Album, error)
	register                       func(input service.RegisterUploadedVideoInput) (*storage.Photo, error)
	upload                         func(input service.UploadInput) (*service.UploadResult, error)
	deletePhoto                    func(id int64, userID int64) error
	emptyTrash                     func(userID int64) error
	getDownloadEntries             func(photoIDs []int64, userID int64) ([]service.DownloadEntry, error)
	getAlbumMedia                  func(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error)
	getPhoto                       func(id int64, userID int64) (*storage.Photo, error)
	getByUUID                      func(uuid string, userID int64) (*storage.Photo, error)
	getFavorites                   func(params storage.ListPhotosParams) (*storage.PhotoPage, error)
	getTrash                       func(params storage.ListPhotosParams) (*storage.PhotoPage, error)
	getTimeline                    func(params storage.ListPhotosParams) (*storage.PhotoPage, error)
	locateTimelineWindow           func(params storage.LocateTimelineParams) (*storage.TimelineLocateResult, error)
	getTimelineBefore              func(params storage.LocateTimelineParams) (*storage.PhotoPage, error)
	locateAlbumWindow              func(params storage.LocateAlbumParams) (*storage.TimelineLocateResult, error)
	getAlbumBefore                 func(params storage.LocateAlbumParams) (*storage.PhotoPage, error)
	searchMedia                    func(params storage.SearchPhotosParams) (*storage.PhotoPage, error)
	getRandomMedia                 func(params storage.RandomPhotosParams) (*storage.PhotoPage, error)
	mediaPath                      func(photo *storage.Photo) string
	browserPlaybackPath            func(photo *storage.Photo) (string, string, error)
	listPlaybackCaches             func(userID int64) ([]service.PlaybackCacheEntry, error)
	buildBrowserPlaybackCache      func(photoID int64, userID int64) (*service.PlaybackCacheEntry, error)
	deletePlaybackCaches           func(userID int64, uuids []string) (int, error)
	startPlaybackCacheBuild        func(userID int64) service.PlaybackCacheBuildStatus
	getPlaybackCacheBuildStatus    func(userID int64) service.PlaybackCacheBuildStatus
	cancelPlaybackCacheBuild       func(userID int64) service.PlaybackCacheBuildStatus
	permanentlyDeletePhoto         func(id int64, userID int64) error
	playWithSystemPlayer           func(id int64, userID int64) error
	posterPath                     func(photo *storage.Photo) string
	refreshVideoThumbnails         func(userID int64) (*service.VideoThumbnailRefreshResult, error)
	startVideoThumbnailRefresh     func(userID int64) (service.VideoThumbnailRefreshStatus, error)
	getVideoThumbnailRefreshStatus func(userID int64) service.VideoThumbnailRefreshStatus
	cancelVideoThumbnailRefresh    func(userID int64) (service.VideoThumbnailRefreshStatus, error)
	getVideoPlaybackPreference     func(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error)
	saveVideoPlaybackPreference    func(photoID int64, userID int64, volume float64, muted bool, resumeTime int64, bookmarks []storage.VideoPlaybackBookmark) (*storage.VideoPlaybackPreference, error)
	revealAlbumInFinder            func(id int64, userID int64) error
	revealInFinder                 func(id int64, userID int64) error
	restorePhoto                   func(id int64, userID int64) error
	setPhotoFavorite               func(id int64, userID int64, favorite bool, superFavorite bool) error
	thumbnailPath                  func(photo *storage.Photo) string
}

func (s stubRegistrar) RegisterUploadedVideo(input service.RegisterUploadedVideoInput) (*storage.Photo, error) {
	return s.register(input)
}

func (s stubRegistrar) Upload(input service.UploadInput) (*service.UploadResult, error) {
	return s.upload(input)
}

func (s stubRegistrar) AddPhoto(albumID int64, photoID int64, userID int64) error {
	return s.addPhoto(albumID, photoID, userID)
}

func (s stubRegistrar) CreateAlbum(name, description string, userID int64) (*storage.Album, error) {
	return s.createAlbum(name, description, userID)
}

func (s stubRegistrar) CreateShare(input service.CreateShareInput) (*storage.ShareLink, error) {
	return s.createShare(input)
}

func (s stubRegistrar) DeleteAlbum(id int64, userID int64) error {
	return s.deleteAlbum(id, userID)
}

func (s stubRegistrar) DeleteShare(id int64, userID int64) error {
	return s.deleteShare(id, userID)
}

func (s stubRegistrar) UpdateAlbum(id int64, name, description string, coverPhotoID *int64, userID int64) (*storage.Album, error) {
	return s.updateAlbum(id, name, description, coverPhotoID, userID)
}

func (s stubRegistrar) GetAlbum(id int64, userID int64) (*storage.Album, error) {
	return s.getAlbum(id, userID)
}

func (s stubRegistrar) GetShareByToken(token string) (*storage.ShareLink, error) {
	return s.getShareByToken(token)
}

func (s stubRegistrar) GetAlbumDownloadEntries(albumID int64, userID int64) (string, []service.DownloadEntry, error) {
	return s.getAlbumDownloadEntries(albumID, userID)
}

func (s stubRegistrar) ListAlbums(userID int64) ([]*storage.Album, error) {
	return s.listAlbums(userID)
}

func (s stubRegistrar) ListAlbumsForPhoto(photoID int64, userID int64) ([]*storage.Album, error) {
	if s.listAlbumsForPhoto != nil {
		return s.listAlbumsForPhoto(photoID, userID)
	}
	return s.listAlbums(userID)
}

func (s stubRegistrar) ListShares(userID int64) ([]*storage.ShareLink, error) {
	return s.listShares(userID)
}

func (s stubRegistrar) RemovePhoto(albumID int64, photoID int64, userID int64) error {
	return s.removePhoto(albumID, photoID, userID)
}

func (s stubRegistrar) DeletePhoto(id int64, userID int64) error {
	return s.deletePhoto(id, userID)
}

func (s stubRegistrar) EmptyTrash(userID int64) error {
	return s.emptyTrash(userID)
}

func (s stubRegistrar) GetDownloadEntries(photoIDs []int64, userID int64) ([]service.DownloadEntry, error) {
	return s.getDownloadEntries(photoIDs, userID)
}

func (s stubRegistrar) GetAlbumMedia(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error) {
	return s.getAlbumMedia(params)
}

func (s stubRegistrar) GetTrash(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	return s.getTrash(params)
}

func (s stubRegistrar) GetFavorites(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	return s.getFavorites(params)
}

func (s stubRegistrar) GetPhoto(id int64, userID int64) (*storage.Photo, error) {
	return s.getPhoto(id, userID)
}

func (s stubRegistrar) GetPhotoByUUIDAny(uuid string, userID int64) (*storage.Photo, error) {
	return s.getByUUID(uuid, userID)
}

func (s stubRegistrar) GetTimeline(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	return s.getTimeline(params)
}

func (s stubRegistrar) LocateTimelineWindow(params storage.LocateTimelineParams) (*storage.TimelineLocateResult, error) {
	if s.locateTimelineWindow == nil {
		return nil, nil
	}
	return s.locateTimelineWindow(params)
}

func (s stubRegistrar) GetTimelineBefore(params storage.LocateTimelineParams) (*storage.PhotoPage, error) {
	if s.getTimelineBefore == nil {
		return &storage.PhotoPage{}, nil
	}
	return s.getTimelineBefore(params)
}

func (s stubRegistrar) LocateAlbumWindow(params storage.LocateAlbumParams) (*storage.TimelineLocateResult, error) {
	if s.locateAlbumWindow == nil {
		return nil, nil
	}
	return s.locateAlbumWindow(params)
}

func (s stubRegistrar) GetAlbumBefore(params storage.LocateAlbumParams) (*storage.PhotoPage, error) {
	if s.getAlbumBefore == nil {
		return &storage.PhotoPage{}, nil
	}
	return s.getAlbumBefore(params)
}

func (s stubRegistrar) SearchMedia(params storage.SearchPhotosParams) (*storage.PhotoPage, error) {
	if s.searchMedia == nil {
		return &storage.PhotoPage{}, nil
	}
	return s.searchMedia(params)
}

func (s stubRegistrar) GetRandomMedia(params storage.RandomPhotosParams) (*storage.PhotoPage, error) {
	if s.getRandomMedia == nil {
		return &storage.PhotoPage{}, nil
	}
	return s.getRandomMedia(params)
}

func (s stubRegistrar) MediaPath(photo *storage.Photo) string {
	return s.mediaPath(photo)
}

func (s stubRegistrar) BrowserPlaybackPath(photo *storage.Photo) (string, string, error) {
	if s.browserPlaybackPath != nil {
		return s.browserPlaybackPath(photo)
	}
	return s.MediaPath(photo), photo.MimeType, nil
}

func (s stubRegistrar) ListPlaybackCaches(userID int64) ([]service.PlaybackCacheEntry, error) {
	if s.listPlaybackCaches != nil {
		return s.listPlaybackCaches(userID)
	}
	return nil, nil
}

func (s stubRegistrar) BuildBrowserPlaybackCache(photoID int64, userID int64) (*service.PlaybackCacheEntry, error) {
	if s.buildBrowserPlaybackCache != nil {
		return s.buildBrowserPlaybackCache(photoID, userID)
	}
	return nil, nil
}

func (s stubRegistrar) DeletePlaybackCaches(userID int64, uuids []string) (int, error) {
	if s.deletePlaybackCaches != nil {
		return s.deletePlaybackCaches(userID, uuids)
	}
	return 0, nil
}

func (s stubRegistrar) StartPlaybackCacheBuild(userID int64) service.PlaybackCacheBuildStatus {
	if s.startPlaybackCacheBuild != nil {
		return s.startPlaybackCacheBuild(userID)
	}
	return service.PlaybackCacheBuildStatus{Status: "running", Message: "ok"}
}

func (s stubRegistrar) GetPlaybackCacheBuildStatus(userID int64) service.PlaybackCacheBuildStatus {
	if s.getPlaybackCacheBuildStatus != nil {
		return s.getPlaybackCacheBuildStatus(userID)
	}
	return service.PlaybackCacheBuildStatus{Status: "idle", Message: "当前没有播放兼容缓存任务"}
}

func (s stubRegistrar) CancelPlaybackCacheBuild(userID int64) service.PlaybackCacheBuildStatus {
	if s.cancelPlaybackCacheBuild != nil {
		return s.cancelPlaybackCacheBuild(userID)
	}
	return service.PlaybackCacheBuildStatus{Status: "cancelled", Message: "已取消播放兼容缓存任务"}
}

func (s stubRegistrar) PosterPath(photo *storage.Photo) string {
	if s.posterPath == nil {
		return ""
	}
	return s.posterPath(photo)
}

func (s stubRegistrar) ThumbnailPath(photo *storage.Photo) string {
	if s.thumbnailPath != nil {
		return s.thumbnailPath(photo)
	}
	if photo != nil && photo.MediaKind == storage.MediaKindVideo && s.posterPath != nil {
		return s.posterPath(photo)
	}
	return ""
}

func (s stubRegistrar) ThumbnailCandidates(photo *storage.Photo) []string {
	if photo == nil {
		return nil
	}
	if path := s.ThumbnailPath(photo); path != "" {
		return []string{path}
	}
	return nil
}

func (s stubRegistrar) PermanentlyDeletePhoto(id int64, userID int64) error {
	return s.permanentlyDeletePhoto(id, userID)
}

func (s stubRegistrar) PlayWithSystemPlayer(id int64, userID int64) error {
	return s.playWithSystemPlayer(id, userID)
}

func (s stubRegistrar) RefreshVideoThumbnails(userID int64) (*service.VideoThumbnailRefreshResult, error) {
	if s.refreshVideoThumbnails == nil {
		return &service.VideoThumbnailRefreshResult{}, nil
	}
	return s.refreshVideoThumbnails(userID)
}

func (s stubRegistrar) StartVideoThumbnailRefresh(userID int64) (service.VideoThumbnailRefreshStatus, error) {
	if s.startVideoThumbnailRefresh == nil {
		return service.VideoThumbnailRefreshStatus{Status: "running", Message: "ok"}, nil
	}
	return s.startVideoThumbnailRefresh(userID)
}

func (s stubRegistrar) GetVideoThumbnailRefreshStatus(userID int64) service.VideoThumbnailRefreshStatus {
	if s.getVideoThumbnailRefreshStatus == nil {
		return service.VideoThumbnailRefreshStatus{Status: "idle", Message: "当前没有视频缩略图任务"}
	}
	return s.getVideoThumbnailRefreshStatus(userID)
}

func (s stubRegistrar) CancelVideoThumbnailRefresh(userID int64) (service.VideoThumbnailRefreshStatus, error) {
	if s.cancelVideoThumbnailRefresh == nil {
		return service.VideoThumbnailRefreshStatus{Status: "cancelled", Message: "已取消视频缩略图刷新"}, nil
	}
	return s.cancelVideoThumbnailRefresh(userID)
}

func (s stubRegistrar) GetVideoPlaybackPreference(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error) {
	if s.getVideoPlaybackPreference == nil {
		return nil, nil
	}
	return s.getVideoPlaybackPreference(photoID, userID)
}

func (s stubRegistrar) SaveVideoPlaybackPreference(photoID int64, userID int64, volume float64, muted bool, resumeTime int64, bookmarks []storage.VideoPlaybackBookmark) (*storage.VideoPlaybackPreference, error) {
	if s.saveVideoPlaybackPreference == nil {
		return &storage.VideoPlaybackPreference{
			PhotoID:    photoID,
			Volume:     volume,
			Muted:      muted,
			ResumeTime: resumeTime,
			Bookmarks:  append([]storage.VideoPlaybackBookmark(nil), bookmarks...),
		}, nil
	}
	return s.saveVideoPlaybackPreference(photoID, userID, volume, muted, resumeTime, bookmarks)
}

func (s stubRegistrar) RevealInFinder(id int64, userID int64) error {
	return s.revealInFinder(id, userID)
}

func (s stubRegistrar) RevealAlbumInFinder(id int64, userID int64) error {
	if s.revealAlbumInFinder != nil {
		return s.revealAlbumInFinder(id, userID)
	}
	return nil
}

func (s stubRegistrar) RestorePhoto(id int64, userID int64) error {
	return s.restorePhoto(id, userID)
}

func (s stubRegistrar) SetPhotoFavorite(id int64, userID int64, favorite bool, superFavorite bool) error {
	return s.setPhotoFavorite(id, userID, favorite, superFavorite)
}

func okRegistrar() stubRegistrar {
	mediaFilePath := func(photo *storage.Photo) string {
		return filepath.Join(tTempStoragePath, photo.UUID+".mp4")
	}
	posterFilePath := func(photo *storage.Photo) string {
		return imgpkg.ThumbnailShardPath(filepath.Join(tTempStoragePath, ".thumbnails"), photo.UUID)
	}
	thumbnailFilePath := func(photo *storage.Photo) string {
		return imgpkg.ThumbnailShardPath(filepath.Join(tTempStoragePath, ".thumbnails"), photo.UUID)
	}
	return stubRegistrar{addPhoto: func(albumID int64, photoID int64, userID int64) error {
		return nil
	}, createAlbum: func(name, description string, userID int64) (*storage.Album, error) {
		return &storage.Album{ID: 8, Name: name, Description: description, CreatedBy: userID, CreatedAt: time.Now()}, nil
	}, createShare: func(input service.CreateShareInput) (*storage.ShareLink, error) {
		return &storage.ShareLink{ID: 3, Token: "new-token", Type: input.Type, TargetID: input.TargetID, CreatedBy: input.UserID, ExpiresAt: input.ExpiresAt, CreatedAt: time.Now()}, nil
	}, deleteAlbum: func(id int64, userID int64) error {
		return nil
	}, deleteShare: func(id int64, userID int64) error {
		return nil
	}, getAlbum: func(id int64, userID int64) (*storage.Album, error) {
		coverID := int64(12)
		return &storage.Album{ID: id, Name: "旅行", Description: "相册描述", CreatedBy: userID, CoverPhotoID: &coverID, PhotoCount: 2}, nil
	}, getShareByToken: func(token string) (*storage.ShareLink, error) {
		if token == "missing" {
			return nil, nil
		}
		return &storage.ShareLink{ID: 4, Token: token, Type: storage.ShareTypePhoto, TargetID: 11, CreatedBy: 1, CreatedAt: time.Now()}, nil
	}, getAlbumDownloadEntries: func(albumID int64, userID int64) (string, []service.DownloadEntry, error) {
		return "旅行", []service.DownloadEntry{{FileName: "album.mp4", Path: mediaFilePath(&storage.Photo{UUID: "media-1"}), MimeType: "video/mp4"}}, nil
	}, listAlbums: func(userID int64) ([]*storage.Album, error) {
		coverID := int64(12)
		return []*storage.Album{
			{ID: 1, Name: "旅行", Description: "春游", CreatedBy: userID, CoverPhotoID: &coverID, PhotoCount: 2},
			{ID: 2, Name: "收藏", Description: "混合媒体", CreatedBy: userID, PhotoCount: 5},
		}, nil
	}, listAlbumsForPhoto: func(photoID int64, userID int64) ([]*storage.Album, error) {
		if photoID == 11 {
			return []*storage.Album{
				{ID: 1, Name: "旅行", Description: "春游", CreatedBy: userID, PhotoCount: 2},
				{ID: 2, Name: "收藏", Description: "混合媒体", CreatedBy: userID, PhotoCount: 5},
			}, nil
		}
		return []*storage.Album{}, nil
	}, listShares: func(userID int64) ([]*storage.ShareLink, error) {
		return []*storage.ShareLink{
			{ID: 1, Token: "token-1", Type: storage.ShareTypePhoto, TargetID: 11, CreatedBy: userID, CreatedAt: time.Now()},
			{ID: 2, Token: "token-2", Type: storage.ShareTypeAlbum, TargetID: 8, CreatedBy: userID, CreatedAt: time.Now()},
		}, nil
	}, removePhoto: func(albumID int64, photoID int64, userID int64) error {
		return nil
	}, updateAlbum: func(id int64, name, description string, coverPhotoID *int64, userID int64) (*storage.Album, error) {
		return &storage.Album{ID: id, Name: name, Description: description, CreatedBy: userID, CoverPhotoID: coverPhotoID, CreatedAt: time.Now()}, nil
	}, register: func(input service.RegisterUploadedVideoInput) (*storage.Photo, error) {
		return &storage.Photo{
			ID:           99,
			UUID:         input.UUID,
			OriginalName: input.OriginalName,
			MediaKind:    storage.MediaKindVideo,
			MimeType:     input.MimeType,
			Size:         input.Size,
			Width:        input.Meta.Width,
			Height:       input.Meta.Height,
			DurationMS:   input.Meta.DurationMS,
			UploadedBy:   input.UploadedBy,
			TakenAt:      input.TakenAt,
			UploadedAt:   time.Now(),
		}, nil
	}, upload: func(input service.UploadInput) (*service.UploadResult, error) {
		return &service.UploadResult{Photo: &storage.Photo{ID: 101, UUID: "image-uuid", OriginalName: input.OriginalName, MediaKind: storage.MediaKindImage, MimeType: "image/jpeg", UploadedBy: input.UploadedBy}}, nil
	}, deletePhoto: func(id int64, userID int64) error {
		return nil
	}, emptyTrash: func(userID int64) error {
		return nil
	}, getDownloadEntries: func(photoIDs []int64, userID int64) ([]service.DownloadEntry, error) {
		entries := make([]service.DownloadEntry, 0, len(photoIDs))
		for _, id := range photoIDs {
			entries = append(entries, service.DownloadEntry{
				FileName: fmt.Sprintf("media-%d.mp4", id),
				Path:     mediaFilePath(&storage.Photo{UUID: fmt.Sprintf("media-%d", id)}),
				MimeType: "video/mp4",
			})
		}
		return entries, nil
	}, getAlbumMedia: func(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error) {
		return &storage.PhotoPage{Photos: []*storage.Photo{
			{
				ID:           11,
				UUID:         "album-image-1",
				OriginalName: "album.jpg",
				MediaKind:    storage.MediaKindImage,
				MimeType:     "image/jpeg",
				UploadedBy:   params.UserID,
			},
			{
				ID:           12,
				UUID:         "album-video-1",
				OriginalName: "album.mp4",
				MediaKind:    storage.MediaKindVideo,
				MimeType:     "video/mp4",
				UploadedBy:   params.UserID,
			},
		}, NextCursor: "", HasMore: false}, nil
	}, getPhoto: func(id int64, userID int64) (*storage.Photo, error) {
		return &storage.Photo{
			ID:           id,
			UUID:         fmt.Sprintf("media-%d", id),
			OriginalName: "demo.mp4",
			MediaKind:    storage.MediaKindVideo,
			MimeType:     "video/mp4",
			DurationMS:   12000,
			UploadedBy:   userID,
		}, nil
	}, getByUUID: func(uuid string, userID int64) (*storage.Photo, error) {
		return &storage.Photo{
			ID:           99,
			UUID:         uuid,
			OriginalName: "demo.mp4",
			MediaKind:    storage.MediaKindVideo,
			MimeType:     "video/mp4",
			UploadedBy:   userID,
		}, nil
	}, getTrash: func(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
		return &storage.PhotoPage{Photos: []*storage.Photo{
			{
				ID:           3,
				UUID:         "trash-1",
				OriginalName: "trash.mp4",
				MediaKind:    storage.MediaKindVideo,
				MimeType:     "video/mp4",
				UploadedBy:   params.UserID,
			},
		}, NextCursor: "", HasMore: false}, nil
	}, getFavorites: func(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
		return &storage.PhotoPage{Photos: []*storage.Photo{
			{
				ID:           7,
				UUID:         "favorite-1",
				OriginalName: "favorite.jpg",
				MediaKind:    storage.MediaKindImage,
				MimeType:     "image/jpeg",
				UploadedBy:   params.UserID,
				IsFavorite:   true,
			},
		}, NextCursor: "", HasMore: false}, nil
	}, getTimeline: func(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
		return &storage.PhotoPage{Photos: []*storage.Photo{
			{
				ID:           1,
				UUID:         "image-1",
				OriginalName: "demo.jpg",
				MediaKind:    storage.MediaKindImage,
				MimeType:     "image/jpeg",
				UploadedBy:   params.UserID,
			},
			{
				ID:           2,
				UUID:         "video-1",
				OriginalName: "demo.mp4",
				MediaKind:    storage.MediaKindVideo,
				MimeType:     "video/mp4",
				DurationMS:   12000,
				UploadedBy:   params.UserID,
			},
		}, NextCursor: "", HasMore: false}, nil
	}, searchMedia: func(params storage.SearchPhotosParams) (*storage.PhotoPage, error) {
		return &storage.PhotoPage{Photos: []*storage.Photo{
			{
				ID:           4,
				UUID:         "search-1",
				OriginalName: "search-result.jpg",
				MediaKind:    storage.MediaKindImage,
				MimeType:     "image/jpeg",
				UploadedBy:   params.UserID,
			},
		}, NextCursor: "", HasMore: false, Total: 1}, nil
	}, mediaPath: mediaFilePath, permanentlyDeletePhoto: func(id int64, userID int64) error {
		return nil
	}, playWithSystemPlayer: func(id int64, userID int64) error {
		return nil
	}, posterPath: posterFilePath, revealInFinder: func(id int64, userID int64) error {
		return nil
	}, restorePhoto: func(id int64, userID int64) error {
		return nil
	}, setPhotoFavorite: func(id int64, userID int64, favorite bool, superFavorite bool) error {
		return nil
	}, thumbnailPath: thumbnailFilePath}
}

func testConfig() *config.Config {
	return &config.Config{
		StoragePath: tTempStoragePath,
		AppDataDir:  tTempStoragePath,
		JWTSecret:   "test-secret",
		Users: []config.User{
			{Username: "alice", PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2", Role: config.UserRoleAdmin},
		},
	}
}

func visitorConfig() *config.Config {
	return &config.Config{
		StoragePath: tTempStoragePath,
		AppDataDir:  tTempStoragePath,
		JWTSecret:   "test-secret",
		Users: []config.User{
			{Username: "alice", PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2", Role: config.UserRoleVisitor},
		},
	}
}

const tTempStoragePath = "."

func testToken(t *testing.T, secret, username string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}
	return signed
}

func uploadRequest(t *testing.T, path string, filename string, payload []byte, withFile bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if withFile {
		part, err := writer.CreateFormFile("media", filename)
		if err != nil {
			t.Fatalf("创建表单文件失败: %v", err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatalf("写入表单文件失败: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 multipart writer 失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func mp4Sample() []byte {
	return append([]byte{
		0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70,
		0x69, 0x73, 0x6f, 0x6d, 0x00, 0x00, 0x02, 0x00,
		0x69, 0x73, 0x6f, 0x6d, 0x69, 0x73, 0x6f, 0x32,
	}, bytes.Repeat([]byte{0x00}, 1024)...)
}

func pngSample(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 0x5d, G: 0x8f, B: 0xff, A: 0xff})
	img.Set(1, 0, color.RGBA{R: 0xff, G: 0xc8, B: 0x4d, A: 0xff})
	img.Set(0, 1, color.RGBA{R: 0x3d, G: 0xd4, B: 0x99, A: 0xff})
	img.Set(1, 1, color.RGBA{R: 0xff, G: 0x7b, B: 0x92, A: 0xff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成 PNG 样本失败: %v", err)
	}
	return buf.Bytes()
}

func TestNewRouter_MediaPlaceholder(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var page struct {
		Photos []struct {
			ID        int64  `json:"id"`
			MediaKind string `json:"media_kind"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("解析媒体列表响应失败: %v", err)
	}
	if len(page.Photos) != 2 {
		t.Fatalf("期望 2 条媒体，得到 %d", len(page.Photos))
	}
	if page.Photos[1].MediaKind != storage.MediaKindVideo {
		t.Fatalf("期望第二条为视频，得到 %s", page.Photos[1].MediaKind)
	}
}

func TestSettingsEndpoint_ReturnsConfig(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	cfg.UseSystemPlayer = true
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var resp struct {
		Role            string   `json:"role"`
		CanWrite        bool     `json:"can_write"`
		Port            int      `json:"port"`
		UseSystemPlayer bool     `json:"use_system_player"`
		Users           []string `json:"users"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析设置响应失败: %v", err)
	}
	if resp.Port != 8080 {
		t.Fatalf("期望端口为 8080，得到 %d", resp.Port)
	}
	if resp.Role != config.UserRoleAdmin {
		t.Fatalf("期望 role=admin，得到 %q", resp.Role)
	}
	if !resp.CanWrite {
		t.Fatalf("期望 admin can_write=true")
	}
	if !resp.UseSystemPlayer {
		t.Fatalf("期望返回 use_system_player=true")
	}
	if len(resp.Users) != 1 || resp.Users[0] != "alice" {
		t.Fatalf("期望返回用户 alice，得到 %+v", resp.Users)
	}
}

func TestSettingsEndpoint_VisitorSeesSharedLibraries(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "alice", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "bob", PasswordHash: "hash", Role: config.UserRoleVisitor, AllowedLibraryIDs: []string{"lib_a"}, DefaultLibraryID: "lib_a"},
	}
	cfg.ActiveProfile = "bob"
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a"},
	}

	adminProfile := &config.Profile{
		ActiveLibraryID: "lib_a",
		StoragePath:     "/libraries/a",
		ThumbnailDir:    filepath.Join(cfg.AppDataDir, "thumbs"),
		TrashDir:        filepath.Join(cfg.AppDataDir, "trash"),
		Preferences: config.Preferences{
			Theme:           "light",
			GridSize:        180,
			GridGap:         2,
			ThumbRadius:     2,
			SlideshowMode:   "random",
			PlayerKeymap:    "default",
			LightboxZoom:    100,
			SidebarAutoHide: true,
		},
	}
	if err := config.SaveProfile(cfg, "alice", adminProfile); err != nil {
		t.Fatalf("保存管理员 profile 失败: %v", err)
	}
	visitorProfile := config.NewUserProfileTemplate(cfg)
	visitorProfile.Preferences.Theme = "dark"
	if err := config.SaveProfile(cfg, "bob", visitorProfile); err != nil {
		t.Fatalf("保存访客 profile 失败: %v", err)
	}

	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "bob")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Role        string            `json:"role"`
		CanWrite    bool              `json:"can_write"`
		StoragePath string            `json:"storage_path"`
		Libraries   []libraryResponse `json:"libraries"`
		Theme       string            `json:"theme"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析访客设置响应失败: %v", err)
	}
	if resp.Role != config.UserRoleVisitor {
		t.Fatalf("期望 role=visitor，得到 %q", resp.Role)
	}
	if resp.CanWrite {
		t.Fatalf("期望访客 can_write=false")
	}
	if resp.StoragePath != "/libraries/a" {
		t.Fatalf("期望访客看到管理员资源库路径，得到 %q", resp.StoragePath)
	}
	if len(resp.Libraries) != 1 || resp.Libraries[0].ID != "lib_a" {
		t.Fatalf("期望访客看到共享资源库，得到 %+v", resp.Libraries)
	}
	if resp.Theme != "dark" {
		t.Fatalf("期望保留访客自己的主题，得到 %q", resp.Theme)
	}
}

func TestSettingsEndpoint_VisitorDefaultsToPrimaryLibrary(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "alice", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "bob", PasswordHash: "hash", Role: config.UserRoleVisitor},
	}
	cfg.ActiveProfile = "bob"
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a"},
	}
	if err := config.SaveProfile(cfg, "alice", config.NewUserProfileTemplate(cfg)); err != nil {
		t.Fatalf("保存管理员 profile 失败: %v", err)
	}
	visitorProfile := config.NewUserProfileTemplate(cfg)
	visitorProfile.Preferences.Theme = "dark"
	if err := config.SaveProfile(cfg, "bob", visitorProfile); err != nil {
		t.Fatalf("保存访客 profile 失败: %v", err)
	}

	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "bob")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	var resp struct {
		StoragePath string            `json:"storage_path"`
		Libraries   []libraryResponse `json:"libraries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析访客设置响应失败: %v", err)
	}
	if resp.StoragePath != "/libraries/a" {
		t.Fatalf("期望访客默认落到主要资源库，得到 %q", resp.StoragePath)
	}
	if len(resp.Libraries) != 1 || resp.Libraries[0].ID != "lib_a" {
		t.Fatalf("期望访客默认看到主要资源库，得到 %+v", resp.Libraries)
	}
}

func TestTimelineEndpoint_VisitorUsesSharedLibraryOwner(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "alice", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "bob", PasswordHash: "hash", Role: config.UserRoleVisitor, AllowedLibraryIDs: []string{"lib_a"}, DefaultLibraryID: "lib_a"},
	}
	cfg.ActiveProfile = "bob"
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a"},
	}
	adminProfile := &config.Profile{
		ActiveLibraryID: "lib_a",
		StoragePath:     "/libraries/a",
		ThumbnailDir:    filepath.Join(cfg.AppDataDir, "thumbs"),
		TrashDir:        filepath.Join(cfg.AppDataDir, "trash"),
	}
	if err := config.SaveProfile(cfg, "alice", adminProfile); err != nil {
		t.Fatalf("保存管理员 profile 失败: %v", err)
	}
	if err := config.SaveProfile(cfg, "bob", config.NewUserProfileTemplate(cfg)); err != nil {
		t.Fatalf("保存访客 profile 失败: %v", err)
	}

	var gotUserID int64
	router := NewRouter(cfg, stubRegistrar{
		getTimeline: func(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
			gotUserID = params.UserID
			return &storage.PhotoPage{}, nil
		},
		getPhoto:           okRegistrar().getPhoto,
		getByUUID:          okRegistrar().getByUUID,
		mediaPath:          okRegistrar().mediaPath,
		posterPath:         okRegistrar().posterPath,
		register:           okRegistrar().register,
		deletePhoto:        okRegistrar().deletePhoto,
		emptyTrash:         okRegistrar().emptyTrash,
		getDownloadEntries: okRegistrar().getDownloadEntries,
		getAlbumMedia:      okRegistrar().getAlbumMedia,
		getTrash:           okRegistrar().getTrash,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "bob")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if gotUserID != 1 {
		t.Fatalf("期望访客读取共享资源库 user_id=1，得到 %d", gotUserID)
	}
}

func TestTimelineEndpoint_AdminUsesLibraryOwnerWhenSwitchedUserHasNoMedia(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "alice", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "administrator", PasswordHash: "hash", Role: config.UserRoleAdmin},
	}
	cfg.ActiveProfile = "administrator"
	cfg.ActiveLibraryID = "lib_a"
	cfg.StoragePath = "/libraries/a"
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a", OwnerUsername: "alice"},
	}
	if err := config.SaveProfile(cfg, "alice", &config.Profile{
		ActiveLibraryID: "lib_a",
		StoragePath:     "/libraries/a",
		ThumbnailDir:    filepath.Join(cfg.AppDataDir, "thumbs"),
		TrashDir:        filepath.Join(cfg.AppDataDir, "trash"),
	}); err != nil {
		t.Fatalf("保存 alice profile 失败: %v", err)
	}
	if err := config.SaveProfile(cfg, "administrator", &config.Profile{
		ActiveLibraryID: "lib_a",
		StoragePath:     "/libraries/a",
		ThumbnailDir:    filepath.Join(cfg.AppDataDir, "thumbs"),
		TrashDir:        filepath.Join(cfg.AppDataDir, "trash"),
	}); err != nil {
		t.Fatalf("保存 administrator profile 失败: %v", err)
	}

	var gotUserID int64
	router := NewRouter(cfg, stubRegistrar{
		getTimeline: func(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
			gotUserID = params.UserID
			return &storage.PhotoPage{}, nil
		},
		getPhoto:           okRegistrar().getPhoto,
		getByUUID:          okRegistrar().getByUUID,
		mediaPath:          okRegistrar().mediaPath,
		posterPath:         okRegistrar().posterPath,
		register:           okRegistrar().register,
		deletePhoto:        okRegistrar().deletePhoto,
		emptyTrash:         okRegistrar().emptyTrash,
		getDownloadEntries: okRegistrar().getDownloadEntries,
		getAlbumMedia:      okRegistrar().getAlbumMedia,
		getTrash:           okRegistrar().getTrash,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "administrator")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if gotUserID != 1 {
		t.Fatalf("期望新管理员读取资源库 owner user_id=1，得到 %d", gotUserID)
	}
}

func TestVideoPlaybackPreference_UsesLibraryOwnerRecordForAllUsers(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "alice", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "bob", PasswordHash: "hash", Role: config.UserRoleVisitor, AllowedLibraryIDs: []string{"lib_a"}, DefaultLibraryID: "lib_a"},
	}
	cfg.ActiveProfile = "bob"
	cfg.ActiveLibraryID = "lib_a"
	cfg.StoragePath = "/libraries/a"
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a", OwnerUsername: "alice"},
	}
	if err := config.SaveProfile(cfg, "alice", &config.Profile{
		ActiveLibraryID: "lib_a",
		StoragePath:     "/libraries/a",
		ThumbnailDir:    filepath.Join(cfg.AppDataDir, "thumbs"),
		TrashDir:        filepath.Join(cfg.AppDataDir, "trash"),
	}); err != nil {
		t.Fatalf("保存 alice profile 失败: %v", err)
	}
	if err := config.SaveProfile(cfg, "bob", config.NewUserProfileTemplate(cfg)); err != nil {
		t.Fatalf("保存 bob profile 失败: %v", err)
	}

	savedByUser := make(map[int64]*storage.VideoPlaybackPreference)
	router := NewRouter(cfg, stubRegistrar{
		getVideoPlaybackPreference: func(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error) {
			if pref, ok := savedByUser[userID]; ok {
				copyPref := *pref
				copyPref.Bookmarks = append([]storage.VideoPlaybackBookmark(nil), pref.Bookmarks...)
				return &copyPref, nil
			}
			if userID == 1 {
				return &storage.VideoPlaybackPreference{
					PhotoID:    photoID,
					Volume:     0.7,
					Muted:      true,
					ResumeTime: 33000,
					Bookmarks:  []storage.VideoPlaybackBookmark{{Slot: 1, Time: 12000, Name: "A"}},
				}, nil
			}
			if userID == 2 {
				return &storage.VideoPlaybackPreference{
					PhotoID:    photoID,
					Volume:     0.45,
					Muted:      false,
					ResumeTime: 99000,
				}, nil
			}
			return nil, nil
		},
		saveVideoPlaybackPreference: func(photoID int64, userID int64, volume float64, muted bool, resumeTime int64, bookmarks []storage.VideoPlaybackBookmark) (*storage.VideoPlaybackPreference, error) {
			pref := &storage.VideoPlaybackPreference{
				PhotoID:    photoID,
				Volume:     volume,
				Muted:      muted,
				ResumeTime: resumeTime,
				Bookmarks:  append([]storage.VideoPlaybackBookmark(nil), bookmarks...),
			}
			savedByUser[userID] = pref
			return pref, nil
		},
	})

	getReq := httptest.NewRequest(http.MethodGet, "/api/media/9/playback", nil)
	getReq.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "bob")})
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", getResp.Code, getResp.Body.String())
	}
	var prefResp storage.VideoPlaybackPreference
	if err := json.Unmarshal(getResp.Body.Bytes(), &prefResp); err != nil {
		t.Fatalf("解析播放偏好失败: %v", err)
	}
	if prefResp.ResumeTime != 33000 || prefResp.Volume != 0.7 || !prefResp.Muted {
		t.Fatalf("期望 visitor 读取资源库 owner 的媒体播放偏好，得到 %+v", prefResp)
	}
	if len(prefResp.Bookmarks) != 1 || prefResp.Bookmarks[0].Time != 12000 {
		t.Fatalf("期望 visitor 读取共享书签，得到 %+v", prefResp.Bookmarks)
	}

	muteReq := httptest.NewRequest(http.MethodPut, "/api/media/9/playback", strings.NewReader(`{"muted":false,"resume_time":45000}`))
	muteReq.Header.Set("Content-Type", "application/json")
	muteReq.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "bob")})
	muteResp := httptest.NewRecorder()
	router.ServeHTTP(muteResp, muteReq)
	if muteResp.Code != http.StatusOK {
		t.Fatalf("期望 visitor 可保存媒体播放状态 200，得到 %d，响应: %s", muteResp.Code, muteResp.Body.String())
	}
	if ownerPref := savedByUser[1]; ownerPref == nil || ownerPref.Muted || ownerPref.ResumeTime != 45000 {
		t.Fatalf("期望 visitor 的静音/续播写入资源库 owner 记录，得到 %+v", ownerPref)
	}
	if visitorPref := savedByUser[2]; visitorPref != nil {
		t.Fatalf("visitor 不应写入自己的播放记录，得到 %+v", visitorPref)
	}

	saveReq := httptest.NewRequest(http.MethodPut, "/api/media/9/playback", strings.NewReader(`{"bookmarks":[{"slot":2,"time":45000,"name":"Shared"}]}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveReq.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "bob")})
	saveResp := httptest.NewRecorder()
	router.ServeHTTP(saveResp, saveReq)
	if saveResp.Code != http.StatusForbidden {
		t.Fatalf("期望 403，得到 %d，响应: %s", saveResp.Code, saveResp.Body.String())
	}
	if ownerPref := savedByUser[1]; ownerPref == nil || len(ownerPref.Bookmarks) != 1 || ownerPref.Bookmarks[0].Slot != 1 {
		t.Fatalf("visitor 不应修改共享书签，得到 %+v", ownerPref)
	}
	if visitorPref := savedByUser[2]; visitorPref != nil {
		t.Fatalf("visitor 不应写入自己的书签，得到 %+v", visitorPref)
	}
}

func TestSettingsUpdate_VisitorOnlySavesOwnPreferences(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "alice", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "bob", PasswordHash: "hash", Role: config.UserRoleVisitor},
	}
	cfg.ActiveProfile = "alice"
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a"},
	}

	adminProfile := &config.Profile{
		ActiveLibraryID: "lib_a",
		StoragePath:     "/libraries/a",
		ThumbnailDir:    filepath.Join(cfg.AppDataDir, "thumbs"),
		TrashDir:        filepath.Join(cfg.AppDataDir, "trash"),
		Preferences: config.Preferences{
			Theme:           "light",
			GridSize:        180,
			GridGap:         2,
			ThumbRadius:     2,
			SlideshowMode:   "random",
			LightboxZoom:    100,
			PlayerKeymap:    "default",
			SidebarAutoHide: true,
		},
	}
	if err := config.SaveProfile(cfg, "alice", adminProfile); err != nil {
		t.Fatalf("保存管理员 profile 失败: %v", err)
	}
	visitorProfile := config.NewUserProfileTemplate(cfg)
	if err := config.SaveProfile(cfg, "bob", visitorProfile); err != nil {
		t.Fatalf("保存访客 profile 失败: %v", err)
	}

	router := NewRouter(cfg, okRegistrar())
	body := `{
		"port":9090,
		"active_library_id":"lib_a",
		"storage_path":"/libraries/a",
		"libraries":[{"id":"lib_a","name":"A","path":"/libraries/a"}],
		"thumbnail_dir":"` + filepath.ToSlash(filepath.Join(cfg.AppDataDir, "thumbs")) + `",
		"thumbnail_size":512,
		"trash_dir":"` + filepath.ToSlash(filepath.Join(cfg.AppDataDir, "trash")) + `",
		"use_system_player":true,
		"theme":"dark",
		"grid_size":220,
		"grid_gap":4,
		"thumb_radius":6,
		"sidebar_auto_hide":true,
		"slideshow_mode":"sequential",
		"slideshow_loop":false,
		"slideshow_interval":8000,
		"lightbox_zoom":110,
		"video_autoplay_next":true,
		"video_section_min_minutes":12,
		"experimental_prefetch_neighbors":true,
		"experimental_restore_last_view":true,
		"continue_last_video_position":true,
		"warm_enabled":false,
		"low_resource_mode":true,
		"player_keymap":"custom"
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "bob")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	var resp struct {
		RequiresRestart bool `json:"requires_restart"`
		Data            struct {
			Theme string `json:"theme"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析访客设置更新响应失败: %v", err)
	}
	if resp.RequiresRestart {
		t.Fatalf("期望访客偏好保存不触发重启")
	}
	if resp.Data.Theme != "dark" {
		t.Fatalf("期望返回访客自己的主题 dark，得到 %q", resp.Data.Theme)
	}
	if cfg.ActiveProfile != "alice" {
		t.Fatalf("期望访客保存后不修改全局 active_profile，得到 %q", cfg.ActiveProfile)
	}
	savedVisitor, err := config.LoadProfile(cfg, "bob")
	if err != nil {
		t.Fatalf("读取访客 profile 失败: %v", err)
	}
	if savedVisitor.Preferences.Theme != "dark" {
		t.Fatalf("期望访客主题已保存，得到 %q", savedVisitor.Preferences.Theme)
	}
	if len(savedVisitor.Libraries) != 0 || strings.TrimSpace(savedVisitor.StoragePath) != "" {
		t.Fatalf("期望访客 profile 不写入共享资源库，得到 %+v", savedVisitor)
	}
}

func TestSettingsUpdate_ReturnsRequiresRestartWhenSwitchingLibrary(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	cfg.ThumbnailDir = filepath.Join(cfg.AppDataDir, "thumbnails")
	cfg.TrashDir = filepath.Join(cfg.AppDataDir, "Trash")
	libA := filepath.Join(cfg.AppDataDir, "library-a")
	libB := filepath.Join(cfg.AppDataDir, "library-b")
	cfg.ActiveLibraryID = "lib_a"
	cfg.StoragePath = libA
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: libA},
		{ID: "lib_b", Name: "B", Path: libB},
	}
	router := NewRouter(cfg, okRegistrar())
	body := fmt.Sprintf(`{
		"port":8080,
		"active_library_id":"lib_b",
		"storage_path":%q,
		"libraries":[
			{"id":"lib_a","name":"A","path":%q},
			{"id":"lib_b","name":"B","path":%q}
		],
		"thumbnail_dir":%q,
		"thumbnail_size":512,
		"trash_dir":%q,
		"use_system_player":false,
		"theme":"light",
		"grid_size":180,
		"grid_gap":2,
		"thumb_radius":2,
		"slideshow_mode":"random",
		"slideshow_loop":true,
		"slideshow_interval":5000,
		"lightbox_zoom":100,
		"video_section_min_minutes":10,
		"experimental_prefetch_neighbors":true,
		"continue_last_video_position":true,
		"warm_enabled":true
	}`, libB, libA, libB, cfg.ThumbnailDir, cfg.TrashDir)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	var resp struct {
		RequiresRestart bool `json:"requires_restart"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析设置更新响应失败: %v", err)
	}
	if !resp.RequiresRestart {
		t.Fatalf("期望切换资源库时返回 requires_restart=true，得到 %s", w.Body.String())
	}
}

func TestSettingsUpdate_PreservesLibraryOwnerUsername(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	cfg.ActiveProfile = "Administrator"
	cfg.ActiveLibraryID = "lib_a"
	cfg.StoragePath = "/libraries/a"
	cfg.ThumbnailDir = filepath.Join(cfg.AppDataDir, "thumbnails")
	cfg.TrashDir = filepath.Join(cfg.AppDataDir, "Trash")
	cfg.Users = []config.User{
		{Username: "admin", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "Administrator", PasswordHash: "hash", Role: config.UserRoleAdmin},
	}
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a", OwnerUsername: "admin"},
	}
	profile := config.NewUserProfileTemplate(cfg)
	profile.ActiveLibraryID = "lib_a"
	profile.StoragePath = "/libraries/a"
	if err := config.SaveProfile(cfg, "Administrator", profile); err != nil {
		t.Fatalf("保存管理员 profile 失败: %v", err)
	}

	router := NewRouter(cfg, okRegistrar())
	body := fmt.Sprintf(`{
		"port":8080,
		"active_library_id":"lib_a",
		"storage_path":%q,
		"libraries":[{"id":"lib_a","name":"A","path":%q}],
		"thumbnail_dir":%q,
		"thumbnail_size":512,
		"trash_dir":%q,
		"use_system_player":false,
		"theme":"light",
		"grid_size":180,
		"grid_gap":2,
		"thumb_radius":2,
		"slideshow_mode":"random",
		"slideshow_loop":true,
		"slideshow_interval":5000,
		"lightbox_zoom":100,
		"video_section_min_minutes":10,
		"experimental_prefetch_neighbors":true,
		"continue_last_video_position":true,
		"warm_enabled":true
	}`, "/libraries/a", "/libraries/a", cfg.ThumbnailDir, cfg.TrashDir)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "Administrator")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if got := cfg.Libraries[0].OwnerUsername; got != "admin" {
		t.Fatalf("期望设置保存后保留原 owner=admin，得到 %q", got)
	}
}

func TestSettingsUpdate_LibraryAccessChangesAreIgnoredBySettingsAPI(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	cfg.ActiveProfile = "alice"
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: "/libraries/a"},
		{ID: "lib_b", Name: "B", Path: "/libraries/b"},
	}
	cfg.Users = []config.User{
		{Username: "alice", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "bob", PasswordHash: "hash", Role: config.UserRoleVisitor, AllowedLibraryIDs: []string{"lib_a"}, DefaultLibraryID: "lib_a"},
	}
	adminProfile := config.NewUserProfileTemplate(cfg)
	adminProfile.ActiveLibraryID = "lib_a"
	adminProfile.StoragePath = "/libraries/a"
	if err := config.SaveProfile(cfg, "alice", adminProfile); err != nil {
		t.Fatalf("保存管理员 profile 失败: %v", err)
	}
	visitorProfile := config.NewUserProfileTemplate(cfg)
	visitorProfile.ActiveLibraryID = "lib_a"
	visitorProfile.StoragePath = "/libraries/a"
	if err := config.SaveProfile(cfg, "bob", visitorProfile); err != nil {
		t.Fatalf("保存访客 profile 失败: %v", err)
	}

	router := NewRouter(cfg, okRegistrar())
	body := `{
		"port":8080,
		"active_library_id":"lib_a",
		"storage_path":"/libraries/a",
		"libraries":[
			{"id":"lib_a","name":"A","path":"/libraries/a"},
			{"id":"lib_b","name":"B","path":"/libraries/b"}
		],
		"thumbnail_dir":"` + filepath.ToSlash(filepath.Join(cfg.AppDataDir, "thumbs")) + `",
		"thumbnail_size":512,
		"trash_dir":"` + filepath.ToSlash(filepath.Join(cfg.AppDataDir, "trash")) + `",
		"use_system_player":false,
		"theme":"light",
		"grid_size":180,
		"grid_gap":2,
		"thumb_radius":2,
		"sidebar_auto_hide":true,
		"slideshow_mode":"random",
		"slideshow_loop":true,
		"slideshow_interval":5000,
		"lightbox_zoom":100,
		"video_section_min_minutes":10,
		"experimental_prefetch_neighbors":true,
		"continue_last_video_position":true,
		"library_access":[
			{"username":"alice","allowed_library_ids":["lib_a","lib_b"],"default_library_id":"lib_b","current_library_id":"lib_a"},
			{"username":"bob","allowed_library_ids":["lib_b"],"default_library_id":"lib_b","current_library_id":"lib_b"}
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	adminUser, _ := config.FindUser(cfg, "alice")
	if adminUser == nil {
		t.Fatal("期望管理员用户仍存在")
	}
	if !slices.Equal(adminUser.AllowedLibraryIDs, []string{"lib_a", "lib_b"}) {
		t.Fatalf("期望管理员固定可访问全部资源库，得到 %+v", adminUser.AllowedLibraryIDs)
	}
	visitorUser, _ := config.FindUser(cfg, "bob")
	if visitorUser == nil {
		t.Fatal("期望访客用户仍存在")
	}
	if !slices.Equal(visitorUser.AllowedLibraryIDs, []string{"lib_a"}) {
		t.Fatalf("设置页不应再改动访客资源库授权，得到 %+v", visitorUser.AllowedLibraryIDs)
	}
	if visitorUser.DefaultLibraryID != "lib_a" {
		t.Fatalf("设置页不应再改动访客默认资源库，得到 %q", visitorUser.DefaultLibraryID)
	}
	savedVisitor, err := config.LoadProfile(cfg, "bob")
	if err != nil {
		t.Fatalf("读取访客 profile 失败: %v", err)
	}
	if savedVisitor.ActiveLibraryID != "lib_a" || savedVisitor.StoragePath != "/libraries/a" {
		t.Fatalf("设置页不应再改动访客当前资源库，得到 id=%q path=%q", savedVisitor.ActiveLibraryID, savedVisitor.StoragePath)
	}
}

func TestServeLibraryLogo_RegeneratesFromLibraryThumbnails(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	cfg.ThumbnailDir = filepath.Join(cfg.AppDataDir, "thumbnails")
	cfg.TrashDir = filepath.Join(cfg.AppDataDir, "Trash")
	libA := filepath.Join(cfg.AppDataDir, "library-a")
	cfg.ActiveLibraryID = "lib_a"
	cfg.StoragePath = libA
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: libA},
	}
	thumbPath := filepath.Join(cfg.ThumbnailDir, "lib_a", "ab", "cd", "sample.png")
	if err := os.MkdirAll(filepath.Dir(thumbPath), 0755); err != nil {
		t.Fatalf("创建缩略图目录失败: %v", err)
	}
	if err := os.WriteFile(thumbPath, pngSample(t), 0644); err != nil {
		t.Fatalf("写入缩略图样本失败: %v", err)
	}
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/settings/libraries/0/logo", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(cfg.Libraries[0].LogoAsset) == "" {
		t.Fatalf("期望自动写入资源库头像")
	}
}

func TestPlayMediaWithSystemPlayerRoute(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		playWithSystemPlayer: func(id int64, userID int64) error {
			called = true
			if id != 12 {
				t.Fatalf("期望媒体 ID 为 12，得到 %d", id)
			}
			return nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/12/play", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatalf("期望命中系统播放器处理函数")
	}
}

func TestRefreshVideoThumbnailsRoute(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		startVideoThumbnailRefresh: func(userID int64) (service.VideoThumbnailRefreshStatus, error) {
			called = true
			return service.VideoThumbnailRefreshStatus{
				Status:    "running",
				Total:     2,
				Refreshed: 1,
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/video-thumbnails/refresh", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("期望命中刷新视频缩略图任务处理函数")
	}
	if !strings.Contains(w.Body.String(), `"status":"running"`) {
		t.Fatalf("期望返回任务状态，得到 %s", w.Body.String())
	}
}

func TestMediaList_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestGetMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestGetMedia_InvalidID(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/abc", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestGetMedia_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/2", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var photo struct {
		ID        int64  `json:"id"`
		MediaKind string `json:"media_kind"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &photo); err != nil {
		t.Fatalf("解析媒体详情响应失败: %v", err)
	}
	if photo.ID != 2 || photo.MediaKind != storage.MediaKindVideo {
		t.Fatalf("返回媒体详情不正确: %+v", photo)
	}
}

func TestGetMedia_NotFound(t *testing.T) {
	registrar := okRegistrar()
	registrar.getPhoto = func(id int64, userID int64) (*storage.Photo, error) {
		return nil, nil
	}
	router := NewRouter(testConfig(), registrar)
	req := httptest.NewRequest(http.MethodGet, "/api/media/99", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", w.Code)
	}
}

func TestDeleteMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodDelete, "/api/media/1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestDeleteMedia_InvalidID(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodDelete, "/api/media/abc", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestDeleteMedia_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		register:    okRegistrar().register,
		deletePhoto: func(id int64, userID int64) error { called = true; return nil },
		getPhoto:    okRegistrar().getPhoto,
		getByUUID:   okRegistrar().getByUUID,
		getTimeline: okRegistrar().getTimeline,
		mediaPath:   okRegistrar().mediaPath,
		posterPath:  okRegistrar().posterPath,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/3", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用删除逻辑")
	}
	var resp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析删除响应失败: %v", err)
	}
	if resp.Message != "已移入回收站" {
		t.Fatalf("删除响应不正确: %+v", resp)
	}
}

func TestDeleteMedia_ReturnsServiceError(t *testing.T) {
	router := NewRouter(testConfig(), stubRegistrar{
		register:    okRegistrar().register,
		deletePhoto: func(id int64, userID int64) error { return fmt.Errorf("照片/视频不存在") },
		getPhoto:    okRegistrar().getPhoto,
		getByUUID:   okRegistrar().getByUUID,
		getTimeline: okRegistrar().getTimeline,
		mediaPath:   okRegistrar().mediaPath,
		posterPath:  okRegistrar().posterPath,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/99", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestListAlbumMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestGetAlbumDetail_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/1/detail", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestGetAlbumDetail_InvalidAlbumID(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/abc/detail", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestGetAlbumDetail_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/5/detail", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var album struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		PhotoCount  int    `json:"photo_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &album); err != nil {
		t.Fatalf("解析相册详情响应失败: %v", err)
	}
	if album.ID != 5 || album.Name != "旅行" || album.PhotoCount != 2 {
		t.Fatalf("相册详情响应不正确: %+v", album)
	}
}

func TestListAlbumsMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestListAlbumsMedia_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var albums []struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		PhotoCount int    `json:"photo_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &albums); err != nil {
		t.Fatalf("解析相册列表响应失败: %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("期望 2 个相册，得到 %d", len(albums))
	}
	if albums[0].ID != 1 || albums[1].PhotoCount != 5 {
		t.Fatalf("相册列表响应不正确: %+v", albums)
	}
}

func TestListMediaAlbums_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/media/11/albums", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	NewRouter(testConfig(), okRegistrar()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var albums []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &albums); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if len(albums) != 2 || albums[0].Name != "旅行" {
		t.Fatalf("媒体所在相册响应不正确: %+v", albums)
	}
}

func TestCreateAlbumMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/media/albums", strings.NewReader(`{"name":"旅行"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestCreateAlbumMedia_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto: func(albumID int64, photoID int64, userID int64) error { return nil },
		createAlbum: func(name, description string, userID int64) (*storage.Album, error) {
			called = true
			if name != "旅行" || description != "相册描述" {
				return nil, fmt.Errorf("unexpected payload: %s / %s", name, description)
			}
			return &storage.Album{ID: 8, Name: name, Description: description, CreatedBy: userID, CreatedAt: time.Now()}, nil
		},
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		listAlbums:              okRegistrar().listAlbums,
		removePhoto:             okRegistrar().removePhoto,
		register:                okRegistrar().register,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID:               okRegistrar().getByUUID,
		getTrash:                okRegistrar().getTrash,
		getTimeline:             okRegistrar().getTimeline,
		mediaPath:               okRegistrar().mediaPath,
		permanentlyDeletePhoto:  okRegistrar().permanentlyDeletePhoto,
		posterPath:              okRegistrar().posterPath,
		restorePhoto:            okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/albums", strings.NewReader(`{"name":"旅行","description":"相册描述"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("期望 201，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用新建相册逻辑")
	}
	var album struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &album); err != nil {
		t.Fatalf("解析新建相册响应失败: %v", err)
	}
	if album.ID != 8 || album.Name != "旅行" {
		t.Fatalf("新建相册响应不正确: %+v", album)
	}
}

func TestUpdateAlbumMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPut, "/api/media/albums/8", strings.NewReader(`{"name":"旅行 2"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestUpdateAlbumMedia_Success(t *testing.T) {
	called := false
	var gotCoverID *int64
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto:                func(albumID int64, photoID int64, userID int64) error { return nil },
		createAlbum:             okRegistrar().createAlbum,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		listAlbums:              okRegistrar().listAlbums,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum: func(id int64, name, description string, coverPhotoID *int64, userID int64) (*storage.Album, error) {
			called = true
			gotCoverID = coverPhotoID
			return &storage.Album{ID: id, Name: name, Description: description, CreatedBy: userID, CoverPhotoID: coverPhotoID, CreatedAt: time.Now()}, nil
		},
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getAlbumMedia:          okRegistrar().getAlbumMedia,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/media/albums/8", strings.NewReader(`{"name":"旅行 2","description":"更新描述","cover_photo_id":12}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用更新相册逻辑")
	}
	if gotCoverID == nil || *gotCoverID != 12 {
		t.Fatalf("封面参数不正确: %+v", gotCoverID)
	}
	var album struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &album); err != nil {
		t.Fatalf("解析更新相册响应失败: %v", err)
	}
	if album.ID != 8 || album.Name != "旅行 2" {
		t.Fatalf("更新相册响应不正确: %+v", album)
	}
}

func TestDeleteAlbumMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodDelete, "/api/media/albums/8", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestDeleteAlbumMedia_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto:                func(albumID int64, photoID int64, userID int64) error { return nil },
		createAlbum:             okRegistrar().createAlbum,
		deleteAlbum:             func(id int64, userID int64) error { called = true; return nil },
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		listAlbums:              okRegistrar().listAlbums,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID:               okRegistrar().getByUUID,
		getTrash:                okRegistrar().getTrash,
		getTimeline:             okRegistrar().getTimeline,
		mediaPath:               okRegistrar().mediaPath,
		permanentlyDeletePhoto:  okRegistrar().permanentlyDeletePhoto,
		posterPath:              okRegistrar().posterPath,
		restorePhoto:            okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/albums/8", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用删除相册逻辑")
	}
	var resp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析删除相册响应失败: %v", err)
	}
	if resp.Message != "相册已删除" {
		t.Fatalf("删除相册响应不正确: %+v", resp)
	}
}

func TestListSharesMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/shares", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestListSharesMedia_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/shares", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var links []struct {
		ID    int64  `json:"id"`
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatalf("解析分享列表响应失败: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("期望 2 条分享，得到 %d", len(links))
	}
	if links[0].Token != "token-1" || links[1].Type != storage.ShareTypeAlbum {
		t.Fatalf("分享列表响应不正确: %+v", links)
	}
}

func TestCreateShareMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/media/shares", strings.NewReader(`{"type":"photo","target_id":11}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestCreateShareMedia_Success(t *testing.T) {
	called := false
	var gotInput service.CreateShareInput
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto:    okRegistrar().addPhoto,
		createAlbum: okRegistrar().createAlbum,
		createShare: func(input service.CreateShareInput) (*storage.ShareLink, error) {
			called = true
			gotInput = input
			return &storage.ShareLink{ID: 3, Token: "new-token", Type: input.Type, TargetID: input.TargetID, CreatedBy: input.UserID, ExpiresAt: input.ExpiresAt, CreatedAt: time.Now()}, nil
		},
		deleteAlbum:             okRegistrar().deleteAlbum,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		listAlbums:              okRegistrar().listAlbums,
		listShares:              okRegistrar().listShares,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID:               okRegistrar().getByUUID,
		getTrash:                okRegistrar().getTrash,
		getTimeline:             okRegistrar().getTimeline,
		mediaPath:               okRegistrar().mediaPath,
		permanentlyDeletePhoto:  okRegistrar().permanentlyDeletePhoto,
		posterPath:              okRegistrar().posterPath,
		restorePhoto:            okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/shares", strings.NewReader(`{"type":"photo","target_id":11}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("期望 201，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用创建分享逻辑")
	}
	if gotInput.Type != storage.ShareTypePhoto || gotInput.TargetID != 11 || gotInput.ExpiresAt != nil {
		t.Fatalf("创建分享参数不正确: %+v", gotInput)
	}
	var link struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &link); err != nil {
		t.Fatalf("解析创建分享响应失败: %v", err)
	}
	if link.ID != 3 || link.Token != "new-token" {
		t.Fatalf("创建分享响应不正确: %+v", link)
	}
}

func TestDeleteShareMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodDelete, "/api/media/shares/3", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestDeleteShareMedia_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             func(id int64, userID int64) error { called = true; return nil },
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		listAlbums:              okRegistrar().listAlbums,
		listShares:              okRegistrar().listShares,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID:               okRegistrar().getByUUID,
		getTrash:                okRegistrar().getTrash,
		getTimeline:             okRegistrar().getTimeline,
		mediaPath:               okRegistrar().mediaPath,
		permanentlyDeletePhoto:  okRegistrar().permanentlyDeletePhoto,
		posterPath:              okRegistrar().posterPath,
		restorePhoto:            okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/shares/3", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用删除分享逻辑")
	}
	var resp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析删除分享响应失败: %v", err)
	}
	if resp.Message != "分享链接已删除" {
		t.Fatalf("删除分享响应不正确: %+v", resp)
	}
}

func TestGetShareByToken_NotFound(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/s/missing", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", w.Code)
	}
}

func TestGetShareByToken_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/s/token-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var link struct {
		Token           string `json:"token"`
		Type            string `json:"type"`
		TargetMediaKind string `json:"target_media_kind"`
		TargetMimeType  string `json:"target_mime_type"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &link); err != nil {
		t.Fatalf("解析分享详情响应失败: %v", err)
	}
	if link.Token != "token-1" || link.Type != storage.ShareTypePhoto {
		t.Fatalf("分享详情响应不正确: %+v", link)
	}
	if link.TargetMediaKind != storage.MediaKindVideo || link.TargetMimeType != "video/mp4" {
		t.Fatalf("分享详情响应不正确: %+v", link)
	}
}

func TestGetSharedAlbumMedia_NotFound(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/s/missing/photos", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", w.Code)
	}
}

func TestGetSharedAlbumMedia_BadType(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/s/token-1/photos", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestGetSharedAlbumMedia_Success(t *testing.T) {
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken: func(token string) (*storage.ShareLink, error) {
			return &storage.ShareLink{ID: 9, Token: token, Type: storage.ShareTypeAlbum, TargetID: 8, CreatedBy: 1, CreatedAt: time.Now()}, nil
		},
		listAlbums:             okRegistrar().listAlbums,
		listShares:             okRegistrar().listShares,
		removePhoto:            okRegistrar().removePhoto,
		updateAlbum:            okRegistrar().updateAlbum,
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getAlbumMedia:          okRegistrar().getAlbumMedia,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/s/album-token/photos", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var page struct {
		Photos []struct {
			ID        int64  `json:"id"`
			MediaKind string `json:"media_kind"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("解析分享相册内容响应失败: %v", err)
	}
	if len(page.Photos) != 2 || page.Photos[1].MediaKind != storage.MediaKindVideo {
		t.Fatalf("分享相册内容响应不正确: %+v", page)
	}
}

func TestServePhotoFile_Success(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	mediaFile := filepath.Join(storageDir, "shared.jpg")
	if err := os.WriteFile(mediaFile, []byte("image-data"), 0644); err != nil {
		t.Fatalf("创建测试原图失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken:         okRegistrar().getShareByToken,
		listAlbums:              okRegistrar().listAlbums,
		listShares:              okRegistrar().listShares,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID: func(uuid string, userID int64) (*storage.Photo, error) {
			return &storage.Photo{ID: 21, UUID: uuid, OriginalName: "shared.jpg", MediaKind: storage.MediaKindImage, MimeType: "image/jpeg", UploadedBy: userID}, nil
		},
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              func(photo *storage.Photo) string { return mediaFile },
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
		thumbnailPath:          okRegistrar().thumbnailPath,
	})
	req := httptest.NewRequest(http.MethodGet, "/media/photos/shared.jpg", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
}

func TestServeThumbnailFile_Success(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	thumbDir := filepath.Join(storageDir, ".thumbnails")
	thumbFile := imgpkg.ThumbnailShardPath(thumbDir, "shared")
	if err := os.MkdirAll(filepath.Dir(thumbFile), 0755); err != nil {
		t.Fatalf("创建缩略图目录失败: %v", err)
	}
	if err := os.WriteFile(thumbFile, []byte("thumb-data"), 0644); err != nil {
		t.Fatalf("创建测试缩略图失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken:         okRegistrar().getShareByToken,
		listAlbums:              okRegistrar().listAlbums,
		listShares:              okRegistrar().listShares,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID: func(uuid string, userID int64) (*storage.Photo, error) {
			return &storage.Photo{ID: 21, UUID: uuid, OriginalName: "shared.jpg", MediaKind: storage.MediaKindImage, MimeType: "image/jpeg", UploadedBy: userID}, nil
		},
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
		thumbnailPath:          func(photo *storage.Photo) string { return imgpkg.ThumbnailShardPath(thumbDir, photo.UUID) },
	})
	req := httptest.NewRequest(http.MethodGet, "/media/thumbnails/shared.jpg", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if cacheControl := w.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "immutable") {
		t.Fatalf("期望缩略图使用强缓存，得到 %q", cacheControl)
	}
}

func TestListMedia_IncludesThumbnailMetadata(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	thumbDir := filepath.Join(storageDir, ".thumbnails")
	thumbFile := imgpkg.ThumbnailShardPath(thumbDir, "photo-1")
	if err := os.MkdirAll(filepath.Dir(thumbFile), 0755); err != nil {
		t.Fatalf("创建缩略图目录失败: %v", err)
	}
	if err := os.WriteFile(thumbFile, []byte("thumb-data"), 0644); err != nil {
		t.Fatalf("创建测试缩略图失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		register: okRegistrar().register,
		getTimeline: func(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
			return &storage.PhotoPage{Photos: []*storage.Photo{{
				ID:           11,
				UUID:         "photo-1",
				OriginalName: "photo.jpg",
				MediaKind:    storage.MediaKindImage,
				MimeType:     "image/jpeg",
				Width:        1200,
				Height:       800,
				UploadedBy:   params.UserID,
			}}}, nil
		},
		thumbnailPath: func(photo *storage.Photo) string { return imgpkg.ThumbnailShardPath(thumbDir, photo.UUID) },
	})
	req := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Photos []struct {
			UUID           string `json:"uuid"`
			Width          int    `json:"width"`
			Height         int    `json:"height"`
			ThumbnailURL   string `json:"thumbnail_url"`
			ThumbnailReady bool   `json:"thumbnail_ready"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if len(resp.Photos) != 1 {
		t.Fatalf("期望 1 张媒体，得到 %d", len(resp.Photos))
	}
	photo := resp.Photos[0]
	if photo.ThumbnailURL != "/media/thumbnails/photo-1" || !photo.ThumbnailReady {
		t.Fatalf("缩略图字段不正确: %+v", photo)
	}
	if photo.Width != 1200 || photo.Height != 800 {
		t.Fatalf("尺寸字段不正确: %+v", photo)
	}
}

func TestServeSharedMediaFile_AlbumSuccess(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	mediaFile := filepath.Join(storageDir, "album.mp4")
	if err := os.WriteFile(mediaFile, []byte("album-media"), 0644); err != nil {
		t.Fatalf("创建测试分享媒体失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken: func(token string) (*storage.ShareLink, error) {
			return &storage.ShareLink{ID: 9, Token: token, Type: storage.ShareTypeAlbum, TargetID: 8, CreatedBy: 1, CreatedAt: time.Now()}, nil
		},
		listAlbums:         okRegistrar().listAlbums,
		listShares:         okRegistrar().listShares,
		removePhoto:        okRegistrar().removePhoto,
		updateAlbum:        okRegistrar().updateAlbum,
		register:           okRegistrar().register,
		deletePhoto:        okRegistrar().deletePhoto,
		emptyTrash:         okRegistrar().emptyTrash,
		getDownloadEntries: okRegistrar().getDownloadEntries,
		getAlbumMedia: func(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error) {
			return &storage.PhotoPage{Photos: []*storage.Photo{{ID: 31, UUID: "album-media", OriginalName: "album.mp4", MediaKind: storage.MediaKindVideo, MimeType: "video/mp4", UploadedBy: params.UserID}}}, nil
		},
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              func(photo *storage.Photo) string { return mediaFile },
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
		thumbnailPath:          okRegistrar().thumbnailPath,
	})
	req := httptest.NewRequest(http.MethodGet, "/media/s/album-token/album-media.mp4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
}

func TestDownloadSharedMedia_Success(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	mediaFile := filepath.Join(storageDir, "shared.mp4")
	if err := os.WriteFile(mediaFile, []byte("shared-media"), 0644); err != nil {
		t.Fatalf("创建测试分享下载文件失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken: func(token string) (*storage.ShareLink, error) {
			return &storage.ShareLink{ID: 4, Token: token, Type: storage.ShareTypePhoto, TargetID: 11, CreatedBy: 1, CreatedAt: time.Now()}, nil
		},
		listAlbums:         okRegistrar().listAlbums,
		listShares:         okRegistrar().listShares,
		removePhoto:        okRegistrar().removePhoto,
		updateAlbum:        okRegistrar().updateAlbum,
		register:           okRegistrar().register,
		deletePhoto:        okRegistrar().deletePhoto,
		emptyTrash:         okRegistrar().emptyTrash,
		getDownloadEntries: okRegistrar().getDownloadEntries,
		getAlbumMedia:      okRegistrar().getAlbumMedia,
		getPhoto: func(id int64, userID int64) (*storage.Photo, error) {
			return &storage.Photo{ID: id, UUID: "shared", OriginalName: "shared.mp4", MediaKind: storage.MediaKindVideo, MimeType: "video/mp4", UploadedBy: userID}, nil
		},
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              func(photo *storage.Photo) string { return mediaFile },
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
		thumbnailPath:          okRegistrar().thumbnailPath,
	})
	req := httptest.NewRequest(http.MethodGet, "/s/photo-token/download", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "shared.mp4") {
		t.Fatalf("下载头不正确: %s", w.Header().Get("Content-Disposition"))
	}
}

func TestHandleSharePage_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/s/token-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "分享 - EchoGallery") {
		t.Fatalf("分享页内容不正确")
	}
	if !strings.Contains(w.Body.String(), `/static/lightbox.js`) || !strings.Contains(w.Body.String(), `createShareController`) {
		t.Fatalf("分享页未接入共享灯箱")
	}
}

func TestDownloadAlbumMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/1/download", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestDownloadAlbumMedia_Success(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	mediaFile := filepath.Join(storageDir, "album-video.mp4")
	if err := os.WriteFile(mediaFile, []byte("album-video"), 0644); err != nil {
		t.Fatalf("创建测试相册媒体文件失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		addPhoto: func(albumID int64, photoID int64, userID int64) error { return nil },
		getAlbum: okRegistrar().getAlbum,
		getAlbumDownloadEntries: func(albumID int64, userID int64) (string, []service.DownloadEntry, error) {
			return "旅行/2026", []service.DownloadEntry{{FileName: "clip.mp4", Path: mediaFile, MimeType: "video/mp4"}}, nil
		},
		listAlbums:             okRegistrar().listAlbums,
		removePhoto:            okRegistrar().removePhoto,
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getAlbumMedia:          okRegistrar().getAlbumMedia,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/5/download", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/zip") {
		t.Fatalf("Content-Type 不正确: %s", ct)
	}
	disposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "旅行-2026.zip") {
		t.Fatalf("下载头不正确: %s", disposition)
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("解析 zip 失败: %v", err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "clip.mp4" {
		t.Fatalf("zip 条目不正确: %+v", zr.File)
	}
}

func TestListAlbumMedia_InvalidAlbumID(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/abc", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestListAlbumMedia_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/albums/5", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var page struct {
		Photos []struct {
			ID        int64  `json:"id"`
			MediaKind string `json:"media_kind"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("解析相册媒体响应失败: %v", err)
	}
	if len(page.Photos) != 2 {
		t.Fatalf("期望 2 条相册媒体，得到 %d", len(page.Photos))
	}
	if page.Photos[1].MediaKind != storage.MediaKindVideo {
		t.Fatalf("期望第二条为视频，得到 %s", page.Photos[1].MediaKind)
	}
}

func TestAddMediaToAlbum_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/media/albums/1", strings.NewReader(`{"media_id":9}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestAddMediaToAlbum_UsesMediaID(t *testing.T) {
	called := false
	var gotAlbumID, gotMediaID int64
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto: func(albumID int64, photoID int64, userID int64) error {
			called = true
			gotAlbumID = albumID
			gotMediaID = photoID
			return nil
		},
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getAlbumMedia:          okRegistrar().getAlbumMedia,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/albums/3", strings.NewReader(`{"media_id":9}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用添加到相册逻辑")
	}
	if gotAlbumID != 3 || gotMediaID != 9 {
		t.Fatalf("透传参数不正确: album=%d media=%d", gotAlbumID, gotMediaID)
	}
}

func TestAddMediaToAlbum_AcceptsLegacyPhotoID(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto: func(albumID int64, photoID int64, userID int64) error {
			called = true
			if albumID != 3 || photoID != 7 {
				return fmt.Errorf("unexpected params: album=%d photo=%d", albumID, photoID)
			}
			return nil
		},
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getAlbumMedia:          okRegistrar().getAlbumMedia,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/albums/3", strings.NewReader(`{"photo_id":7}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用兼容 photo_id 的添加逻辑")
	}
}

func TestRemoveMediaFromAlbum_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodDelete, "/api/media/albums/1/9", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestRemoveMediaFromAlbum_InvalidMediaID(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodDelete, "/api/media/albums/1/abc", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestRemoveMediaFromAlbum_Success(t *testing.T) {
	called := false
	var gotAlbumID, gotMediaID int64
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto: func(albumID int64, photoID int64, userID int64) error {
			return nil
		},
		removePhoto: func(albumID int64, photoID int64, userID int64) error {
			called = true
			gotAlbumID = albumID
			gotMediaID = photoID
			return nil
		},
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getAlbumMedia:          okRegistrar().getAlbumMedia,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/albums/3/9", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用从相册移除逻辑")
	}
	if gotAlbumID != 3 || gotMediaID != 9 {
		t.Fatalf("透传参数不正确: album=%d media=%d", gotAlbumID, gotMediaID)
	}
}

func TestListTrashMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/trash", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestListTrashMedia_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/trash", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var page struct {
		Photos []struct {
			ID int64 `json:"id"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("解析回收站响应失败: %v", err)
	}
	if len(page.Photos) != 1 || page.Photos[0].ID != 3 {
		t.Fatalf("回收站响应不正确: %+v", page)
	}
}

func TestRestoreMedia_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           func(id int64, userID int64) error { called = true; return nil },
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/6/restore", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用恢复逻辑")
	}
}

func TestHardDeleteMedia_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             okRegistrar().emptyTrash,
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: func(id int64, userID int64) error { called = true; return nil },
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/trash/6", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用永久删除逻辑")
	}
}

func TestEmptyTrashMedia_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		register:               okRegistrar().register,
		deletePhoto:            okRegistrar().deletePhoto,
		emptyTrash:             func(userID int64) error { called = true; return nil },
		getDownloadEntries:     okRegistrar().getDownloadEntries,
		getPhoto:               okRegistrar().getPhoto,
		getByUUID:              okRegistrar().getByUUID,
		getTrash:               okRegistrar().getTrash,
		getTimeline:            okRegistrar().getTimeline,
		mediaPath:              okRegistrar().mediaPath,
		permanentlyDeletePhoto: okRegistrar().permanentlyDeletePhoto,
		posterPath:             okRegistrar().posterPath,
		restorePhoto:           okRegistrar().restorePhoto,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/trash", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用清空回收站逻辑")
	}
}

func TestDownloadMedia_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/1/download", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestDownloadMedia_InvalidID(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/media/abc/download", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestDownloadMedia_Success(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	mediaFile := filepath.Join(storageDir, "media-2.mp4")
	if err := os.WriteFile(mediaFile, []byte("video-download"), 0644); err != nil {
		t.Fatalf("创建测试媒体文件失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		register: okRegistrar().register,
		getPhoto: func(id int64, userID int64) (*storage.Photo, error) {
			return &storage.Photo{ID: id, UUID: "media-2", OriginalName: "demo video.mp4", MediaKind: storage.MediaKindVideo, MimeType: "video/mp4", UploadedBy: userID}, nil
		},
		getByUUID:   okRegistrar().getByUUID,
		getTimeline: okRegistrar().getTimeline,
		mediaPath:   func(photo *storage.Photo) string { return mediaFile },
		posterPath:  okRegistrar().posterPath,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/media/2/download", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if body := w.Body.String(); body != "video-download" {
		t.Fatalf("下载内容不正确: %s", body)
	}
	disposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment;") || !strings.Contains(disposition, "demo video.mp4") {
		t.Fatalf("下载头不正确: %s", disposition)
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Fatalf("Content-Type 不正确: %s", ct)
	}
}

func TestRevealMediaInFinder_Success(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken:         okRegistrar().getShareByToken,
		listAlbums:              okRegistrar().listAlbums,
		listShares:              okRegistrar().listShares,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		upload:                  okRegistrar().upload,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID:               okRegistrar().getByUUID,
		getTrash:                okRegistrar().getTrash,
		getTimeline:             okRegistrar().getTimeline,
		mediaPath:               okRegistrar().mediaPath,
		permanentlyDeletePhoto:  okRegistrar().permanentlyDeletePhoto,
		posterPath:              okRegistrar().posterPath,
		revealInFinder:          func(id int64, userID int64) error { called = id == 2 && userID == 1; return nil },
		restorePhoto:            okRegistrar().restorePhoto,
		thumbnailPath:           okRegistrar().thumbnailPath,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/2/reveal", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用 revealInFinder")
	}
}

func TestRevealMediaInFinder_UsesLibraryOwnerUserID(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "owner", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "administrator", PasswordHash: "hash", Role: config.UserRoleAdmin},
	}
	cfg.ActiveProfile = "administrator"
	cfg.ActiveLibraryID = "lib_owner"
	cfg.StoragePath = "/tmp/owner-library"
	cfg.Libraries = []config.Library{
		{ID: "lib_owner", Name: "Owner Library", Path: "/tmp/owner-library", OwnerUsername: "owner"},
	}
	if err := config.SaveProfile(cfg, "owner", &config.Profile{
		ActiveLibraryID: "lib_owner",
		StoragePath:     "/tmp/owner-library",
	}); err != nil {
		t.Fatalf("保存 owner profile 失败: %v", err)
	}
	if err := config.SaveProfile(cfg, "administrator", &config.Profile{
		ActiveLibraryID: "lib_owner",
		StoragePath:     "/tmp/owner-library",
	}); err != nil {
		t.Fatalf("保存 administrator profile 失败: %v", err)
	}

	var gotUserID int64
	router := NewRouter(cfg, stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken:         okRegistrar().getShareByToken,
		listAlbums:              okRegistrar().listAlbums,
		listShares:              okRegistrar().listShares,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		upload:                  okRegistrar().upload,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID:               okRegistrar().getByUUID,
		getTrash:                okRegistrar().getTrash,
		getTimeline:             okRegistrar().getTimeline,
		mediaPath:               okRegistrar().mediaPath,
		permanentlyDeletePhoto:  okRegistrar().permanentlyDeletePhoto,
		posterPath:              okRegistrar().posterPath,
		revealInFinder: func(id int64, userID int64) error {
			gotUserID = userID
			return nil
		},
		restorePhoto:  okRegistrar().restorePhoto,
		thumbnailPath: okRegistrar().thumbnailPath,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/2/reveal", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "administrator")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if gotUserID != 1 {
		t.Fatalf("期望 reveal 使用资源库 owner user_id=1，得到 %d", gotUserID)
	}
}

func TestRevealAlbumInFinder_UsesLibraryOwnerUserID(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Users = []config.User{
		{Username: "owner", PasswordHash: "hash", Role: config.UserRoleAdmin},
		{Username: "administrator", PasswordHash: "hash", Role: config.UserRoleAdmin},
	}
	cfg.ActiveProfile = "administrator"
	cfg.ActiveLibraryID = "lib_owner"
	cfg.StoragePath = "/tmp/owner-library"
	cfg.Libraries = []config.Library{
		{ID: "lib_owner", Name: "Owner Library", Path: "/tmp/owner-library", OwnerUsername: "owner"},
	}
	if err := config.SaveProfile(cfg, "owner", &config.Profile{
		ActiveLibraryID: "lib_owner",
		StoragePath:     "/tmp/owner-library",
	}); err != nil {
		t.Fatalf("保存 owner profile 失败: %v", err)
	}
	if err := config.SaveProfile(cfg, "administrator", &config.Profile{
		ActiveLibraryID: "lib_owner",
		StoragePath:     "/tmp/owner-library",
	}); err != nil {
		t.Fatalf("保存 administrator profile 失败: %v", err)
	}

	var gotUserID int64
	router := NewRouter(cfg, stubRegistrar{
		addPhoto:                okRegistrar().addPhoto,
		createAlbum:             okRegistrar().createAlbum,
		createShare:             okRegistrar().createShare,
		deleteAlbum:             okRegistrar().deleteAlbum,
		deleteShare:             okRegistrar().deleteShare,
		getAlbum:                okRegistrar().getAlbum,
		getAlbumDownloadEntries: okRegistrar().getAlbumDownloadEntries,
		getShareByToken:         okRegistrar().getShareByToken,
		listAlbums:              okRegistrar().listAlbums,
		listShares:              okRegistrar().listShares,
		removePhoto:             okRegistrar().removePhoto,
		updateAlbum:             okRegistrar().updateAlbum,
		register:                okRegistrar().register,
		upload:                  okRegistrar().upload,
		deletePhoto:             okRegistrar().deletePhoto,
		emptyTrash:              okRegistrar().emptyTrash,
		getDownloadEntries:      okRegistrar().getDownloadEntries,
		getAlbumMedia:           okRegistrar().getAlbumMedia,
		getPhoto:                okRegistrar().getPhoto,
		getByUUID:               okRegistrar().getByUUID,
		getTrash:                okRegistrar().getTrash,
		getTimeline:             okRegistrar().getTimeline,
		mediaPath:               okRegistrar().mediaPath,
		permanentlyDeletePhoto:  okRegistrar().permanentlyDeletePhoto,
		posterPath:              okRegistrar().posterPath,
		revealAlbumInFinder: func(id int64, userID int64) error {
			gotUserID = userID
			return nil
		},
		restorePhoto:  okRegistrar().restorePhoto,
		thumbnailPath: okRegistrar().thumbnailPath,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/albums/7/reveal", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "administrator")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if gotUserID != 1 {
		t.Fatalf("期望相册 reveal 使用资源库 owner user_id=1，得到 %d", gotUserID)
	}
}

func TestDownloadMedia_NotFound(t *testing.T) {
	registrar := okRegistrar()
	registrar.getPhoto = func(id int64, userID int64) (*storage.Photo, error) {
		return nil, nil
	}
	router := NewRouter(testConfig(), registrar)
	req := httptest.NewRequest(http.MethodGet, "/api/media/99/download", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", w.Code)
	}
}

func TestDownloadMediaBatch_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/media/download", strings.NewReader(`{"media_ids":[1]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestDownloadMediaBatch_UsesMediaIDsAndReturnsZip(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	firstFile := filepath.Join(storageDir, "media-1.mp4")
	secondFile := filepath.Join(storageDir, "media-2.mp4")
	if err := os.WriteFile(firstFile, []byte("video-one"), 0644); err != nil {
		t.Fatalf("创建测试媒体文件失败: %v", err)
	}
	if err := os.WriteFile(secondFile, []byte("video-two"), 0644); err != nil {
		t.Fatalf("创建测试媒体文件失败: %v", err)
	}

	var gotIDs []int64
	router := NewRouter(cfg, stubRegistrar{
		register:    okRegistrar().register,
		deletePhoto: okRegistrar().deletePhoto,
		getDownloadEntries: func(photoIDs []int64, userID int64) ([]service.DownloadEntry, error) {
			gotIDs = append([]int64(nil), photoIDs...)
			return []service.DownloadEntry{
				{FileName: "clip-a.mp4", Path: firstFile, MimeType: "video/mp4"},
				{FileName: "clip-b.mp4", Path: secondFile, MimeType: "video/mp4"},
			}, nil
		},
		getPhoto:    okRegistrar().getPhoto,
		getByUUID:   okRegistrar().getByUUID,
		getTimeline: okRegistrar().getTimeline,
		mediaPath:   okRegistrar().mediaPath,
		posterPath:  okRegistrar().posterPath,
	})
	body := bytes.NewBufferString(`{"media_ids":[7,9]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/media/download", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if len(gotIDs) != 2 || gotIDs[0] != 7 || gotIDs[1] != 9 {
		t.Fatalf("media_ids 透传不正确: %+v", gotIDs)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/zip") {
		t.Fatalf("Content-Type 不正确: %s", ct)
	}

	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("解析 zip 失败: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("期望 2 个文件，得到 %d", len(zr.File))
	}
	file, err := zr.File[0].Open()
	if err != nil {
		t.Fatalf("打开 zip 条目失败: %v", err)
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatalf("读取 zip 条目失败: %v", err)
	}
	if string(data) != "video-one" {
		t.Fatalf("zip 内容不正确: %s", string(data))
	}
}

func TestDownloadMediaBatch_AcceptsLegacyPhotoIDs(t *testing.T) {
	called := false
	router := NewRouter(testConfig(), stubRegistrar{
		register:    okRegistrar().register,
		deletePhoto: okRegistrar().deletePhoto,
		getDownloadEntries: func(photoIDs []int64, userID int64) ([]service.DownloadEntry, error) {
			called = true
			if len(photoIDs) != 1 || photoIDs[0] != 5 {
				return nil, fmt.Errorf("unexpected ids: %+v", photoIDs)
			}
			return []service.DownloadEntry{}, nil
		},
		getPhoto:    okRegistrar().getPhoto,
		getByUUID:   okRegistrar().getByUUID,
		getTimeline: okRegistrar().getTimeline,
		mediaPath:   okRegistrar().mediaPath,
		posterPath:  okRegistrar().posterPath,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/media/download", strings.NewReader(`{"photo_ids":[5]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !called {
		t.Fatal("应调用批量下载逻辑")
	}
}

func TestNewRouter_UnknownRouteReturns404(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", w.Code)
	}
}

func TestNewRouterWithStatic_ServesLocalStaticFile(t *testing.T) {
	staticFS := fstest.MapFS{"web/static/app.css": {Data: []byte(":root{--bg:#fff;}")}}
	router := NewRouterWithStatic(testConfig(), staticFS, okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "--bg") {
		t.Fatalf("静态文件内容不正确")
	}
}

func TestLogin_Success(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == authCookieName {
			found = true
		}
	}
	if !found {
		t.Fatal("登录后应返回认证 cookie")
	}
}

func TestLogin_UsesPortScopedCookie(t *testing.T) {
	cfg := testConfig()
	cfg.Port = 8081
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	scopedName := authCookieNameForConfig(cfg)
	foundScoped := false
	clearedLegacy := false
	for _, c := range w.Result().Cookies() {
		if c.Name == scopedName && c.Value != "" {
			foundScoped = true
		}
		if c.Name == authCookieName && c.MaxAge < 0 {
			clearedLegacy = true
		}
	}
	if !foundScoped {
		t.Fatalf("登录后应返回端口隔离认证 cookie %q", scopedName)
	}
	if !clearedLegacy {
		t.Fatal("登录后应清理旧的固定名称认证 cookie")
	}
}

func TestLogin_NewPageSessionReplacesOldPage(t *testing.T) {
	cfg := testConfig()
	store, err := pagesession.New(filepath.Join(t.TempDir(), "page-sessions.db"))
	if err != nil {
		t.Fatalf("failed to create page session store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("failed to close page session store: %v", closeErr)
		}
	}()
	router := NewRouterWithOptions(cfg, okRegistrar(), RouterOptions{PageSessions: store})

	loginA := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	loginA.Header.Set("Content-Type", "application/json")
	loginA.Header.Set(pageSessionHeader, "page-a")
	loginAW := httptest.NewRecorder()
	router.ServeHTTP(loginAW, loginA)
	if loginAW.Code != http.StatusOK {
		t.Fatalf("first login expected 200, got %d with body %s", loginAW.Code, loginAW.Body.String())
	}

	loginB := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	loginB.Header.Set("Content-Type", "application/json")
	loginB.Header.Set(pageSessionHeader, "page-b")
	loginBW := httptest.NewRecorder()
	router.ServeHTTP(loginBW, loginB)
	if loginBW.Code != http.StatusOK {
		t.Fatalf("second login expected 200, got %d with body %s", loginBW.Code, loginBW.Body.String())
	}

	oldReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	oldReq.Header.Set(pageSessionHeader, "page-a")
	for _, cookie := range loginAW.Result().Cookies() {
		oldReq.AddCookie(cookie)
	}
	oldW := httptest.NewRecorder()
	router.ServeHTTP(oldW, oldReq)
	if oldW.Code != http.StatusUnauthorized {
		t.Fatalf("old page session expected 401, got %d with body %s", oldW.Code, oldW.Body.String())
	}
	if !strings.Contains(oldW.Body.String(), "duplicate_page_session") {
		t.Fatalf("old page session should be rejected as duplicate, got %s", oldW.Body.String())
	}

	newReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	newReq.Header.Set(pageSessionHeader, "page-b")
	for _, cookie := range loginBW.Result().Cookies() {
		newReq.AddCookie(cookie)
	}
	newW := httptest.NewRecorder()
	router.ServeHTTP(newW, newReq)
	if newW.Code != http.StatusOK {
		t.Fatalf("new page session expected 200, got %d with body %s", newW.Code, newW.Body.String())
	}
}

func TestAuth_PageSessionKeepsDifferentUsersIsolatedWhenCookieChanges(t *testing.T) {
	cfg := testConfig()
	cfg.Users = append(cfg.Users, config.User{
		Username:     "bob",
		PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2",
		Role:         config.UserRoleAdmin,
	})
	store, err := pagesession.New(filepath.Join(t.TempDir(), "page-sessions.db"))
	if err != nil {
		t.Fatalf("failed to create page session store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("failed to close page session store: %v", closeErr)
		}
	}()
	router := NewRouterWithOptions(cfg, okRegistrar(), RouterOptions{PageSessions: store})

	loginAlice := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	loginAlice.Header.Set("Content-Type", "application/json")
	loginAlice.Header.Set(pageSessionHeader, "page-alice")
	loginAliceW := httptest.NewRecorder()
	router.ServeHTTP(loginAliceW, loginAlice)
	if loginAliceW.Code != http.StatusOK {
		t.Fatalf("alice login expected 200, got %d with body %s", loginAliceW.Code, loginAliceW.Body.String())
	}

	loginBob := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"bob","password":"password123"}`))
	loginBob.Header.Set("Content-Type", "application/json")
	loginBob.Header.Set(pageSessionHeader, "page-bob")
	loginBobW := httptest.NewRecorder()
	router.ServeHTTP(loginBobW, loginBob)
	if loginBobW.Code != http.StatusOK {
		t.Fatalf("bob login expected 200, got %d with body %s", loginBobW.Code, loginBobW.Body.String())
	}

	oldAliceReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	oldAliceReq.Header.Set(pageSessionHeader, "page-alice")
	for _, cookie := range loginBobW.Result().Cookies() {
		oldAliceReq.AddCookie(cookie)
	}
	oldAliceW := httptest.NewRecorder()
	router.ServeHTTP(oldAliceW, oldAliceReq)
	if oldAliceW.Code != http.StatusOK {
		t.Fatalf("alice page should survive bob cookie overwrite, got %d with body %s", oldAliceW.Code, oldAliceW.Body.String())
	}
	if !strings.Contains(oldAliceW.Body.String(), `"current_username":"alice"`) {
		t.Fatalf("alice page should keep alice identity, got %s", oldAliceW.Body.String())
	}
}

func TestLogin_RedirectToFreshPageSessionStillAuthenticates(t *testing.T) {
	cfg := testConfig()
	store, err := pagesession.New(filepath.Join(t.TempDir(), "page-sessions.db"))
	if err != nil {
		t.Fatalf("failed to create page session store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("failed to close page session store: %v", closeErr)
		}
	}()
	router := NewRouterWithOptions(cfg, okRegistrar(), RouterOptions{PageSessions: store})

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set(pageSessionHeader, "login-page-session")
	loginW := httptest.NewRecorder()
	router.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("login expected 200, got %d with body %s", loginW.Code, loginW.Body.String())
	}

	releaseReq := httptest.NewRequest(http.MethodPost, "/api/auth/session/release?eg_page_session=login-page-session", nil)
	for _, cookie := range loginW.Result().Cookies() {
		releaseReq.AddCookie(cookie)
	}
	releaseW := httptest.NewRecorder()
	router.ServeHTTP(releaseW, releaseReq)
	if releaseW.Code != http.StatusOK {
		t.Fatalf("release expected 200, got %d with body %s", releaseW.Code, releaseW.Body.String())
	}

	appReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	appReq.Header.Set(pageSessionHeader, "app-page-session")
	for _, cookie := range loginW.Result().Cookies() {
		appReq.AddCookie(cookie)
	}
	appW := httptest.NewRecorder()
	router.ServeHTTP(appW, appReq)
	if appW.Code != http.StatusOK {
		t.Fatalf("fresh app page session should authenticate, got %d with body %s", appW.Code, appW.Body.String())
	}
}

func TestLogin_RedirectUsingSamePageSessionStillAuthenticates(t *testing.T) {
	cfg := testConfig()
	store, err := pagesession.New(filepath.Join(t.TempDir(), "page-sessions.db"))
	if err != nil {
		t.Fatalf("failed to create page session store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("failed to close page session store: %v", closeErr)
		}
	}()
	router := NewRouterWithOptions(cfg, okRegistrar(), RouterOptions{PageSessions: store})

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set(pageSessionHeader, "login-page-session")
	loginW := httptest.NewRecorder()
	router.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("login expected 200, got %d with body %s", loginW.Code, loginW.Body.String())
	}

	appReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	appReq.Header.Set(pageSessionHeader, "login-page-session")
	for _, cookie := range loginW.Result().Cookies() {
		appReq.AddCookie(cookie)
	}
	appW := httptest.NewRecorder()
	router.ServeHTTP(appW, appReq)
	if appW.Code != http.StatusOK {
		t.Fatalf("same page session after login redirect should authenticate, got %d with body %s", appW.Code, appW.Body.String())
	}
}

func TestPageSessionRelease_WithoutAuthCookieStillReleasesLocks(t *testing.T) {
	cfg := testConfig()
	pageStore, err := pagesession.New(filepath.Join(t.TempDir(), "page-sessions.db"))
	if err != nil {
		t.Fatalf("failed to create page session store: %v", err)
	}
	defer func() {
		if closeErr := pageStore.Close(); closeErr != nil {
			t.Fatalf("failed to close page session store: %v", closeErr)
		}
	}()
	lockStore, err := sessionlock.New(filepath.Join(t.TempDir(), "locks.db"))
	if err != nil {
		t.Fatalf("failed to create lock store: %v", err)
	}
	defer func() {
		if closeErr := lockStore.Close(); closeErr != nil {
			t.Fatalf("failed to close lock store: %v", closeErr)
		}
	}()
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
	}
	cfg.ActiveLibraryID = "lib_a"
	cfg.StoragePath = "/tmp/library-a"
	router := NewRouterWithOptions(cfg, okRegistrar(), RouterOptions{PageSessions: pageStore, LockStore: lockStore})

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set(pageSessionHeader, "page-a")
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login expected 200, got %d with body %s", loginResp.Code, loginResp.Body.String())
	}

	settingsReq := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	settingsReq.Header.Set(pageSessionHeader, "page-a")
	for _, cookie := range loginResp.Result().Cookies() {
		settingsReq.AddCookie(cookie)
	}
	settingsResp := httptest.NewRecorder()
	router.ServeHTTP(settingsResp, settingsReq)
	if settingsResp.Code != http.StatusOK {
		t.Fatalf("media expected 200, got %d with body %s", settingsResp.Code, settingsResp.Body.String())
	}

	activeBefore, err := lockStore.ActiveForLibrary("lib_a")
	if err != nil {
		t.Fatalf("failed to inspect active locks before release: %v", err)
	}
	if len(activeBefore) == 0 {
		t.Fatal("expected browse lock to exist before release")
	}

	releaseReq := httptest.NewRequest(http.MethodPost, "/api/auth/session/release?eg_page_session=page-a", nil)
	releaseResp := httptest.NewRecorder()
	router.ServeHTTP(releaseResp, releaseReq)
	if releaseResp.Code != http.StatusOK {
		t.Fatalf("release expected 200, got %d with body %s", releaseResp.Code, releaseResp.Body.String())
	}

	activeAfter, err := lockStore.ActiveForLibrary("lib_a")
	if err != nil {
		t.Fatalf("failed to inspect active locks after release: %v", err)
	}
	if len(activeAfter) != 0 {
		t.Fatalf("expected release to clear browse locks immediately, got %+v", activeAfter)
	}
}

func TestLogin_VisitorDefaultsToPrimaryLibrary(t *testing.T) {
	cfg := visitorConfig()
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
	}
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"active_library_id":"lib_a"`) {
		t.Fatalf("期望登录后落到主要资源库，得到 %s", w.Body.String())
	}
}

func TestLogin_RootAccountMustUseRootConsole(t *testing.T) {
	cfg := &config.Config{
		StoragePath: tTempStoragePath,
		AppDataDir:  tTempStoragePath,
		JWTSecret:   "test-secret",
		Users: []config.User{
			{Username: "root", PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2", Role: config.UserRoleRoot},
		},
	}
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"root","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("期望 403，得到 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "./EchoGallery root") {
		t.Fatalf("期望提示使用 root 控制台，得到 %s", w.Body.String())
	}
}

func TestFavoritesUseCurrentLibraryOwnerForAdminBrowsing(t *testing.T) {
	cfg := testConfig()
	cfg.Users = []config.User{
		{Username: "owner", PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2", Role: config.UserRoleAdmin},
		{Username: "alice", PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2", Role: config.UserRoleAdmin},
	}
	cfg.Libraries = []config.Library{
		{ID: "lib_owner", Name: "Owner Library", Path: "/tmp/owner-library", OwnerUsername: "owner"},
	}
	cfg.ActiveLibraryID = "lib_owner"
	cfg.StoragePath = "/tmp/owner-library"

	called := false
	router := NewRouter(cfg, stubRegistrar{
		register: okRegistrar().register,
		getFavorites: func(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
			called = true
			if params.UserID != 1 {
				return nil, fmt.Errorf("unexpected user id: %d", params.UserID)
			}
			return &storage.PhotoPage{Photos: []*storage.Photo{}, NextCursor: "", HasMore: false, Total: 0}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/media/favorites", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("应调用收藏列表查询")
	}
}

func TestFavoritesUseCurrentLibraryOwnerForAdminMutation(t *testing.T) {
	cfg := testConfig()
	cfg.Users = []config.User{
		{Username: "owner", PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2", Role: config.UserRoleAdmin},
		{Username: "alice", PasswordHash: "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2", Role: config.UserRoleAdmin},
	}
	cfg.Libraries = []config.Library{
		{ID: "lib_owner", Name: "Owner Library", Path: "/tmp/owner-library", OwnerUsername: "owner"},
	}
	cfg.ActiveLibraryID = "lib_owner"
	cfg.StoragePath = "/tmp/owner-library"

	called := false
	router := NewRouter(cfg, stubRegistrar{
		register: okRegistrar().register,
		setPhotoFavorite: func(id int64, userID int64, favorite bool, superFavorite bool) error {
			called = true
			if id != 9 {
				return fmt.Errorf("unexpected photo id: %d", id)
			}
			if userID != 1 {
				return fmt.Errorf("unexpected user id: %d", userID)
			}
			if !favorite || superFavorite {
				return fmt.Errorf("unexpected favorite payload: favorite=%v super=%v", favorite, superFavorite)
			}
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/api/media/9/favorite", strings.NewReader(`{"favorite":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("应调用收藏写入逻辑")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice","password":"wrongpass"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestLogout_ClearsCookie(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
}

func TestLoginPage_Returns200(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
}

func TestRegisterPage_Returns200(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/api/auth/register") {
		t.Fatalf("注册页应包含注册接口")
	}
}

func TestRootConsole_ForcedSessionCanOpenAppAndReadSettings(t *testing.T) {
	cfg := testConfig()
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
		{ID: "lib_b", Name: "资源库 B", Path: "/tmp/library-b"},
	}
	cfg.StoragePath = "/tmp/library-a"
	router := NewRouterWithOptions(cfg, okRegistrar(), RouterOptions{ForcedUsername: rootConsoleUsername})

	pageReq := httptest.NewRequest(http.MethodGet, "/", nil)
	pageResp := httptest.NewRecorder()
	router.ServeHTTP(pageResp, pageReq)
	if pageResp.Code != http.StatusOK {
		t.Fatalf("期望 root 控制台首页返回 200，得到 %d", pageResp.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	var resp struct {
		CurrentUsername string `json:"current_username"`
		Role            string `json:"role"`
		CanRoot         bool   `json:"can_root"`
		CanWrite        bool   `json:"can_write"`
		ActiveLibraryID string `json:"active_library_id"`
		StoragePath     string `json:"storage_path"`
		Libraries       []struct {
			ID string `json:"id"`
		} `json:"libraries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析 settings 响应失败: %v", err)
	}
	if resp.CurrentUsername != "root" || resp.Role != config.UserRoleRoot {
		t.Fatalf("期望 root 会话信息，得到 %+v", resp)
	}
	if !resp.CanRoot || resp.CanWrite {
		t.Fatalf("期望 root 仅拥有 root 能力，得到 can_root=%v can_write=%v", resp.CanRoot, resp.CanWrite)
	}
	if resp.ActiveLibraryID != "" || resp.StoragePath != "" {
		t.Fatalf("root 控制台不应绑定当前资源库，得到 active=%q storage=%q", resp.ActiveLibraryID, resp.StoragePath)
	}
	if len(resp.Libraries) != 2 {
		t.Fatalf("期望 root 可查看全部资源库，得到 %d", len(resp.Libraries))
	}
}

func TestRootConsole_ForcedSessionCanListUsers(t *testing.T) {
	cfg := testConfig()
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
	}
	cfg.Users = append(cfg.Users, config.User{
		Username:          "guest",
		PasswordHash:      "$2a$10$m2CWsTFrqFNGPW/bGg4UluO.WX/e.rgEkX4yxHJI.VABfOyGA8BA2",
		Role:              config.UserRoleVisitor,
		AllowedLibraryIDs: nil,
	})
	router := NewRouterWithOptions(cfg, okRegistrar(), RouterOptions{ForcedUsername: rootConsoleUsername})
	req := httptest.NewRequest(http.MethodGet, "/api/root/users", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Users []struct {
			Username           string `json:"username"`
			Role               string `json:"role"`
			CanLogin           bool   `json:"can_login"`
			LoginBlockedReason string `json:"login_blocked_reason"`
		} `json:"users"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析 root users 响应失败: %v", err)
	}
	if len(resp.Users) != 2 {
		t.Fatalf("期望返回 2 个用户，得到 %d", len(resp.Users))
	}
	if resp.Users[1].Username != "guest" || resp.Users[1].Role != config.UserRoleVisitor {
		t.Fatalf("期望 guest 为 visitor，得到 %+v", resp.Users[1])
	}
	if !resp.Users[1].CanLogin {
		t.Fatalf("默认 visitor 应可登录，得到 %+v", resp.Users[1])
	}
	if resp.Users[1].LoginBlockedReason != "" {
		t.Fatalf("默认 visitor 不应带授权阻塞原因，得到 %+v", resp.Users[1])
	}
}

func TestAppPage_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("期望 303，得到 %d", w.Code)
	}
}

func TestAppPage_WithAuthReturns200(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/albums/1", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, testConfig().JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
}

func TestUploadPlaceholder_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := uploadRequest(t, "/api/media/upload", "demo.mp4", mp4Sample(), true)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestVisitor_CanOnlyUpdateHarmlessSettings(t *testing.T) {
	cfg := visitorConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	cfg.StoragePath = filepath.Join(cfg.AppDataDir, "library-a")
	cfg.Libraries = []config.Library{
		{ID: "lib_a", Name: "A", Path: cfg.StoragePath},
	}
	cfg.ThumbnailDir = filepath.Join(cfg.AppDataDir, "thumbnails")
	cfg.TrashDir = filepath.Join(cfg.AppDataDir, "Trash")
	cfg.Preferences.Theme = "light"
	cfg.Preferences.GridGap = 2
	cfg.Preferences.LowResourceMode = false
	router := NewRouter(cfg, okRegistrar())
	body := fmt.Sprintf(`{"port":9090,"active_library_id":"lib_b","storage_path":%q,"libraries":[{"id":"lib_b","name":"B","path":%q}],"thumbnail_dir":%q,"thumbnail_size":512,"trash_dir":%q,"use_system_player":true,"theme":"dark","grid_size":196,"grid_gap":6,"thumb_radius":8,"sidebar_auto_hide":true,"slideshow_mode":"sequential","slideshow_loop":false,"slideshow_interval":7000,"lightbox_zoom":100,"experimental_autoplay_video":true,"video_autoplay_next":true,"video_section_min_minutes":12,"experimental_prefetch_neighbors":false,"experimental_restore_last_view":true,"continue_last_video_position":false,"warm_enabled":false,"low_resource_mode":true,"player_keymap":"SPACE pause"}`, filepath.Join(cfg.AppDataDir, "library-b"), filepath.Join(cfg.AppDataDir, "library-b"), filepath.Join(cfg.AppDataDir, "other-thumbnails"), filepath.Join(cfg.AppDataDir, "other-trash"))
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d，响应: %s", w.Code, w.Body.String())
	}

	profile, err := config.LoadProfile(cfg, "alice")
	if err != nil {
		t.Fatalf("读取 visitor profile 失败: %v", err)
	}
	if profile.StoragePath != filepath.Join(cfg.AppDataDir, "library-a") {
		t.Fatalf("visitor 不应改动 storage_path，得到 %q", profile.StoragePath)
	}
	if len(profile.Libraries) != 0 {
		t.Fatalf("visitor profile 不应持久化共享 libraries，得到 %+v", profile.Libraries)
	}
	if profile.ThumbnailDir != filepath.Join(cfg.AppDataDir, "thumbnails") {
		t.Fatalf("visitor 不应改动 thumbnail_dir，得到 %q", profile.ThumbnailDir)
	}
	if profile.TrashDir != filepath.Join(cfg.AppDataDir, "Trash") {
		t.Fatalf("visitor 不应改动 trash_dir，得到 %q", profile.TrashDir)
	}
	if profile.Preferences.Theme != "dark" || profile.Preferences.GridGap != 6 {
		t.Fatalf("visitor 应可保存无害偏好，得到 %+v", profile.Preferences)
	}
	if profile.Preferences.LowResourceMode {
		t.Fatalf("visitor 不应改动 low_resource_mode")
	}
	if cfg.Port != 8080 {
		t.Fatalf("visitor 不应改动全局端口，得到 %d", cfg.Port)
	}
}

func TestVisitor_CannotDeleteMedia(t *testing.T) {
	cfg := visitorConfig()
	called := false
	router := NewRouter(cfg, stubRegistrar{
		deletePhoto: func(id int64, userID int64) error {
			called = true
			return nil
		},
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/media/1", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("期望 403，得到 %d", w.Code)
	}
	if called {
		t.Fatal("visitor 不应进入删除处理逻辑")
	}
}

func TestVisitor_CannotStartLibraryBatchBuild(t *testing.T) {
	cfg := visitorConfig()
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/settings/libraries/build-all", strings.NewReader(`{"selection":[0]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("期望 403，得到 %d", w.Code)
	}
}

func TestAdmin_CanReachLibraryBatchBuildRoutes(t *testing.T) {
	cfg := testConfig()
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodPost, "/api/settings/libraries/build-all", strings.NewReader(`{"selection":[0]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Fatalf("管理员不应被批量工作流路由拒绝，得到 %d", w.Code)
	}
}

func TestUploadPlaceholder_RequiresMediaField(t *testing.T) {
	cfg := testConfig()
	router := NewRouter(cfg, okRegistrar())
	req := uploadRequest(t, "/api/media/upload", "", nil, false)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestUploadPlaceholder_RejectsUnsupportedVideoExtension(t *testing.T) {
	cfg := testConfig()
	cfg.StoragePath = t.TempDir()
	cfg.AppDataDir = t.TempDir()
	router := NewRouter(cfg, okRegistrar())
	req := uploadRequest(t, "/api/media/upload", "demo.flv", mp4Sample(), true)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", w.Code)
	}
}

func TestUploadPlaceholder_SavesFinalFileAndRecordAfterValidation(t *testing.T) {
	cfg := testConfig()
	cfg.StoragePath = t.TempDir()
	cfg.AppDataDir = t.TempDir()
	router := NewRouter(cfg, okRegistrar())
	req := uploadRequest(t, "/api/media/upload", "demo.mp4", mp4Sample(), true)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("期望 201，得到 %d", w.Code)
	}

	var resp struct {
		Message     string `json:"message"`
		Filename    string `json:"filename"`
		Path        string `json:"path"`
		PosterPath  string `json:"poster_path"`
		PosterError string `json:"poster_error"`
		ProbeError  string `json:"probe_error"`
		Meta        struct {
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			DurationMS int64  `json:"duration_ms"`
			FormatName string `json:"format_name"`
			CodecName  string `json:"codec_name"`
		} `json:"meta"`
		Photo struct {
			ID         int64  `json:"id"`
			UUID       string `json:"uuid"`
			MediaKind  string `json:"media_kind"`
			DurationMS int64  `json:"duration_ms"`
		} `json:"photo"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Filename != "demo.mp4" {
		t.Fatalf("期望文件名 demo.mp4，得到 %s", resp.Filename)
	}
	if resp.Path == "" {
		t.Fatal("响应中应返回最终文件路径")
	}
	if resp.Meta.FormatName != "mp4" {
		t.Fatalf("返回的元数据不正确: %+v", resp.Meta)
	}
	if resp.Photo.ID != 99 || resp.Photo.MediaKind != storage.MediaKindVideo {
		t.Fatalf("返回的媒体记录不正确: %+v", resp.Photo)
	}
	if !strings.HasPrefix(resp.Path, filepath.Join(cfg.StoragePath, "EchoGallery Uploads")) {
		t.Fatalf("最终文件应放在目标资源库上传目录下，得到 %s", resp.Path)
	}
	if filepath.Base(resp.Path) != resp.Photo.UUID+".mp4" {
		t.Fatalf("最终文件名应与 UUID 对应，得到 %s", filepath.Base(resp.Path))
	}
	data, err := os.ReadFile(resp.Path)
	if err != nil {
		t.Fatalf("读取临时文件失败: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("临时保存的文件内容不能为空")
	}
}

func TestUploadPlaceholder_CleansFileWhenRegisterFails(t *testing.T) {
	cfg := testConfig()
	cfg.StoragePath = t.TempDir()
	cfg.AppDataDir = t.TempDir()
	router := NewRouter(cfg, stubRegistrar{register: func(input service.RegisterUploadedVideoInput) (*storage.Photo, error) {
		return nil, fmt.Errorf("保存视频记录失败")
	}})
	req := uploadRequest(t, "/api/media/upload", "demo.mp4", mp4Sample(), true)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("期望 500，得到 %d", w.Code)
	}
	if err := filepath.WalkDir(cfg.StoragePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".mp4") {
			t.Fatalf("注册失败后不应残留视频文件: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("检查资源库目录失败: %v", err)
	}
}

func TestUploadPlaceholder_AcceptsPhotoFieldAndReturnsImageRecord(t *testing.T) {
	cfg := testConfig()
	cfg.StoragePath = t.TempDir()
	cfg.AppDataDir = t.TempDir()
	router := NewRouter(cfg, okRegistrar())
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("photo", "demo.jpg")
	if err != nil {
		t.Fatalf("创建表单文件失败: %v", err)
	}
	if _, err := part.Write([]byte("fake-jpeg-data")); err != nil {
		t.Fatalf("写入表单文件失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 multipart writer 失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/media/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("期望 201，得到 %d，body=%s", w.Code, w.Body.String())
	}
	var photo struct {
		ID        int64  `json:"id"`
		UUID      string `json:"uuid"`
		MediaKind string `json:"media_kind"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &photo); err != nil {
		t.Fatalf("解析图片上传响应失败: %v", err)
	}
	if photo.ID != 101 || photo.MediaKind != storage.MediaKindImage || photo.UUID != "image-uuid" {
		t.Fatalf("图片上传返回不正确: %+v", photo)
	}
}

func TestServeMediaFile_RequiresAuth(t *testing.T) {
	router := NewRouter(testConfig(), okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/media/files/video-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %d", w.Code)
	}
}

func TestServeMediaFile_Success(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	mediaFile := filepath.Join(storageDir, "video-1.mp4")
	if err := os.WriteFile(mediaFile, []byte("video"), 0644); err != nil {
		t.Fatalf("创建测试视频失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		register: okRegistrar().register,
		getByUUID: func(uuid string, userID int64) (*storage.Photo, error) {
			return &storage.Photo{UUID: uuid, OriginalName: "demo.mp4", MediaKind: storage.MediaKindVideo, MimeType: "video/mp4", UploadedBy: userID}, nil
		},
		mediaPath:  func(photo *storage.Photo) string { return mediaFile },
		posterPath: func(photo *storage.Photo) string { return filepath.Join(storageDir, ".posters", photo.UUID+".webp") },
	})
	req := httptest.NewRequest(http.MethodGet, "/media/files/video-1", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if body := w.Body.String(); body != "video" {
		t.Fatalf("返回内容不正确: %s", body)
	}
}

func TestServePoster_Success(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	posterRoot := filepath.Join(storageDir, ".thumbnails")
	posterFile := imgpkg.ThumbnailShardPath(posterRoot, "video-1")
	if err := os.MkdirAll(filepath.Dir(posterFile), 0755); err != nil {
		t.Fatalf("创建 poster 目录失败: %v", err)
	}
	if err := os.WriteFile(posterFile, []byte("webp"), 0644); err != nil {
		t.Fatalf("创建测试 poster 失败: %v", err)
	}
	router := NewRouter(cfg, stubRegistrar{
		register: okRegistrar().register,
		getByUUID: func(uuid string, userID int64) (*storage.Photo, error) {
			return &storage.Photo{UUID: uuid, OriginalName: "demo.mp4", MediaKind: storage.MediaKindVideo, MimeType: "video/mp4", UploadedBy: userID}, nil
		},
		mediaPath:  func(photo *storage.Photo) string { return filepath.Join(storageDir, photo.UUID+".mp4") },
		posterPath: func(photo *storage.Photo) string { return imgpkg.ThumbnailShardPath(posterRoot, photo.UUID) },
	})
	req := httptest.NewRequest(http.MethodGet, "/media/posters/video-1", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	if body := w.Body.String(); body != "webp" {
		t.Fatalf("返回内容不正确: %s", body)
	}
}

func TestServePoster_Returns404WhenMissing(t *testing.T) {
	cfg := testConfig()
	storageDir := t.TempDir()
	cfg.StoragePath = storageDir
	router := NewRouter(cfg, stubRegistrar{
		register: okRegistrar().register,
		getByUUID: func(uuid string, userID int64) (*storage.Photo, error) {
			return &storage.Photo{UUID: uuid, OriginalName: "demo.mp4", MediaKind: storage.MediaKindVideo, MimeType: "video/mp4", UploadedBy: userID}, nil
		},
		mediaPath: func(photo *storage.Photo) string { return filepath.Join(storageDir, photo.UUID+".mp4") },
		posterPath: func(photo *storage.Photo) string {
			return imgpkg.ThumbnailShardPath(filepath.Join(storageDir, ".thumbnails"), photo.UUID)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/media/posters/video-1", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", w.Code)
	}
}
