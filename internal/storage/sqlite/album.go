package sqlite

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"echogallery/internal/storage"
)

type albumPhotoCursor struct {
	Sort string    `json:"s"`
	Time time.Time `json:"t,omitempty"`
	Name string    `json:"n,omitempty"`
	Size int64     `json:"z,omitempty"`
	ID   int64     `json:"i"`
}

func normalizeAlbumPhotoSort(sort string) string {
	switch strings.TrimSpace(strings.ToLower(sort)) {
	case "name":
		return "name"
	case "size":
		return "size"
	case "timeline_asc":
		return "timeline_asc"
	default:
		return "timeline_desc"
	}
}

func encodeAlbumPhotoCursor(c albumPhotoCursor) string {
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decodeAlbumPhotoCursor(value string, sort string) (*albumPhotoCursor, error) {
	b, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("无效的相册游标: %w", err)
	}
	var c albumPhotoCursor
	if err := json.Unmarshal(b, &c); err == nil && c.ID > 0 {
		if c.Sort == "" {
			c.Sort = normalizeAlbumPhotoSort(sort)
		}
		return &c, nil
	}
	legacy, err := decodeCursor(value)
	if err != nil {
		return nil, fmt.Errorf("无效的相册游标: %w", err)
	}
	return &albumPhotoCursor{Sort: normalizeAlbumPhotoSort(sort), Time: legacy.TakenAt, ID: legacy.ID}, nil
}

