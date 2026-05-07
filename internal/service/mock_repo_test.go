package service

import (
	"fmt"
	"strings"
	"time"

	"echogallery/internal/storage"
)

// mockRepo 用于测试的 Repository mock 实现
type mockRepo struct {
	photos                   map[int64]*storage.Photo
	albums                   map[int64]*storage.Album
	albumPhotos              map[int64][]int64 // albumID -> []photoID
	shareLinks               map[int64]*storage.ShareLink
	playbackPrefs            map[string]*storage.VideoPlaybackPreference
	nextID                   int64
	sourceRelPathLookupCount int
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		photos:        make(map[int64]*storage.Photo),
		albums:        make(map[int64]*storage.Album),
		albumPhotos:   make(map[int64][]int64),
		shareLinks:    make(map[int64]*storage.ShareLink),
		playbackPrefs: make(map[string]*storage.VideoPlaybackPreference),
		nextID:        1,
	}
}

func (m *mockRepo) genID() int64 {
	id := m.nextID
	m.nextID++
	return id
}

func (m *mockRepo) SavePhoto(photo *storage.Photo) error {
	photo.ID = m.genID()
	m.photos[photo.ID] = photo
	return nil
}

func (m *mockRepo) GetPhotoByID(id int64, userID int64) (*storage.Photo, error) {
	p, ok := m.photos[id]
	if !ok || p.UploadedBy != userID || p.DeletedAt != nil {
		return nil, nil
	}
	return p, nil
}

func (m *mockRepo) GetPhotoByIDAny(id int64, userID int64) (*storage.Photo, error) {
	p, ok := m.photos[id]
	if !ok || p.UploadedBy != userID {
		return nil, nil
	}
	return p, nil
}

