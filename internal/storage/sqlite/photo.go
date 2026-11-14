package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"slices"
	"strings"
	"time"

	"echogallery/internal/storage"
)

// cursor 游标结构，用于分页
type cursor struct {
	TakenAt time.Time `json:"t"`
	ID      int64     `json:"i"`
}

type randomCursor struct {
	Key     int64 `json:"k"`
	ID      int64 `json:"i"`
	Wrapped bool  `json:"w"`
}

type favoriteCursor struct {
	Super   bool      `json:"s"`
	TakenAt time.Time `json:"t"`
	ID      int64     `json:"i"`
}

const randomSortKeyMax int64 = 2147483647

// encodeCursor 将游标编码为字符串
func encodeCursor(takenAt time.Time, id int64) string {
	c := cursor{TakenAt: takenAt, ID: id}
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

// decodeCursor 解码游标字符串
func decodeCursor(s string) (*cursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("无效的游标: %w", err)
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("无效的游标: %w", err)
	}
	return &c, nil
}

func encodeFavoriteCursor(superFavorite bool, takenAt time.Time, id int64) string {
	c := favoriteCursor{Super: superFavorite, TakenAt: takenAt, ID: id}
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decodeFavoriteCursor(s string) (*favoriteCursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("无效的游标: %w", err)
	}
	var c favoriteCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("无效的游标: %w", err)
	}
	return &c, nil
}

func encodeRandomCursor(key, id int64, wrapped bool) string {
	c := randomCursor{Key: key, ID: id, Wrapped: wrapped}
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decodeRandomCursor(s string) (*randomCursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("无效的乱序游标: %w", err)
	}
	var c randomCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("无效的乱序游标: %w", err)
	}
	return &c, nil
}

func normalizeRandomSeed(seed int64) int64 {
	if seed < 0 {
		seed = -seed
	}
	seed = seed % randomSortKeyMax
	if seed == 0 {
		return 1
	}
	return seed
}

func makeRandomSortKey(photo *storage.Photo) int64 {
	input := fmt.Sprintf("%s:%s:%d:%d", photo.UUID, photo.OriginalName, photo.UploadedBy, photo.UploadedAt.UnixNano())
	h := fnv.New64a()
	_, _ = h.Write([]byte(input))
	return int64(h.Sum64()%uint64(randomSortKeyMax)) + 1
}

// scanPhoto isolates conversion failures to one media row. Scan raw values first
// so a malformed field cannot hide later fields needed to advance the cursor.
func scanPhoto(row interface{ Scan(...interface{}) error }) (*storage.Photo, error) {
	return scanPhotoRecord(row, nil)
}

func scanPhotoWithExtraInt64(row interface{ Scan(...interface{}) error }, extra *int64) (*storage.Photo, error) {
	return scanPhotoRecord(row, extra)
}

type invalidMediaRowError struct {
	id    int64
	cause error
}

func (e *invalidMediaRowError) Error() string {
	return fmt.Sprintf("invalid media row %d: %v", e.id, e.cause)
}
func (e *invalidMediaRowError) Unwrap() error { return e.cause }

func skipInvalidMediaRow(err error) bool {
	var invalid *invalidMediaRowError
	if !errors.As(err, &invalid) {
		return false
	}
	log.Printf("Skipping media metadata: %v", err)
	return true
}

// sql.Null* use database/sql's conversion rules, while accepting absent legacy
// optional fields. Query/driver errors still propagate from row.Scan unchanged.
func scanMediaColumn(dest any, value any) error {
	switch dest := dest.(type) {
	case *string:
		var v sql.NullString
		if err := v.Scan(value); err != nil {
			return err
		}
		*dest = v.String
	case *int64:
		var v sql.NullInt64
		if err := v.Scan(value); err != nil {
			return err
		}
		*dest = v.Int64
	case *int:
		var v sql.NullInt64
		if err := v.Scan(value); err != nil {
			return err
		}
		if int64(int(v.Int64)) != v.Int64 {
			return fmt.Errorf("integer out of range")
		}
		*dest = int(v.Int64)
	case *bool:
		var v sql.NullBool
		if err := v.Scan(value); err != nil {
			return err
		}
		*dest = v.Bool
	case sql.Scanner:
		return dest.Scan(value)
	default:
		return fmt.Errorf("unsupported media column %T", dest)
	}
	return nil
}

func scanPhotoRecord(row interface{ Scan(...interface{}) error }, extra *int64) (*storage.Photo, error) {
	var p storage.Photo
	var takenAt, uploadedAt, deletedAt flexibleSQLiteTime
	var deletedBy sql.NullInt64
	var exifJSON string
	dest := []any{
		&p.ID, &p.UUID, &p.OriginalName, &p.MediaKind, &p.MimeType,
		&p.Size, &p.Width, &p.Height, &p.DurationMS, &p.StorageRelPath, &p.SourceRelPath, &exifJSON, &p.IsFavorite, &p.IsSuperFavorite,
		&takenAt, &uploadedAt, &p.UploadedBy, &deletedAt, &deletedBy,
	}
	if extra != nil {
		dest = append(dest, extra)
	}
	raw := make([]any, len(dest))
	args := make([]any, len(dest))
	for i := range raw {
		args[i] = &raw[i]
	}
	if err := row.Scan(args...); err != nil {
		return nil, err
	}
	var conversionErr error
	for i := range dest {
		if err := scanMediaColumn(dest[i], raw[i]); err != nil && conversionErr == nil {
			conversionErr = fmt.Errorf("column %d: %w", i, err)
		}
	}
	// An INTEGER PRIMARY KEY is always readable; do not mask a broken query.
	if p.ID <= 0 {
		return nil, fmt.Errorf("invalid media ID: %v", raw[0])
	}
	p.UploadedAt = uploadedAt.Time
	if !uploadedAt.Valid {
		p.UploadedAt = time.Unix(0, 0).UTC()
	}
	p.TakenAt = takenAt.Time
	if !takenAt.Valid {
		p.TakenAt = p.UploadedAt
	}
	// A non-NULL deletion marker must stay deleted even if its date is unknown.
	if raw[17] != nil {
		if !deletedAt.Valid {
			deletedAt.Time = time.Unix(0, 0).UTC()
		}
		p.DeletedAt = &deletedAt.Time
	}
	if deletedBy.Valid {
		p.DeletedBy = &deletedBy.Int64
	}
	if extra != nil {
		p.RandomSortKey = *extra
	}
	applyPhotoEXIFJSON(&p, exifJSON)
	if conversionErr != nil {
		return &p, &invalidMediaRowError{id: p.ID, cause: conversionErr}
	}
	return &p, nil
}

