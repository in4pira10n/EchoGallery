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

const legacyFolderAlbumDescription = "自动从文件夹导入"

func normalizeAlbumSourcePath(value string) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/")
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

func reverseAlbumPhotos(photos []*storage.Photo) {
	for i, j := 0, len(photos)-1; i < j; i, j = i+1, j-1 {
		photos[i], photos[j] = photos[j], photos[i]
	}
}

func normalizeAlbumBeforePage(page *storage.PhotoPage, sort string) *storage.PhotoPage {
	if page == nil || len(page.Photos) <= 1 {
		return page
	}
	switch normalizeAlbumPhotoSort(sort) {
	case "timeline_desc", "timeline_asc", "size":
		reverseAlbumPhotos(page.Photos)
	}
	return page
}

func albumLocateCursorForPhoto(photo *storage.Photo, sort string) string {
	if photo == nil {
		return ""
	}
	sort = normalizeAlbumPhotoSort(sort)
	cursor := albumPhotoCursor{
		Sort: sort,
		Time: photo.TakenAt,
		Name: photo.OriginalName,
		Size: photo.Size,
		ID:   photo.ID,
	}
	return encodeAlbumPhotoCursor(cursor)
}

func (s *DB) buildAlbumPhotoScope(albumID int64, userID int64, mediaKind string) (string, string, []interface{}, error) {
	var sourceKind string
	var description string
	var sourceRelPath string
	var name string
	if err := s.db.QueryRow(`
		SELECT name, source_kind, description, source_rel_path
		FROM albums
		WHERE id = ? AND created_by = ?`, albumID, userID).Scan(&name, &sourceKind, &description, &sourceRelPath); err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil, nil
		}
		return "", "", nil, fmt.Errorf("查询相册失败: %w", err)
	}
	isFolderAlbum := sourceKind == "folder" || (sourceKind == "" && description == legacyFolderAlbumDescription)
	sourceRelPath = normalizeAlbumSourcePath(sourceRelPath)
	if isFolderAlbum && sourceRelPath == "" {
		sourceRelPath = normalizeAlbumSourcePath(name)
	}
	where := "p.uploaded_by = ? AND p.deleted_at IS NULL"
	args := []interface{}{userID}
	joinClause := ""
	if isFolderAlbum && sourceRelPath != "" {
		where += ` AND (
			(p.source_rel_path LIKE ? AND instr(substr(p.source_rel_path, ?), '/') = 0)
			OR (
				p.source_rel_path = ''
				AND EXISTS (
					SELECT 1 FROM album_photos legacy_ap
					WHERE legacy_ap.album_id = ? AND legacy_ap.photo_id = p.id
				)
			)
		)`
		args = append(args, sourceRelPath+"/%", len(sourceRelPath)+2, albumID)
	} else {
		joinClause = "JOIN album_photos ap ON ap.photo_id = p.id"
		where += " AND ap.album_id = ?"
		args = append(args, albumID)
	}
	if mediaKind != "" {
		where += " AND p.media_kind = ?"
		args = append(args, mediaKind)
	}
	return where, joinClause, args, nil
}

func (s *DB) queryAlbumTarget(where string, joinClause string, args []interface{}, photoID int64) (*storage.Photo, error) {
	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, photoID)
	row := s.db.QueryRow(`
		SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
		       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
		       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
		FROM photos p
		`+joinClause+`
		WHERE `+where+` AND p.id = ?
		LIMIT 1`, queryArgs...)
	target, err := scanPhoto(row)
	if err == sql.ErrNoRows || target == nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询目标媒体失败: %w", err)
	}
	return target, nil
}

func (s *DB) listAlbumNaturalScope(where string, joinClause string, args []interface{}) ([]*storage.Photo, error) {
	queryArgs := append([]interface{}{}, args...)
	rows, err := s.db.Query(`
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			`+joinClause+`
			WHERE `+where+`
			ORDER BY lower(p.original_name) ASC, p.id ASC`, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询相册图片失败: %w", err)
	}
	defer rows.Close()

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
	sortPhotosByNaturalName(photos)
	return photos, nil
}

func findAlbumNaturalCursorStart(photos []*storage.Photo, cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	c, err := decodeAlbumPhotoCursor(cursor, "name")
	if err != nil {
		return 0, err
	}
	start := 0
	for start < len(photos) {
		cmp := compareNaturalStrings(photos[start].OriginalName, c.Name)
		if cmp > 0 || (cmp == 0 && photos[start].ID > c.ID) {
			break
		}
		start++
	}
	return start, nil
}