// CreateAlbum 创建相册
func (s *DB) CreateAlbum(album *storage.Album) error {
	result, err := s.db.Exec(`
		INSERT INTO albums (name, description, cover_photo_id, source_kind, source_rel_path, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		album.Name, album.Description, album.CoverPhotoID,
		album.SourceKind, album.SourceRelPath,
		album.CreatedBy, album.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("创建相册失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	album.ID = id
	return nil
}

// GetAlbumByID 按 ID 查询相册，附带图片数量（不含已软删除的图片）
func (s *DB) GetAlbumByID(id int64, userID int64) (*storage.Album, error) {
	row := s.db.QueryRow(`
		SELECT a.id, a.name, a.description, a.cover_photo_id, a.source_kind, a.source_rel_path, a.created_by, a.created_at,
		       COUNT(p.id) as photo_count,
		       COALESCE(
		         (SELECT ph.uuid FROM photos ph
		          WHERE ph.id = a.cover_photo_id AND ph.deleted_at IS NULL LIMIT 1),
		         (SELECT ph.uuid FROM photos ph
		          INNER JOIN album_photos ap2 ON ap2.photo_id = ph.id
		          WHERE ap2.album_id = a.id AND ph.deleted_at IS NULL
		          ORDER BY ph.taken_at DESC LIMIT 1)
		       ) as cover_uuid
		FROM albums a
		LEFT JOIN album_photos ap ON ap.album_id = a.id
		LEFT JOIN photos p ON p.id = ap.photo_id AND p.deleted_at IS NULL
		WHERE a.id = ? AND a.created_by = ?
		GROUP BY a.id`, id, userID)

	album, err := scanAlbum(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return album, err
}

// ListAlbums 查询用户所有相册（photo_count 不含已软删除的图片，附带封面 UUID）
func (s *DB) ListAlbums(userID int64) ([]*storage.Album, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.description, a.cover_photo_id, a.source_kind, a.source_rel_path, a.created_by, a.created_at,
		       COUNT(p.id) as photo_count,
		       COALESCE(
		         (SELECT ph.uuid FROM photos ph
		          WHERE ph.id = a.cover_photo_id AND ph.deleted_at IS NULL LIMIT 1),
		         (SELECT ph.uuid FROM photos ph
		          INNER JOIN album_photos ap2 ON ap2.photo_id = ph.id
		          WHERE ap2.album_id = a.id AND ph.deleted_at IS NULL
		          ORDER BY ph.taken_at DESC LIMIT 1)
		       ) as cover_uuid
		FROM albums a
		LEFT JOIN album_photos ap ON ap.album_id = a.id
		LEFT JOIN photos p ON p.id = ap.photo_id AND p.deleted_at IS NULL
		WHERE a.created_by = ?
		GROUP BY a.id
		ORDER BY a.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("查询相册失败: %w", err)
	}
	defer rows.Close()

	var albums []*storage.Album
	for rows.Next() {
		a, err := scanAlbum(rows)
		if err != nil {
			return nil, err
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// ListAlbumsForPhoto 查询包含指定图片/视频的相册。
func (s *DB) ListAlbumsForPhoto(photoID int64, userID int64) ([]*storage.Album, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.description, a.cover_photo_id, a.source_kind, a.source_rel_path, a.created_by, a.created_at,
		       COUNT(p.id) as photo_count,
		       COALESCE(
		         (SELECT ph.uuid FROM photos ph
		          WHERE ph.id = a.cover_photo_id AND ph.deleted_at IS NULL LIMIT 1),
		         (SELECT ph.uuid FROM photos ph
		          INNER JOIN album_photos ap2 ON ap2.photo_id = ph.id
		          WHERE ap2.album_id = a.id AND ph.deleted_at IS NULL
		          ORDER BY ph.taken_at DESC LIMIT 1)
		       ) as cover_uuid
		FROM albums a
		INNER JOIN album_photos target_ap ON target_ap.album_id = a.id AND target_ap.photo_id = ?
		INNER JOIN photos target_photo ON target_photo.id = target_ap.photo_id
			AND target_photo.uploaded_by = ? AND target_photo.deleted_at IS NULL
		LEFT JOIN album_photos ap ON ap.album_id = a.id
		LEFT JOIN photos p ON p.id = ap.photo_id AND p.deleted_at IS NULL
		WHERE a.created_by = ?
		GROUP BY a.id
		ORDER BY a.created_at DESC`, photoID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("查询媒体所在相册失败: %w", err)
	}
	defer rows.Close()

	var albums []*storage.Album
	for rows.Next() {
		a, err := scanAlbum(rows)
		if err != nil {
			return nil, err
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// UpdateAlbum 更新相册信息
func (s *DB) UpdateAlbum(album *storage.Album) error {
	result, err := s.db.Exec(`
		UPDATE albums SET name = ?, description = ?, cover_photo_id = ?
		WHERE id = ? AND created_by = ?`,
		album.Name, album.Description, album.CoverPhotoID,
		album.ID, album.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("更新相册失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("相册不存在")
	}
	return nil
}

// DeleteAlbum 删除相册（级联删除 album_photos 关联，不删除图片本身）
func (s *DB) DeleteAlbum(id int64, userID int64) error {
	result, err := s.db.Exec(`DELETE FROM albums WHERE id = ? AND created_by = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("删除相册失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("相册不存在")
	}
	return nil
}

// AddPhotoToAlbum 将图片添加到相册
func (s *DB) AddPhotoToAlbum(albumID int64, photoID int64, userID int64) error {
	// 验证相册属于当前用户
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM albums WHERE id = ? AND created_by = ?`,
		albumID, userID).Scan(&count); err != nil || count == 0 {
		return fmt.Errorf("相册不存在")
	}
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO album_photos (album_id, photo_id, added_at) VALUES (?, ?, ?)`,
		albumID, photoID, time.Now())
	if err != nil {
		return fmt.Errorf("添加图片到相册失败: %w", err)
	}
	return nil
}

// RemovePhotoFromAlbum 从相册移除图片
func (s *DB) RemovePhotoFromAlbum(albumID int64, photoID int64, userID int64) error {
	// 验证相册属于当前用户
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM albums WHERE id = ? AND created_by = ?`,
		albumID, userID).Scan(&count); err != nil || count == 0 {
		return fmt.Errorf("相册不存在")
	}
	_, err := s.db.Exec(`DELETE FROM album_photos WHERE album_id = ? AND photo_id = ?`,
		albumID, photoID)
	return err
}

// ListAlbumPhotos 查询相册内图片（游标分页，支持相册详情页排序）。
func (s *DB) ListAlbumPhotos(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	sort := normalizeAlbumPhotoSort(params.Sort)
	where := "ap.album_id = ? AND p.uploaded_by = ? AND p.deleted_at IS NULL"
	args := []interface{}{params.AlbumID, params.UserID}
	if params.MediaKind != "" {
		where += " AND p.media_kind = ?"
		args = append(args, params.MediaKind)
	}

	orderBy := "p.taken_at DESC, p.id DESC"
	if params.Cursor != "" {
		c, err := decodeAlbumPhotoCursor(params.Cursor, sort)
		if err != nil {
			return nil, err
		}
		switch sort {
		case "timeline_asc":
			where += " AND (p.taken_at > ? OR (p.taken_at = ? AND p.id > ?))"
			args = append(args, c.Time, c.Time, c.ID)
		case "name":
			where += " AND (lower(p.original_name) > ? OR (lower(p.original_name) = ? AND p.id > ?))"
			args = append(args, c.Name, c.Name, c.ID)
		case "size":
			where += " AND (p.size < ? OR (p.size = ? AND p.id < ?))"
			args = append(args, c.Size, c.Size, c.ID)
		default:
			where += " AND (p.taken_at < ? OR (p.taken_at = ? AND p.id < ?))"
			args = append(args, c.Time, c.Time, c.ID)
		}
	}
	switch sort {
	case "timeline_asc":
		orderBy = "p.taken_at ASC, p.id ASC"
	case "name":
		orderBy = "lower(p.original_name) ASC, p.id ASC"
	case "size":
		orderBy = "p.size DESC, p.id DESC"
	}

	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, limit+1)
	rows, err := s.db.Query(`
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			JOIN album_photos ap ON ap.photo_id = p.id
			WHERE `+where+`
			ORDER BY `+orderBy+`
			LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询相册图片失败: %w", err)
	}
	defer rows.Close()

	return collectAlbumPhotoPage(rows, limit, sort)
}

func collectAlbumPhotoPage(rows *sql.Rows, limit int, sort string) (*storage.PhotoPage, error) {
	var photos []*storage.Photo
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	page := &storage.PhotoPage{}
	if len(photos) > limit {
		page.HasMore = true
		photos = photos[:limit]
		last := photos[len(photos)-1]
		cursor := albumPhotoCursor{Sort: sort, Time: last.TakenAt, Name: strings.ToLower(last.OriginalName), Size: last.Size, ID: last.ID}
		page.NextCursor = encodeAlbumPhotoCursor(cursor)
	}
	page.Photos = photos
	return page, nil
}

// IsPhotoInAlbum 检查图片是否在相册中
func (s *DB) IsPhotoInAlbum(albumID int64, photoID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM album_photos WHERE album_id = ? AND photo_id = ?`,
		albumID, photoID).Scan(&count)
	return count > 0, err
}

// scanAlbum 从数据库行扫描 Album 对象
func scanAlbum(row interface {
	Scan(...interface{}) error
}) (*storage.Album, error) {
	var a storage.Album
	var coverPhotoID sql.NullInt64
	var coverUUID sql.NullString
	err := row.Scan(
		&a.ID, &a.Name, &a.Description, &coverPhotoID,
		&a.SourceKind, &a.SourceRelPath,
		&a.CreatedBy, &a.CreatedAt, &a.PhotoCount, &coverUUID,
	)
	if err != nil {
		return nil, err
	}
	if coverPhotoID.Valid {
		a.CoverPhotoID = &coverPhotoID.Int64
	}
	if coverUUID.Valid {
		a.CoverUUID = coverUUID.String
	}
	return &a, nil
}