// Keep the database's exact ordering value. Re-encoding a time.Time loses zone
// names (CST becomes +0800), and fallback display dates must never drive SQL
// pagination. This is one primary-key lookup per boundary, not per media row.
func (s *DB) mediaSortBoundary(ctx context.Context, id int64, column string, fallback any) (any, error) {
	if column != "taken_at" && column != "deleted_at" && column != "size" && column != "is_super_favorite" {
		return nil, fmt.Errorf("invalid media sort column")
	}
	var kind string
	var raw any
	err := s.db.QueryRowContext(ctx, "SELECT typeof("+column+"), CAST("+column+" AS TEXT) FROM photos WHERE id = ?", id).Scan(&kind, &raw)
	if err == sql.ErrNoRows {
		return fallback, nil
	}
	if err != nil {
		return nil, err
	}
	switch kind {
	case "integer":
		var number sql.NullInt64
		if err := number.Scan(raw); err != nil {
			return nil, err
		}
		return number.Int64, nil
	case "real":
		var number sql.NullFloat64
		if err := number.Scan(raw); err != nil {
			return nil, err
		}
		return number.Float64, nil
	case "blob":
		if value, ok := raw.(string); ok {
			return []byte(value), nil
		}
	}
	return raw, nil
}

type flexibleSQLiteTime struct {
	Time    time.Time
	Valid   bool
	Present bool
}

func (t *flexibleSQLiteTime) Scan(value any) error {
	if t == nil {
		return fmt.Errorf("cannot scan SQLite time into nil receiver")
	}
	t.Time = time.Time{}
	t.Valid = false
	t.Present = value != nil
	switch value := value.(type) {
	case time.Time:
		t.Time = value
		t.Valid = !value.IsZero()
	case string:
		t.Time, t.Valid = parseFlexibleSQLiteTime(value)
	case []byte:
		t.Time, t.Valid = parseFlexibleSQLiteTime(string(value))
	case int64:
		if value > 1_000_000_000_000 || value < -1_000_000_000_000 {
			t.Time = time.UnixMilli(value).UTC()
		} else {
			t.Time = time.Unix(value, 0).UTC()
		}
		t.Valid = true
	}
	if t.Time.Year() < 1 || t.Time.Year() > 9999 {
		t.Time, t.Valid = time.Time{}, false
	}
	return nil
}

func parseFlexibleSQLiteTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if i := strings.Index(value, " m="); i >= 0 {
		value = value[:i]
	}
	fields := strings.Fields(value)
	if len(fields) == 4 && fields[2] == fields[3] && (strings.HasPrefix(fields[3], "+") || strings.HasPrefix(fields[3], "-")) {
		value = strings.Join(fields[:3], " ")
	}
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05.999999999 -07:00",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func (s *DB) attachSharedPlaybackMetadata(photos []*storage.Photo) error {
	if len(photos) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(photos))
	index := make(map[int64]*storage.Photo, len(photos))
	for _, photo := range photos {
		if photo == nil || photo.ID <= 0 || photo.MediaKind != storage.MediaKindVideo {
			continue
		}
		if _, exists := index[photo.ID]; exists {
			continue
		}
		index[photo.ID] = photo
		ids = append(ids, photo.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	rows, err := s.db.Query(`
		SELECT photo_id, resume_time, bookmarks_json
		FROM video_playback_preferences
		WHERE photo_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return fmt.Errorf("加载共享播放偏好失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			photoID      int64
			resumeTime   int64
			bookmarksRaw string
		)
		if err := rows.Scan(&photoID, &resumeTime, &bookmarksRaw); err != nil {
			return err
		}
		photo := index[photoID]
		if photo == nil {
			continue
		}
		photo.VideoResumeTime = resumeTime
		photo.VideoBookmarkCount = len(decodePlaybackBookmarks(bookmarksRaw))
	}
	return rows.Err()
}

func applyPhotoEXIFJSON(photo *storage.Photo, raw string) {
	if photo == nil || strings.TrimSpace(raw) == "" {
		return
	}
	var exif storage.PhotoEXIF
	if err := json.Unmarshal([]byte(raw), &exif); err == nil {
		photo.EXIF = &exif
	}
}

func encodePhotoEXIFJSON(photo *storage.Photo) string {
	if photo == nil || photo.EXIF == nil {
		return ""
	}
	data, err := json.Marshal(photo.EXIF)
	if err != nil {
		return ""
	}
	return string(data)
}

// UpdatePhotoEXIF 更新图片的 EXIF JSON。
func (s *DB) UpdatePhotoEXIF(id int64, userID int64, exif *storage.PhotoEXIF) error {
	result, err := s.db.Exec(`
		UPDATE photos
		SET exif_json = ?
		WHERE id = ? AND uploaded_by = ?`,
		encodePhotoEXIFJSON(&storage.Photo{EXIF: exif}), id, userID,
	)
	if err != nil {
		return fmt.Errorf("更新 EXIF 失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("图片不存在")
	}
	return nil
}

// UpdatePhotoSourceMedia 更新导入源媒体的路径与基础源信息。
func (s *DB) UpdatePhotoSourceMedia(id int64, userID int64, sourceRelPath string, originalName string, size int64, sourceModUnix int64) error {
	result, err := s.db.Exec(`
		UPDATE photos
		SET source_rel_path = ?, original_name = ?, size = ?, source_mod_unix = ?
		WHERE id = ? AND uploaded_by = ?`,
		sourceRelPath, originalName, size, sourceModUnix, id, userID,
	)
	if err != nil {
		return fmt.Errorf("更新源媒体路径失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("图片不存在")
	}
	return nil
}

// UpdatePhotoMediaPath 更新媒体文件路径与显示名称。
func (s *DB) UpdatePhotoMediaPath(id int64, userID int64, sourceRelPath string, storageRelPath string, originalName string, size int64, sourceModUnix int64) error {
	result, err := s.db.Exec(`
		UPDATE photos
		SET source_rel_path = ?, storage_rel_path = ?, original_name = ?, size = ?, source_mod_unix = ?
		WHERE id = ? AND uploaded_by = ?`,
		sourceRelPath, storageRelPath, originalName, size, sourceModUnix, id, userID,
	)
	if err != nil {
		return fmt.Errorf("更新媒体路径失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("图片不存在")
	}
	return nil
}

// UpdatePhotoCapturedMetadata 更新拍摄时间与基础媒体元数据。
func (s *DB) UpdatePhotoCapturedMetadata(id int64, userID int64, takenAt time.Time, exif *storage.PhotoEXIF, width int, height int, durationMS int64) error {
	return s.UpdatePhotoCapturedMetadataContext(context.Background(), id, userID, takenAt, exif, width, height, durationMS)
}

// UpdatePhotoCapturedMetadataContext updates captured metadata and allows long-running
// metadata backfills to stop waiting on a busy SQLite database after cancellation.
func (s *DB) UpdatePhotoCapturedMetadataContext(ctx context.Context, id int64, userID int64, takenAt time.Time, exif *storage.PhotoEXIF, width int, height int, durationMS int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE photos
		SET taken_at = ?, exif_json = ?, width = ?, height = ?, duration_ms = ?
		WHERE id = ? AND uploaded_by = ?`,
		takenAt, encodePhotoEXIFJSON(&storage.Photo{EXIF: exif}), width, height, durationMS, id, userID,
	)
	if err != nil {
		return fmt.Errorf("更新媒体元数据失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("图片不存在")
	}
	return nil
}

// SavePhoto 保存图片记录
func (s *DB) SavePhoto(photo *storage.Photo) error {
	if photo.MediaKind == "" {
		photo.MediaKind = storage.MediaKindImage
	}
	if photo.RandomSortKey <= 0 {
		photo.RandomSortKey = makeRandomSortKey(photo)
	}
	result, err := s.db.Exec(`
		INSERT INTO photos (uuid, original_name, media_kind, mime_type, size, width, height, duration_ms, storage_rel_path, source_rel_path, exif_json, source_mod_unix, random_sort_key, is_favorite, is_super_favorite, taken_at, uploaded_at, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		photo.UUID, photo.OriginalName, photo.MediaKind, photo.MimeType,
		photo.Size, photo.Width, photo.Height, photo.DurationMS, photo.StorageRelPath, photo.SourceRelPath, encodePhotoEXIFJSON(photo), photo.SourceModUnix, photo.RandomSortKey, photo.IsFavorite, photo.IsSuperFavorite,
		photo.TakenAt, photo.UploadedAt, photo.UploadedBy,
	)
	if err != nil {
		return fmt.Errorf("保存图片失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	photo.ID = id
	return nil
}

// GetPhotoByID 按 ID 查询图片
func (s *DB) GetPhotoByID(id int64, userID int64) (*storage.Photo, error) {
	row := s.db.QueryRow(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE id = ? AND uploaded_by = ? AND deleted_at IS NULL`, id, userID)
	p, err := scanPhoto(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil && p != nil {
		if attachErr := s.attachSharedPlaybackMetadata([]*storage.Photo{p}); attachErr != nil {
			return nil, attachErr
		}
	}
	return p, err
}

// GetPhotoByIDAny 按 ID 查询图片，包含已软删除
func (s *DB) GetPhotoByIDAny(id int64, userID int64) (*storage.Photo, error) {
	row := s.db.QueryRow(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE id = ? AND uploaded_by = ?`, id, userID)
	p, err := scanPhoto(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil && p != nil {
		if attachErr := s.attachSharedPlaybackMetadata([]*storage.Photo{p}); attachErr != nil {
			return nil, attachErr
		}
	}
	return p, err
}

// GetPhotoByUUID 按 UUID 查询图片（不含软删除）
func (s *DB) GetPhotoByUUID(uuid string, userID int64) (*storage.Photo, error) {
	row := s.db.QueryRow(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE uuid = ? AND uploaded_by = ? AND deleted_at IS NULL`, uuid, userID)
	p, err := scanPhoto(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil && p != nil {
		if attachErr := s.attachSharedPlaybackMetadata([]*storage.Photo{p}); attachErr != nil {
			return nil, attachErr
		}
	}
	return p, err
}

// GetPhotoByUUIDAny 按 UUID 查询图片，包含已软删除（用于文件服务回收站图片）
func (s *DB) GetPhotoByUUIDAny(uuid string, userID int64) (*storage.Photo, error) {
	row := s.db.QueryRow(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE uuid = ? AND uploaded_by = ?`, uuid, userID)
	p, err := scanPhoto(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil && p != nil {
		if attachErr := s.attachSharedPlaybackMetadata([]*storage.Photo{p}); attachErr != nil {
			return nil, attachErr
		}
	}
	return p, err
}

// GetPhotoBySourceRelPath 按导入源相对路径查询图片，包含已软删除。
func (s *DB) GetPhotoBySourceRelPath(sourceRelPath string, userID int64) (*storage.Photo, error) {
	row := s.db.QueryRow(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE source_rel_path = ? AND uploaded_by = ?`, sourceRelPath, userID)
	p, err := scanPhoto(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ListSourceMediaIndex 批量加载源文件索引。只读取启动扫描需要的轻量字段，避免大库启动时逐文件查库。
func (s *DB) ListSourceMediaIndex(userID int64) (map[string]storage.SourceMediaInfo, error) {
	rows, err := s.db.Query(`
		SELECT id, source_rel_path, size, source_mod_unix, deleted_at
		FROM photos
		WHERE uploaded_by = ? AND source_rel_path <> ''`, userID)
	if err != nil {
		return nil, fmt.Errorf("加载源媒体索引失败: %w", err)
	}
	defer rows.Close()

	index := make(map[string]storage.SourceMediaInfo)
	for rows.Next() {
		var item storage.SourceMediaInfo
		var deletedAt flexibleSQLiteTime
		var raw [5]any
		if err := rows.Scan(&raw[0], &raw[1], &raw[2], &raw[3], &raw[4]); err != nil {
			return nil, err
		}
		dest := []any{&item.ID, &item.SourceRelPath, &item.Size, &item.SourceModUnix, &deletedAt}
		var invalid error
		for i := range dest {
			if err := scanMediaColumn(dest[i], raw[i]); err != nil && invalid == nil {
				invalid = err
			}
		}
		if invalid != nil {
			log.Printf("Skipping source media index row %d: %v", item.ID, invalid)
			continue
		}
		item.Deleted = deletedAt.Present
		if deletedAt.Valid {
			item.DeletedAtUnixNano = deletedAt.Time.UnixNano()
		}
		index[item.SourceRelPath] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// ListPhotos 查询用户图片（时间线，游标分页）
func (s *DB) ListPhotos(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	return s.listPhotos(context.Background(), params, true)
}

// ListPhotosForMetadataBackfillContext loads the fields needed to repair media
// metadata, without the extra playback-preference lookups used by the timeline.
func (s *DB) ListPhotosForMetadataBackfillContext(ctx context.Context, userID, afterID int64, limit int, includeTotal bool) (*storage.PhotoPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 200
	}
	page := &storage.PhotoPage{Photos: make([]*storage.Photo, 0, limit)}
	if includeTotal {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM photos WHERE uploaded_by = ? AND deleted_at IS NULL`, userID).Scan(&page.Total); err != nil {
			return nil, fmt.Errorf("统计待修正媒体失败: %w", err)
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE uploaded_by = ? AND deleted_at IS NULL AND id > ?
		ORDER BY id ASC
		LIMIT ?`, userID, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("查询待修正媒体失败: %w", err)
	}
	defer rows.Close()
	collected, err := collectMediaPage(rows, limit, func(p *storage.Photo) string { return encodeCursor(p.TakenAt, p.ID) })
	if err != nil {
		return nil, err
	}
	collected.Total = page.Total
	return collected, nil
}

func (s *DB) listPhotos(ctx context.Context, params storage.ListPhotosParams, attachPlayback bool) (*storage.PhotoPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	where := "uploaded_by = ? AND deleted_at IS NULL"
	args := []interface{}{params.UserID}
	if params.OnlyFavorite {
		where += " AND is_favorite = 1"
	}
	if params.MediaKind != "" {
		where += " AND media_kind = ?"
		args = append(args, params.MediaKind)
	}

	var total int
	if !params.SkipTotal {
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM photos WHERE "+where, args...).Scan(&total); err != nil {
			return nil, fmt.Errorf("统计图片失败: %w", err)
		}
	}

	var rows *sql.Rows
	var err error

	if params.Reverse {
		if params.Cursor == "" {
			queryArgs := append([]interface{}{}, args...)
			queryArgs = append(queryArgs, limit+1)
			rows, err = s.db.QueryContext(ctx, `
				SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
				       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
				       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
				FROM photos
				WHERE `+where+`
				ORDER BY taken_at ASC, id ASC
				LIMIT ?`, queryArgs...)
		} else {
			c, err2 := decodeCursor(params.Cursor)
			if err2 != nil {
				return nil, err2
			}
			boundary, boundaryErr := s.mediaSortBoundary(ctx, c.ID, "taken_at", c.TakenAt)
			if boundaryErr != nil {
				return nil, boundaryErr
			}
			queryArgs := append([]interface{}{}, args...)
			queryArgs = append(queryArgs, boundary, boundary, c.ID, limit+1)
			rows, err = s.db.QueryContext(ctx, `
				SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
				       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
				       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
				FROM photos
				WHERE `+where+`
				  AND (taken_at > ? OR (taken_at = ? AND id > ?))
				ORDER BY taken_at ASC, id ASC
				LIMIT ?`, queryArgs...)
		}
	} else if params.Cursor == "" {
		queryArgs := append([]interface{}{}, args...)
		queryArgs = append(queryArgs, limit+1)
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE `+where+`
			ORDER BY taken_at DESC, id DESC
			LIMIT ?`, queryArgs...)
	} else {
		c, err2 := decodeCursor(params.Cursor)
		if err2 != nil {
			return nil, err2
		}
		boundary, boundaryErr := s.mediaSortBoundary(ctx, c.ID, "taken_at", c.TakenAt)
		if boundaryErr != nil {
			return nil, boundaryErr
		}
		queryArgs := append([]interface{}{}, args...)
		queryArgs = append(queryArgs, boundary, boundary, c.ID, limit+1)
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE `+where+`
			  AND (taken_at < ? OR (taken_at = ? AND id < ?))
			ORDER BY taken_at DESC, id DESC
			LIMIT ?`, queryArgs...)
	}
	if err != nil {
		return nil, fmt.Errorf("查询图片失败: %w", err)
	}
	defer rows.Close()

	page, err := collectPhotoPage(rows, limit, func(p *storage.Photo) time.Time {
		return p.TakenAt
	})
	if err != nil {
		return nil, err
	}
	if attachPlayback {
		if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
			return nil, err
		}
	}
	page.Total = total
	return page, nil
}

func (s *DB) LocateTimelineWindow(params storage.LocateTimelineParams) (*storage.TimelineLocateResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	row := s.db.QueryRow(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE id = ? AND uploaded_by = ? AND deleted_at IS NULL`,
		params.PhotoID, params.UserID,
	)
	target, err := scanPhoto(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询目标媒体失败: %w", err)
	}

	where := "uploaded_by = ? AND deleted_at IS NULL"
	baseArgs := []interface{}{params.UserID}
	if params.MediaKind != "" {
		where += " AND media_kind = ?"
		baseArgs = append(baseArgs, params.MediaKind)
	}
	missingBeforeCount, err := s.countTimelineBefore(where, baseArgs, target, params.Reverse)
	if err != nil {
		return nil, err
	}
	beforePage, afterPage, err := s.locateTimelinePages(where, baseArgs, target, limit, params.Reverse)
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
		MissingBeforeCount: max(0, missingBeforeCount-len(beforePage.Photos)),
	}
	if beforePage.BoundaryCursor != "" {
		result.PrevCursor = beforePage.BoundaryCursor
	} else {
		result.PrevCursor = encodeCursor(target.TakenAt, target.ID)
	}
	if afterPage.BoundaryCursor != "" {
		result.NextCursor = afterPage.BoundaryCursor
	} else {
		result.NextCursor = encodeCursor(target.TakenAt, target.ID)
	}
	return result, nil
}

func (s *DB) countTimelineBefore(where string, baseArgs []interface{}, target *storage.Photo, reverse bool) (int, error) {
	boundary, err := s.mediaSortBoundary(context.Background(), target.ID, "taken_at", target.TakenAt)
	if err != nil {
		return 0, err
	}
	queryArgs := append([]interface{}{}, baseArgs...)
	var condition string
	if reverse {
		condition = " AND (taken_at < ? OR (taken_at = ? AND id < ?))"
	} else {
		condition = " AND (taken_at > ? OR (taken_at = ? AND id > ?))"
	}
	queryArgs = append(queryArgs, boundary, boundary, target.ID)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM photos WHERE `+where+condition, queryArgs...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计目标之前媒体失败: %w", err)
	}
	return count, nil
}

func (s *DB) ListPhotosBefore(params storage.LocateTimelineParams) (*storage.PhotoPage, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	row := s.db.QueryRow(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE id = ? AND uploaded_by = ? AND deleted_at IS NULL`,
		params.PhotoID, params.UserID,
	)
	target, err := scanPhoto(row)
	if err == sql.ErrNoRows {
		return &storage.PhotoPage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询目标媒体失败: %w", err)
	}

	where := "uploaded_by = ? AND deleted_at IS NULL"
	baseArgs := []interface{}{params.UserID}
	if params.MediaKind != "" {
		where += " AND media_kind = ?"
		baseArgs = append(baseArgs, params.MediaKind)
	}
	beforePage, err := readPastInvalidMediaIDs(target, func(boundary *storage.Photo) (*storage.PhotoPage, error) {
		page, _, err := s.locateTimelinePages(where, baseArgs, boundary, limit, params.Reverse)
		return page, err
	})
	if err != nil {
		return nil, err
	}
	if err := s.attachSharedPlaybackMetadata(beforePage.Photos); err != nil {
		return nil, err
	}
	return beforePage, nil
}

func normalizeTimelineBeforePage(page *storage.PhotoPage) *storage.PhotoPage {
	if page == nil || len(page.Photos) <= 1 {
		return page
	}
	slices.Reverse(page.Photos)
	return page
}

func (s *DB) locateTimelinePages(where string, baseArgs []interface{}, target *storage.Photo, limit int, reverse bool) (*storage.PhotoPage, *storage.PhotoPage, error) {
	boundary, err := s.mediaSortBoundary(context.Background(), target.ID, "taken_at", target.TakenAt)
	if err != nil {
		return nil, nil, err
	}
	var beforeQuery string
	var afterQuery string
	var beforeArgs []interface{}
	var afterArgs []interface{}

	beforeArgs = append([]interface{}{}, baseArgs...)
	afterArgs = append([]interface{}{}, baseArgs...)

	if reverse {
		beforeArgs = append(beforeArgs, boundary, boundary, target.ID, limit+1)
		beforeQuery = `
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE ` + where + `
			  AND (taken_at < ? OR (taken_at = ? AND id < ?))
			ORDER BY taken_at DESC, id DESC
			LIMIT ?`

		afterArgs = append(afterArgs, boundary, boundary, target.ID, limit+1)
		afterQuery = `
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE ` + where + `
			  AND (taken_at > ? OR (taken_at = ? AND id > ?))
			ORDER BY taken_at ASC, id ASC
			LIMIT ?`
	} else {
		beforeArgs = append(beforeArgs, boundary, boundary, target.ID, limit+1)
		beforeQuery = `
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE ` + where + `
			  AND (taken_at > ? OR (taken_at = ? AND id > ?))
			ORDER BY taken_at ASC, id ASC
			LIMIT ?`

		afterArgs = append(afterArgs, boundary, boundary, target.ID, limit+1)
		afterQuery = `
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE ` + where + `
			  AND (taken_at < ? OR (taken_at = ? AND id < ?))
			ORDER BY taken_at DESC, id DESC
			LIMIT ?`
	}

	beforeRows, err := s.db.Query(beforeQuery, beforeArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("查询目标之前媒体失败: %w", err)
	}
	beforePage, err := collectPhotoPage(beforeRows, limit, func(p *storage.Photo) time.Time { return p.TakenAt })
	beforeRows.Close()
	if err != nil {
		return nil, nil, err
	}
	beforePage = normalizeTimelineBeforePage(beforePage)

	afterRows, err := s.db.Query(afterQuery, afterArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("查询目标之后媒体失败: %w", err)
	}
	afterPage, err := collectPhotoPage(afterRows, limit, func(p *storage.Photo) time.Time { return p.TakenAt })
	afterRows.Close()
	if err != nil {
		return nil, nil, err
	}

	return beforePage, afterPage, nil
}

// SearchPhotos 搜索用户媒体（时间倒序，游标分页）。
func (s *DB) SearchPhotos(params storage.SearchPhotosParams) (*storage.PhotoPage, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	query := strings.TrimSpace(params.Query)
	if query == "" {
		return &storage.PhotoPage{}, nil
	}
	pattern := "%" + escapeLikePattern(query) + "%"

	where := `uploaded_by = ? AND deleted_at IS NULL AND (
		original_name LIKE ? ESCAPE '\' OR
		uuid LIKE ? ESCAPE '\' OR
		mime_type LIKE ? ESCAPE '\' OR
		media_kind LIKE ? ESCAPE '\'
	)`
	args := []interface{}{params.UserID, pattern, pattern, pattern, pattern}
	if canUsePhotoSearchIndex(query) {
		where = `uploaded_by = ? AND deleted_at IS NULL AND id IN (
			SELECT rowid FROM photos_search WHERE photos_search MATCH ?
		)`
		args = []interface{}{params.UserID, photoSearchMatch(query)}
	}
	if params.MediaKind != "" {
		where += " AND media_kind = ?"
		args = append(args, params.MediaKind)
	}
	if params.OnlyFavorite {
		where += " AND is_favorite = 1"
	}

	var total int
	if params.IncludeTotal {
		countQuery := "SELECT COUNT(*) FROM photos WHERE " + where
		if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
			return nil, fmt.Errorf("统计搜索结果失败: %w", err)
		}
	}

	if params.Cursor != "" {
		c, err := decodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		boundary, boundaryErr := s.mediaSortBoundary(context.Background(), c.ID, "taken_at", c.TakenAt)
		if boundaryErr != nil {
			return nil, boundaryErr
		}
		where += " AND (taken_at < ? OR (taken_at = ? AND id < ?))"
		args = append(args, boundary, boundary, c.ID)
	}
	args = append(args, limit+1)

	rows, err := s.db.Query(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE `+where+`
		ORDER BY taken_at DESC, id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("搜索媒体失败: %w", err)
	}
	defer rows.Close()

	page, err := collectPhotoPage(rows, limit, func(p *storage.Photo) time.Time {
		return p.TakenAt
	})
	if err != nil {
		return nil, err
	}
	if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
		return nil, err
	}
	if params.IncludeTotal {
		page.Total = total
	}
	return page, nil
}

// ListRandomPhotos 查询乱序相册媒体。通过持久 random_sort_key + seed 旋转顺序，避免前端一次性加载全库再排序。
func (s *DB) ListRandomPhotos(params storage.RandomPhotosParams) (*storage.PhotoPage, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	seed := normalizeRandomSeed(params.Seed)
	state := randomCursor{Key: seed, Wrapped: false}
	if params.Cursor != "" {
		c, err := decodeRandomCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		state = *c
	}

	baseWhere := "uploaded_by = ? AND deleted_at IS NULL"
	baseArgs := []interface{}{params.UserID}
	if params.MediaKind != "" {
		baseWhere += " AND media_kind = ?"
		baseArgs = append(baseArgs, params.MediaKind)
	}
	if params.OnlyFavorite {
		baseWhere += " AND is_favorite = 1"
	}
	if len(params.AlbumIDs) > 0 {
		// Reuse authorized album predicates; IN deduplicates overlapping albums.
		scopes := make([]string, 0, len(params.AlbumIDs))
		for _, id := range params.AlbumIDs {
			where, join, args, err := s.buildAlbumPhotoScope(id, params.UserID, params.MediaKind)
			if err != nil {
				return nil, err
			}
			if where == "" {
				continue
			}
			scopes = append(scopes, "SELECT p.id FROM photos p "+join+" WHERE "+where)
			baseArgs = append(baseArgs, args...)
		}
		if len(scopes) == 0 {
			return &storage.PhotoPage{}, nil
		}
		baseWhere += " AND id IN (" + strings.Join(scopes, " UNION ") + ")"
	}
	var total int
	if !params.SkipTotal {
		if err := s.db.QueryRow("SELECT COUNT(*) FROM photos WHERE "+baseWhere, baseArgs...).Scan(&total); err != nil {
			return nil, fmt.Errorf("统计乱序相册失败: %w", err)
		}
	}

	type randomPhoto struct {
		photo   *storage.Photo
		key     int64
		wrapped bool
		invalid bool
	}
	items := make([]randomPhoto, 0, limit+1)
	appendPhase := func(wrapped bool, afterKey, afterID int64, maxKey int64, remaining int) error {
		if remaining <= 0 {
			return nil
		}
		where := baseWhere + ` AND random_sort_key <= ?
			  AND (random_sort_key > ? OR (random_sort_key = ? AND id > ?))`
		args := append([]interface{}{}, baseArgs...)
		args = append(args, maxKey, afterKey, afterKey, afterID)
		args = append(args, remaining)
		rows, err := s.db.Query(`
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by, random_sort_key
			FROM photos
			WHERE `+where+`
			ORDER BY random_sort_key ASC, id ASC
			LIMIT ?`, args...)
		if err != nil {
			return fmt.Errorf("查询乱序相册失败: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var key int64
			p, err := scanPhotoWithExtraInt64(rows, &key)
			if err != nil && !skipInvalidMediaRow(err) {
				return err
			}
			items = append(items, randomPhoto{photo: p, key: key, wrapped: wrapped, invalid: err != nil})
		}
		return rows.Err()
	}

	if state.Wrapped {
		if err := appendPhase(true, state.Key, state.ID, seed-1, limit+1); err != nil {
			return nil, err
		}
	} else {
		if err := appendPhase(false, state.Key, state.ID, randomSortKeyMax, limit+1); err != nil {
			return nil, err
		}
		if len(items) <= limit {
			if err := appendPhase(true, 0, 0, seed-1, limit+1-len(items)); err != nil {
				return nil, err
			}
		}
	}

	page := &storage.PhotoPage{Total: total}
	if len(items) > limit {
		page.HasMore = true
		items = items[:limit]
		last := items[len(items)-1]
		page.NextCursor = encodeRandomCursor(last.key, last.photo.ID, last.wrapped)
	}
	page.Photos = make([]*storage.Photo, 0, len(items))
	for _, item := range items {
		if item.invalid {
			page.Skipped++
			continue
		}
		page.Photos = append(page.Photos, item.photo)
	}
	if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
		return nil, err
	}
	return page, nil
}

// ListTrashedPhotos 查询回收站图片
func (s *DB) ListTrashedPhotos(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	// Folder deletions are represented by their folder batch in the recycle
	// bin. Keep their media out of the ordinary media stream so the same files
	// are not shown once per photo and once per folder batch.
	where := "uploaded_by = ? AND deleted_at IS NOT NULL AND deleted_group_id = ''"
	args := []interface{}{params.UserID}
	if params.MediaKind != "" {
		where += " AND media_kind = ?"
		args = append(args, params.MediaKind)
	}

	var rows *sql.Rows
	var err error

	if params.Cursor == "" {
		queryArgs := append([]interface{}{}, args...)
		queryArgs = append(queryArgs, limit+1)
		rows, err = s.db.Query(`
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE `+where+`
			ORDER BY deleted_at DESC, id DESC
			LIMIT ?`, queryArgs...)
	} else {
		c, err2 := decodeCursor(params.Cursor)
		if err2 != nil {
			return nil, err2
		}
		boundary, boundaryErr := s.mediaSortBoundary(context.Background(), c.ID, "deleted_at", c.TakenAt)
		if boundaryErr != nil {
			return nil, boundaryErr
		}
		queryArgs := append([]interface{}{}, args...)
		queryArgs = append(queryArgs, boundary, boundary, c.ID, limit+1)
		rows, err = s.db.Query(`
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE `+where+`
			  AND (deleted_at < ? OR (deleted_at = ? AND id < ?))
			ORDER BY deleted_at DESC, id DESC
			LIMIT ?`, queryArgs...)
	}
	if err != nil {
		return nil, fmt.Errorf("查询回收站失败: %w", err)
	}
	defer rows.Close()

	page, err := collectPhotoPage(rows, limit, func(p *storage.Photo) time.Time {
		if p.DeletedAt != nil {
			return *p.DeletedAt
		}
		return time.Time{}
	})
	if err != nil {
		return nil, err
	}
	if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
		return nil, err
	}
	return page, nil
}

// ListFavoritePhotos 查询个人收藏
func (s *DB) ListFavoritePhotos(params storage.ListPhotosParams) (*storage.PhotoPage, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	where := "uploaded_by = ? AND deleted_at IS NULL AND is_favorite = 1"
	args := []interface{}{params.UserID}
	if params.MediaKind != "" {
		where += " AND media_kind = ?"
		args = append(args, params.MediaKind)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM photos WHERE "+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("统计个人收藏失败: %w", err)
	}

	var rows *sql.Rows
	var err error

	if params.Cursor == "" {
		queryArgs := append([]interface{}{}, args...)
		queryArgs = append(queryArgs, limit+1)
		rows, err = s.db.Query(`
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE `+where+`
			ORDER BY is_super_favorite DESC, taken_at DESC, id DESC
			LIMIT ?`, queryArgs...)
	} else {
		c, err2 := decodeFavoriteCursor(params.Cursor)
		if err2 != nil {
			return nil, err2
		}
		boundary, boundaryErr := s.mediaSortBoundary(context.Background(), c.ID, "taken_at", c.TakenAt)
		if boundaryErr != nil {
			return nil, boundaryErr
		}
		queryArgs := append([]interface{}{}, args...)
		superValue := 0
		if c.Super {
			superValue = 1
		}
		superBoundary, err := s.mediaSortBoundary(context.Background(), c.ID, "is_super_favorite", superValue)
		if err != nil {
			return nil, err
		}
		queryArgs = append(queryArgs, superBoundary, superBoundary, boundary, superBoundary, boundary, c.ID, limit+1)
		rows, err = s.db.Query(`
			SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
			       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
			       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
			FROM photos
			WHERE `+where+`
			  AND (
			    is_super_favorite < ?
			    OR (is_super_favorite = ? AND taken_at < ?)
			    OR (is_super_favorite = ? AND taken_at = ? AND id < ?)
			  )
			ORDER BY is_super_favorite DESC, taken_at DESC, id DESC
			LIMIT ?`, queryArgs...)
	}
	if err != nil {
		return nil, fmt.Errorf("查询个人收藏失败: %w", err)
	}
	defer rows.Close()

	page, err := collectFavoritePhotoPage(rows, limit)
	if err != nil {
		return nil, err
	}
	if err := s.attachSharedPlaybackMetadata(page.Photos); err != nil {
		return nil, err
	}
	page.Total = total
	return page, nil
}

// Backward browsing anchors on a visible media ID. A whole skipped window
// must be crossed here, since it has no visible ID for the browser to anchor on.
func readPastInvalidMediaIDs(target *storage.Photo, fetch func(*storage.Photo) (*storage.PhotoPage, error)) (*storage.PhotoPage, error) {
	skipped := 0
	for {
		page, err := fetch(target)
		if err != nil {
			return nil, err
		}
		skipped += page.Skipped
		if len(page.Photos) > 0 || !page.HasMore || page.Skipped == 0 {
			page.Skipped = skipped
			return page, nil
		}
		if page.LastID <= 0 || page.LastID == target.ID {
			return nil, fmt.Errorf("media window did not advance")
		}
		target = &storage.Photo{ID: page.LastID}
	}
}

// collectMediaPage keeps continuation based on rows consumed, including skipped
// rows. The lookahead row is not consumed, so it is retried on the next page.
func collectMediaPage(rows *sql.Rows, limit int, cursorFor func(*storage.Photo) string) (*storage.PhotoPage, error) {
	defer rows.Close()
	page := &storage.PhotoPage{Photos: make([]*storage.Photo, 0, limit)}
	var boundary *storage.Photo
	for count := 0; rows.Next(); count++ {
		if count == limit {
			page.HasMore = true
			break
		}
		photo, err := scanPhoto(rows)
		if err != nil && !skipInvalidMediaRow(err) {
			return nil, err
		}
		boundary = photo
		page.LastID = photo.ID
		if err == nil {
			page.Photos = append(page.Photos, photo)
		} else {
			page.Skipped++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if boundary != nil {
		page.BoundaryCursor = cursorFor(boundary)
		if page.HasMore {
			page.NextCursor = page.BoundaryCursor
		}
	}
	return page, nil
}

func collectFavoritePhotoPage(rows *sql.Rows, limit int) (*storage.PhotoPage, error) {
	return collectMediaPage(rows, limit, func(p *storage.Photo) string { return encodeFavoriteCursor(p.IsSuperFavorite, p.TakenAt, p.ID) })
}

func collectPhotoPage(rows *sql.Rows, limit int, cursorTimeFn func(*storage.Photo) time.Time) (*storage.PhotoPage, error) {
	return collectMediaPage(rows, limit, func(p *storage.Photo) string { return encodeCursor(cursorTimeFn(p), p.ID) })
}

// SoftDeletePhoto 软删除图片
func (s *DB) SoftDeletePhoto(id int64, userID int64, deletedBy int64) error {
	result, err := s.db.Exec(`
		UPDATE photos SET deleted_at = ?, deleted_by = ?, deleted_group_id = ''
		WHERE id = ? AND uploaded_by = ? AND deleted_at IS NULL`,
		time.Now(), deletedBy, id, userID)
	if err != nil {
		return fmt.Errorf("删除图片失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("图片不存在或已删除")
	}
	return nil
}

// RestorePhoto 从回收站恢复图片
func (s *DB) RestorePhoto(id int64, userID int64) error {
	result, err := s.db.Exec(`
		UPDATE photos SET deleted_at = NULL, deleted_by = NULL, deleted_group_id = ''
		WHERE id = ? AND uploaded_by = ? AND deleted_at IS NOT NULL`,
		id, userID)
	if err != nil {
		return fmt.Errorf("恢复图片失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("图片不在回收站中")
	}
	return nil
}

// HardDeletePhoto 彻底删除单张图片记录
func (s *DB) HardDeletePhoto(id int64, userID int64) error {
	_, err := s.db.Exec(`DELETE FROM photos WHERE id = ? AND uploaded_by = ?`, id, userID)
	return err
}

// HardDeleteTrashedPhotos 清空回收站，返回需要逐文件移动的普通媒体记录。
// 文件夹删除批次的源目录由 PhotoService 按整棵目录树处理，但其媒体记录
// 仍在这里一并删除。
func (s *DB) HardDeleteTrashedPhotos(userID int64) ([]*storage.Photo, error) {
	rows, err := s.db.Query(`
		SELECT id, uuid, original_name, media_kind, mime_type, size, width, height, duration_ms,
		       storage_rel_path, source_rel_path, exif_json, is_favorite, is_super_favorite,
		       taken_at, uploaded_at, uploaded_by, deleted_at, deleted_by
		FROM photos
		WHERE uploaded_by = ? AND deleted_at IS NOT NULL AND deleted_group_id = ''`, userID)
	if err != nil {
		return nil, err
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

	if _, err := s.db.Exec(`
		DELETE FROM photos WHERE uploaded_by = ? AND deleted_at IS NOT NULL`, userID); err != nil {
		return nil, fmt.Errorf("清空回收站失败: %w", err)
	}

	return photos, nil
}

// SetPhotoFavorite 设置收藏状态
func (s *DB) SetPhotoFavorite(id int64, userID int64, favorite bool, superFavorite bool) error {
	value := 0
	if favorite {
		value = 1
	}
	superValue := 0
	if superFavorite {
		value = 1
		superValue = 1
	}
	result, err := s.db.Exec(`
		UPDATE photos
		SET is_favorite = ?, is_super_favorite = ?
		WHERE id = ? AND uploaded_by = ? AND deleted_at IS NULL`, value, superValue, id, userID)
	if err != nil {
		return fmt.Errorf("更新收藏状态失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("照片/视频不存在")
	}
	return nil
}