func trimAlbumPagePhotos(photos []*storage.Photo, limit int, sort string) *storage.PhotoPage {
	page := &storage.PhotoPage{}
	if len(photos) > limit {
		page.HasMore = true
		photos = photos[:limit]
	}
	page.Photos = photos
	if len(photos) > 0 {
		page.NextCursor = albumLocateCursorForPhoto(photos[len(photos)-1], sort)
	}
	return page
}

func (s *DB) queryAlbumLocatePage(query string, args []interface{}, limit int, sort string, reverse bool) (*storage.PhotoPage, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询相册定位页面失败: %w", err)
	}
	defer rows.Close()

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
	hasMore := len(photos) > limit
	if hasMore {
		photos = photos[:limit]
	}
	if reverse {
		reverseAlbumPhotos(photos)
	}
	page := &storage.PhotoPage{
		Photos:  photos,
		HasMore: hasMore,
	}
	if len(photos) > 0 {
		page.NextCursor = albumLocateCursorForPhoto(photos[len(photos)-1], sort)
	}
	return page, nil
}

func (s *DB) countAlbumBefore(where string, joinClause string, args []interface{}, target *storage.Photo, sort string) (int, error) {
	queryArgs := append([]interface{}{}, args...)
	var condition string
	switch normalizeAlbumPhotoSort(sort) {
	case "timeline_asc":
		condition = " AND (p.taken_at < ? OR (p.taken_at = ? AND p.id < ?))"
		queryArgs = append(queryArgs, target.TakenAt, target.TakenAt, target.ID)
	case "size":
		condition = " AND (p.size > ? OR (p.size = ? AND p.id > ?))"
		queryArgs = append(queryArgs, target.Size, target.Size, target.ID)
	default:
		condition = " AND (p.taken_at > ? OR (p.taken_at = ? AND p.id > ?))"
		queryArgs = append(queryArgs, target.TakenAt, target.TakenAt, target.ID)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM photos p `+joinClause+` WHERE `+where+condition, queryArgs...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计相册目标前媒体失败: %w", err)
	}
	return count, nil
}

func (s *DB) locateAlbumPages(where string, joinClause string, args []interface{}, target *storage.Photo, limit int, sort string) (*storage.PhotoPage, *storage.PhotoPage, error) {
	baseArgs := append([]interface{}{}, args...)
	var beforeQuery string
	var afterQuery string
	var beforeArgs []interface{}
	var afterArgs []interface{}
	beforeArgs = append([]interface{}{}, baseArgs...)
	afterArgs = append([]interface{}{}, baseArgs...)
	switch normalizeAlbumPhotoSort(sort) {
	case "timeline_asc":
		beforeArgs = append(beforeArgs, target.TakenAt, target.TakenAt, target.ID, limit+1)
		beforeQuery = `
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			` + joinClause + `
			WHERE ` + where + `
			  AND (p.taken_at < ? OR (p.taken_at = ? AND p.id < ?))
			ORDER BY p.taken_at DESC, p.id DESC
			LIMIT ?`
		afterArgs = append(afterArgs, target.TakenAt, target.TakenAt, target.ID, limit+1)
		afterQuery = `
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			` + joinClause + `
			WHERE ` + where + `
			  AND (p.taken_at > ? OR (p.taken_at = ? AND p.id > ?))
			ORDER BY p.taken_at ASC, p.id ASC
			LIMIT ?`
	case "size":
		beforeArgs = append(beforeArgs, target.Size, target.Size, target.ID, limit+1)
		beforeQuery = `
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			` + joinClause + `
			WHERE ` + where + `
			  AND (p.size > ? OR (p.size = ? AND p.id > ?))
			ORDER BY p.size ASC, p.id ASC
			LIMIT ?`
		afterArgs = append(afterArgs, target.Size, target.Size, target.ID, limit+1)
		afterQuery = `
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			` + joinClause + `
			WHERE ` + where + `
			  AND (p.size < ? OR (p.size = ? AND p.id < ?))
			ORDER BY p.size DESC, p.id DESC
			LIMIT ?`
	default:
		beforeArgs = append(beforeArgs, target.TakenAt, target.TakenAt, target.ID, limit+1)
		beforeQuery = `
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			` + joinClause + `
			WHERE ` + where + `
			  AND (p.taken_at > ? OR (p.taken_at = ? AND p.id > ?))
			ORDER BY p.taken_at ASC, p.id ASC
			LIMIT ?`
		afterArgs = append(afterArgs, target.TakenAt, target.TakenAt, target.ID, limit+1)
		afterQuery = `
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			` + joinClause + `
			WHERE ` + where + `
			  AND (p.taken_at < ? OR (p.taken_at = ? AND p.id < ?))
			ORDER BY p.taken_at DESC, p.id DESC
			LIMIT ?`
	}

	beforePage, err := s.queryAlbumLocatePage(beforeQuery, beforeArgs, limit, sort, true)
	if err != nil {
		return nil, nil, err
	}
	afterPage, err := s.queryAlbumLocatePage(afterQuery, afterArgs, limit, sort, false)
	if err != nil {
		return nil, nil, err
	}
	return beforePage, afterPage, nil
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
		WITH user_albums AS (
			SELECT id, name, description, cover_photo_id, source_kind, source_rel_path, created_by, created_at
			FROM albums
			WHERE created_by = ?
		),
		photo_counts AS (
			SELECT ap.album_id, COUNT(p.id) AS photo_count
			FROM album_photos ap
			INNER JOIN user_albums ua ON ua.id = ap.album_id
			INNER JOIN photos p ON p.id = ap.photo_id AND p.deleted_at IS NULL
			GROUP BY ap.album_id
		),
		latest_album_photos AS (
			SELECT album_id, uuid
			FROM (
				SELECT ap.album_id, ph.uuid,
				       ROW_NUMBER() OVER (PARTITION BY ap.album_id ORDER BY ph.taken_at DESC, ph.id DESC) AS rn
				FROM album_photos ap
				INNER JOIN user_albums ua ON ua.id = ap.album_id
				INNER JOIN photos ph ON ph.id = ap.photo_id AND ph.deleted_at IS NULL
			)
			WHERE rn = 1
		)
		SELECT ua.id, ua.name, ua.description, ua.cover_photo_id, ua.source_kind, ua.source_rel_path, ua.created_by, ua.created_at,
		       COALESCE(pc.photo_count, 0) AS photo_count,
		       COALESCE(cover.uuid, latest.uuid) AS cover_uuid
		FROM user_albums ua
		LEFT JOIN photo_counts pc ON pc.album_id = ua.id
		LEFT JOIN photos cover ON cover.id = ua.cover_photo_id AND cover.deleted_at IS NULL
		LEFT JOIN latest_album_photos latest ON latest.album_id = ua.id
		ORDER BY ua.created_at DESC`, userID)
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
		WITH target_photo AS (
			SELECT id, source_rel_path
			FROM photos
			WHERE id = ? AND uploaded_by = ? AND deleted_at IS NULL
		)
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
		FROM target_photo
		INNER JOIN albums a ON a.created_by = ?
		LEFT JOIN album_photos target_ap ON target_ap.album_id = a.id AND target_ap.photo_id = target_photo.id
		LEFT JOIN album_photos ap ON ap.album_id = a.id
		LEFT JOIN photos p ON p.id = ap.photo_id AND p.deleted_at IS NULL
		WHERE (
			(
				target_ap.photo_id IS NOT NULL
				AND (
					NOT (a.source_kind = 'folder' OR (a.source_kind = '' AND a.description = '自动从文件夹导入'))
					OR target_photo.source_rel_path = ''
					OR (
						(CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) <> ''
						AND target_photo.source_rel_path LIKE (CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) || '/%'
						AND instr(substr(target_photo.source_rel_path, length(CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) + 2), '/') = 0
					)
				)
			)
			OR (
				(a.source_kind = 'folder' OR (a.source_kind = '' AND a.description = '自动从文件夹导入'))
				AND (CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) <> ''
				AND target_photo.source_rel_path LIKE (CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) || '/%'
				AND instr(substr(target_photo.source_rel_path, length(CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) + 2), '/') = 0
			)
		)
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

// RefreshFolderAlbumCovers 刷新文件夹相册封面，避免每次查询相册列表时递归计算。
func (s *DB) RefreshFolderAlbumCovers(userID int64) error {
	_, err := s.db.Exec(`
		UPDATE albums AS a
		SET cover_photo_id = COALESCE(
			(SELECT ph.id FROM photos ph
			 INNER JOIN album_photos ap ON ap.photo_id = ph.id
			 WHERE ap.album_id = a.id AND ph.deleted_at IS NULL
			 ORDER BY ph.taken_at DESC, ph.id DESC LIMIT 1),
			(SELECT ph.id FROM photos ph
			 WHERE (CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) <> ''
			   AND ph.uploaded_by = a.created_by
			   AND ph.deleted_at IS NULL
			   AND substr(ph.source_rel_path, 1, length(CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) + 1) = (CASE WHEN a.source_rel_path <> '' THEN a.source_rel_path ELSE trim(a.name) END) || '/'
			 ORDER BY ph.taken_at DESC, ph.id DESC LIMIT 1)
		)
		WHERE a.created_by = ?
		  AND (a.source_kind = 'folder' OR (a.source_kind = '' AND a.description = '自动从文件夹导入'))`, userID)
	if err != nil {
		return fmt.Errorf("刷新文件夹相册封面失败: %w", err)
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
	var sourceKind string
	var description string
	var sourceRelPath string
	var name string
	if err := s.db.QueryRow(`
		SELECT name, source_kind, description, source_rel_path
		FROM albums
		WHERE id = ? AND created_by = ?`, params.AlbumID, params.UserID).Scan(&name, &sourceKind, &description, &sourceRelPath); err != nil {
		if err == sql.ErrNoRows {
			return &storage.PhotoPage{}, nil
		}
		return nil, fmt.Errorf("查询相册失败: %w", err)
	}
	isFolderAlbum := sourceKind == "folder" || (sourceKind == "" && description == legacyFolderAlbumDescription)
	sourceRelPath = normalizeAlbumSourcePath(sourceRelPath)
	if isFolderAlbum && sourceRelPath == "" {
		sourceRelPath = normalizeAlbumSourcePath(name)
	}
	where := "p.uploaded_by = ? AND p.deleted_at IS NULL"
	args := []interface{}{params.UserID}
	joinClause := ""
	if isFolderAlbum && sourceRelPath != "" {
		where += ` AND (
			(p.source_rel_path LIKE ? AND instr(substr(p.source_rel_path, ?), '/') = 0)
			OR (
				p.source_rel_path = ''
				AND EXISTS (
					SELECT 1 FROM album_photos legacy_ap
					WHERE legacy_ap.album_id = ? AND legacy_ap.photo_id = p.id
				)
			)
		)`
		args = append(args, sourceRelPath+"/%", len(sourceRelPath)+2, params.AlbumID)
	} else {
		joinClause = "JOIN album_photos ap ON ap.photo_id = p.id"
		where += " AND ap.album_id = ?"
		args = append(args, params.AlbumID)
	}
	if params.MediaKind != "" {
		where += " AND p.media_kind = ?"
		args = append(args, params.MediaKind)
	}
	if sort == "name" {
		return s.listAlbumPhotosByNaturalName(where, joinClause, args, params.Cursor, limit)
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
	case "size":
		orderBy = "p.size DESC, p.id DESC"
	}

	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, limit+1)
	rows, err := s.db.Query(`
			SELECT p.id, p.uuid, p.original_name, p.media_kind, p.mime_type, p.size, p.width, p.height, p.duration_ms,
			       p.storage_rel_path, p.source_rel_path, p.exif_json, p.is_favorite, p.is_super_favorite,
			       p.taken_at, p.uploaded_at, p.uploaded_by, p.deleted_at, p.deleted_by
			FROM photos p
			`+joinClause+`
			WHERE `+where+`
			ORDER BY `+orderBy+`
			LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询相册图片失败: %w", err)
	}
	defer rows.Close()

	page, err := collectAlbumPhotoPage(rows, limit, sort)
	if err != nil {
		return nil, err
	}
	if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
		return nil, err
	}
	return page, nil
}

func (s *DB) LocateAlbumWindow(params storage.LocateAlbumParams) (*storage.TimelineLocateResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	sort := normalizeAlbumPhotoSort(params.Sort)
	where, joinClause, args, err := s.buildAlbumPhotoScope(params.AlbumID, params.UserID, params.MediaKind)
	if err != nil {
		return nil, err
	}
	if where == "" {
		return nil, nil
	}
	if sort == "name" {
		photos, err := s.listAlbumNaturalScope(where, joinClause, args)
		if err != nil {
			return nil, err
		}
		targetIndex := -1
		for i, photo := range photos {
			if photo != nil && photo.ID == params.PhotoID {
				targetIndex = i
				break
			}
		}
		if targetIndex < 0 {
			return nil, nil
		}
		start := targetIndex - limit
		if start < 0 {
			start = 0
		}
		end := targetIndex + limit + 1
		if end > len(photos) {
			end = len(photos)
		}
		windowPhotos := append([]*storage.Photo(nil), photos[start:end]...)
		if err := s.attachSharedPlaybackMetadata(windowPhotos); err != nil {
			return nil, err
		}
		result := &storage.TimelineLocateResult{
			Photos:             windowPhotos,
			TargetIndex:        targetIndex - start,
			HasBefore:          start > 0,
			HasAfter:           end < len(photos),
			MissingBeforeCount: start,
		}
		if len(windowPhotos) > 0 {
			result.PrevCursor = albumLocateCursorForPhoto(windowPhotos[0], sort)
			result.NextCursor = albumLocateCursorForPhoto(windowPhotos[len(windowPhotos)-1], sort)
		}
		return result, nil
	}

	target, err := s.queryAlbumTarget(where, joinClause, args, params.PhotoID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, nil
	}
	missingBeforeCount, err := s.countAlbumBefore(where, joinClause, args, target, sort)
	if err != nil {
		return nil, err
	}
	beforePage, afterPage, err := s.locateAlbumPages(where, joinClause, args, target, limit, sort)
	if err != nil {
		return nil, err
	}
	photos := make([]*storage.Photo, 0, len(beforePage.Photos)+1+len(afterPage.Photos))
	photos = append(photos, beforePage.Photos...)
	targetIndex := len(photos)
	photos = append(photos, target)
	photos = append(photos, afterPage.Photos...)
	if err := s.attachSharedPlaybackMetadata(photos); err != nil {
		return nil, err
	}
	result := &storage.TimelineLocateResult{
		Photos:             photos,
		TargetIndex:        targetIndex,
		HasBefore:          beforePage.HasMore,
		HasAfter:           afterPage.HasMore,
		MissingBeforeCount: missingBeforeCount,
	}
	if len(beforePage.Photos) > 0 {
		result.PrevCursor = albumLocateCursorForPhoto(beforePage.Photos[0], sort)
	} else {
		result.PrevCursor = albumLocateCursorForPhoto(target, sort)
	}
	if len(afterPage.Photos) > 0 {
		result.NextCursor = albumLocateCursorForPhoto(afterPage.Photos[len(afterPage.Photos)-1], sort)
	} else {
		result.NextCursor = albumLocateCursorForPhoto(target, sort)
	}
	return result, nil
}

func (s *DB) ListAlbumPhotosBefore(params storage.LocateAlbumParams) (*storage.PhotoPage, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	sort := normalizeAlbumPhotoSort(params.Sort)
	where, joinClause, args, err := s.buildAlbumPhotoScope(params.AlbumID, params.UserID, params.MediaKind)
	if err != nil {
		return nil, err
	}
	if where == "" {
		return &storage.PhotoPage{}, nil
	}
	if sort == "name" {
		photos, err := s.listAlbumNaturalScope(where, joinClause, args)
		if err != nil {
			return nil, err
		}
		targetIndex := -1
		for i, photo := range photos {
			if photo != nil && photo.ID == params.PhotoID {
				targetIndex = i
				break
			}
		}
		if targetIndex <= 0 {
			return &storage.PhotoPage{}, nil
		}
		start := targetIndex - limit
		if start < 0 {
			start = 0
		}
		page := &storage.PhotoPage{
			Photos:     append([]*storage.Photo(nil), photos[start:targetIndex]...),
			HasMore:    start > 0,
			NextCursor: "",
		}
		if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
			return nil, err
		}
		return page, nil
	}
	target, err := s.queryAlbumTarget(where, joinClause, args, params.PhotoID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return &storage.PhotoPage{}, nil
	}
	beforePage, _, err := s.locateAlbumPages(where, joinClause, args, target, limit, sort)
	if err != nil {
		return nil, err
	}
	if err := s.attachSharedPlaybackMetadata(beforePage.Photos); err != nil {
		return nil, err
	}
	return beforePage, nil
}

func (s *DB) listAlbumPhotosByNaturalName(where string, joinClause string, args []interface{}, cursor string, limit int) (*storage.PhotoPage, error) {
	photos, err := s.listAlbumNaturalScope(where, joinClause, args)
	if err != nil {
		return nil, err
	}
	start, err := findAlbumNaturalCursorStart(photos, cursor)
	if err != nil {
		return nil, err
	}
	end := start + limit
	if end > len(photos) {
		end = len(photos)
	}
	page := trimAlbumPagePhotos(append([]*storage.Photo(nil), photos[start:end]...), limit, "name")
	if start+limit < len(photos) && len(page.Photos) > 0 {
		page.HasMore = true
		page.NextCursor = albumLocateCursorForPhoto(page.Photos[len(page.Photos)-1], "name")
	}
	if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
		return nil, err
	}
	return page, nil
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