func (m *mockRepo) GetPhotoByUUID(uuid string, userID int64) (*storage.Photo, error) {
	for _, p := range m.photos {
		if p.UUID == uuid && p.UploadedBy == userID && p.DeletedAt == nil {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) GetPhotoByUUIDAny(uuid string, userID int64) (*storage.Photo, error) {
	for _, p := range m.photos {
		if p.UUID == uuid && p.UploadedBy == userID {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) GetPhotoBySourceRelPath(sourceRelPath string, userID int64) (*storage.Photo, error) {
	m.sourceRelPathLookupCount++
	for _, p := range m.photos {
		if p.SourceRelPath == sourceRelPath && p.UploadedBy == userID {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) ListSourceMediaIndex(userID int64) (map[string]storage.SourceMediaInfo, error) {
	index := make(map[string]storage.SourceMediaInfo)
	for _, p := range m.photos {
		if p.UploadedBy == userID && p.SourceRelPath != "" {
			index[p.SourceRelPath] = storage.SourceMediaInfo{
				ID:            p.ID,
				SourceRelPath: p.SourceRelPath,
				Size:          p.Size,
				SourceModUnix: p.SourceModUnix,
			}
		}
	}
	return index, nil
}

func (m *mockRepo) GetVideoPlaybackPreference(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error) {
	pref := m.playbackPrefs[fmt.Sprintf("%d:%d", photoID, userID)]
	if pref == nil {
		return nil, nil
	}
	return pref, nil
}

func (m *mockRepo) UpsertVideoPlaybackPreference(photoID int64, userID int64, volume float64, muted bool) (*storage.VideoPlaybackPreference, error) {
	pref := &storage.VideoPlaybackPreference{
		PhotoID:   photoID,
		Volume:    volume,
		Muted:     muted,
		UpdatedAt: time.Now(),
	}
	m.playbackPrefs[fmt.Sprintf("%d:%d", photoID, userID)] = pref
	return pref, nil
}

func (m *mockRepo) ListPhotos(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	var photos []*storage.Photo
	for _, p := range m.photos {
		if p.UploadedBy == params.UserID && p.DeletedAt == nil {
			photos = append(photos, p)
		}
	}
	return &storage.PhotoPage{Photos: photos}, nil
}

func (m *mockRepo) ListTrashedPhotos(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	var photos []*storage.Photo
	for _, p := range m.photos {
		if p.UploadedBy == params.UserID && p.DeletedAt != nil {
			photos = append(photos, p)
		}
	}
	return &storage.PhotoPage{Photos: photos}, nil
}

func (m *mockRepo) ListFavoritePhotos(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	var photos []*storage.Photo
	for _, p := range m.photos {
		if p.UploadedBy == params.UserID && p.DeletedAt == nil && p.IsFavorite {
			photos = append(photos, p)
		}
	}
	return &storage.PhotoPage{Photos: photos}, nil
}

func (m *mockRepo) SearchPhotos(params storage.SearchPhotosParams) (*storage.PhotoPage, error) {
	var photos []*storage.Photo
	query := strings.ToLower(params.Query)
	for _, p := range m.photos {
		if p.UploadedBy != params.UserID || p.DeletedAt != nil {
			continue
		}
		if strings.Contains(strings.ToLower(p.OriginalName), query) ||
			strings.Contains(strings.ToLower(p.UUID), query) ||
			strings.Contains(strings.ToLower(p.MimeType), query) ||
			strings.Contains(strings.ToLower(string(p.MediaKind)), query) {
			photos = append(photos, p)
		}
	}
	return &storage.PhotoPage{Photos: photos}, nil
}

func (m *mockRepo) ListRandomPhotos(params storage.RandomPhotosParams) (*storage.PhotoPage, error) {
	return m.ListPhotos(storage.ListPhotosParams{UserID: params.UserID, Limit: params.Limit})
}

func (m *mockRepo) SoftDeletePhoto(id int64, userID int64, deletedBy int64) error {
	p, ok := m.photos[id]
	if !ok || p.UploadedBy != userID {
		return fmt.Errorf("图片不存在")
	}
	now := time.Now()
	p.DeletedAt = &now
	p.DeletedBy = &deletedBy
	return nil
}

func (m *mockRepo) RestorePhoto(id int64, userID int64) error {
	p, ok := m.photos[id]
	if !ok || p.UploadedBy != userID {
		return fmt.Errorf("图片不存在")
	}
	p.DeletedAt = nil
	p.DeletedBy = nil
	return nil
}

func (m *mockRepo) HardDeletePhoto(id int64, userID int64) error {
	delete(m.photos, id)
	return nil
}

func (m *mockRepo) HardDeleteTrashedPhotos(userID int64) ([]*storage.Photo, error) {
	var photos []*storage.Photo
	for id, p := range m.photos {
		if p.UploadedBy == userID && p.DeletedAt != nil {
			photos = append(photos, p)
			delete(m.photos, id)
		}
	}
	return photos, nil
}

func (m *mockRepo) SetPhotoFavorite(id int64, userID int64, favorite bool) error {
	p, ok := m.photos[id]
	if !ok || p.UploadedBy != userID || p.DeletedAt != nil {
		return fmt.Errorf("图片不存在")
	}
	p.IsFavorite = favorite
	return nil
}

func (m *mockRepo) UpdatePhotoEXIF(id int64, userID int64, exif *storage.PhotoEXIF) error {
	p, ok := m.photos[id]
	if !ok || p.UploadedBy != userID || p.DeletedAt != nil {
		return fmt.Errorf("图片不存在")
	}
	p.EXIF = exif
	return nil
}

func (m *mockRepo) CreateAlbum(album *storage.Album) error {
	album.ID = m.genID()
	m.albums[album.ID] = album
	return nil
}

func (m *mockRepo) GetAlbumByID(id int64, userID int64) (*storage.Album, error) {
	a, ok := m.albums[id]
	if !ok || a.CreatedBy != userID {
		return nil, nil
	}
	a.PhotoCount = len(m.albumPhotos[id])
	return a, nil
}

func (m *mockRepo) ListAlbums(userID int64) ([]*storage.Album, error) {
	var albums []*storage.Album
	for _, a := range m.albums {
		if a.CreatedBy == userID {
			albums = append(albums, a)
		}
	}
	return albums, nil
}

func (m *mockRepo) ListAlbumsForPhoto(photoID int64, userID int64) ([]*storage.Album, error) {
	var albums []*storage.Album
	for albumID, photoIDs := range m.albumPhotos {
		a, ok := m.albums[albumID]
		if !ok || a.CreatedBy != userID {
			continue
		}
		for _, id := range photoIDs {
			if id == photoID {
				albums = append(albums, a)
				break
			}
		}
	}
	return albums, nil
}

func (m *mockRepo) UpdateAlbum(album *storage.Album) error {
	m.albums[album.ID] = album
	return nil
}

func (m *mockRepo) DeleteAlbum(id int64, userID int64) error {
	a, ok := m.albums[id]
	if !ok || a.CreatedBy != userID {
		return fmt.Errorf("相册不存在")
	}
	delete(m.albums, id)
	delete(m.albumPhotos, id)
	return nil
}

func (m *mockRepo) AddPhotoToAlbum(albumID int64, photoID int64, userID int64) error {
	a, ok := m.albums[albumID]
	if !ok || a.CreatedBy != userID {
		return fmt.Errorf("相册不存在")
	}
	for _, pid := range m.albumPhotos[albumID] {
		if pid == photoID {
			return nil // 已存在
		}
	}
	m.albumPhotos[albumID] = append(m.albumPhotos[albumID], photoID)
	return nil
}

func (m *mockRepo) RemovePhotoFromAlbum(albumID int64, photoID int64, userID int64) error {
	a, ok := m.albums[albumID]
	if !ok || a.CreatedBy != userID {
		return fmt.Errorf("相册不存在")
	}
	pids := m.albumPhotos[albumID]
	for i, pid := range pids {
		if pid == photoID {
			m.albumPhotos[albumID] = append(pids[:i], pids[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockRepo) ListAlbumPhotos(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error) {
	pids := m.albumPhotos[params.AlbumID]
	var photos []*storage.Photo
	for _, pid := range pids {
		if p, ok := m.photos[pid]; ok && p.DeletedAt == nil {
			photos = append(photos, p)
		}
	}
	return &storage.PhotoPage{Photos: photos}, nil
}

func (m *mockRepo) IsPhotoInAlbum(albumID int64, photoID int64) (bool, error) {
	for _, pid := range m.albumPhotos[albumID] {
		if pid == photoID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockRepo) CreateShareLink(link *storage.ShareLink) error {
	link.ID = m.genID()
	m.shareLinks[link.ID] = link
	return nil
}

func (m *mockRepo) GetShareLinkByToken(token string) (*storage.ShareLink, error) {
	for _, l := range m.shareLinks {
		if l.Token == token {
			if l.ExpiresAt != nil && l.ExpiresAt.Before(time.Now()) {
				return nil, nil
			}
			return l, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) ListShareLinks(userID int64) ([]*storage.ShareLink, error) {
	var links []*storage.ShareLink
	for _, l := range m.shareLinks {
		if l.CreatedBy == userID {
			links = append(links, l)
		}
	}
	return links, nil
}

func (m *mockRepo) DeleteShareLink(id int64, userID int64) error {
	l, ok := m.shareLinks[id]
	if !ok || l.CreatedBy != userID {
		return fmt.Errorf("分享链接不存在")
	}
	delete(m.shareLinks, id)
	return nil
}

func (m *mockRepo) Close() error { return nil }
