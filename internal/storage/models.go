package storage

import "time"

// Photo 图片模型
type Photo struct {
	ID                 int64      `json:"id"`
	UUID               string     `json:"uuid"`          // 对应磁盘文件名（不含扩展名）
	OriginalName       string     `json:"original_name"` // 用户上传时的原始文件名
	MediaKind          string     `json:"media_kind"`    // image 或 video
	MimeType           string     `json:"mime_type"`     // image/jpeg 等
	Size               int64      `json:"size"`          // 文件大小（字节）
	Width              int        `json:"width"`
	Height             int        `json:"height"`
	DurationMS         int64      `json:"duration_ms"`
	IsFavorite         bool       `json:"is_favorite"`
	IsSuperFavorite    bool       `json:"is_super_favorite"`
	ThumbnailURL       string     `json:"thumbnail_url,omitempty"`
	PosterURL          string     `json:"poster_url,omitempty"`
	ThumbnailReady     bool       `json:"thumbnail_ready,omitempty"`
	PosterReady        bool       `json:"poster_ready,omitempty"`
	DominantColor      string     `json:"dominant_color,omitempty"`
	StorageRelPath     string     `json:"-"` // 应用内部实际存储的相对路径
	SourceRelPath      string     `json:"-"` // 启动扫描导入时源文件的相对路径，用于避免重复导入
	EXIF               *PhotoEXIF `json:"exif,omitempty"`
	SourceModUnix      int64      `json:"-"`        // 源文件修改时间，用于大库启动时快速判断是否可跳过
	RandomSortKey      int64      `json:"-"`        // 乱序相册使用的持久随机键，避免前端一次性打乱全库
	TakenAt            time.Time  `json:"taken_at"` // ���摄时间（EXIF 或文件创建时间）
	UploadedAt         time.Time  `json:"uploaded_at"`
	UploadedBy         int64      `json:"uploaded_by"` // 关联 users.id
	DeletedAt          *time.Time `json:"deleted_at"`  // nil 表示未删除
	DeletedBy          *int64     `json:"deleted_by"`  // nil 表示未删除
	DeletedGroupID     string     `json:"-"`           // 文件夹相册批量删除时的回收站批次
	VideoBookmarkCount int        `json:"video_bookmark_count,omitempty"`
	VideoResumeTime    int64      `json:"video_resume_time,omitempty"`
}

type LegacyUUIDMatch struct {
	SourceRelPath string
	OldUUID       string
	NewUUID       string
}

type LegacyUUIDReport struct {
	Matches      []LegacyUUIDMatch
	SkippedPaths []string
}

// PhotoEXIF 保存可展示的 EXIF 字段。
type PhotoEXIF struct {
	Make           string    `json:"make,omitempty"`
	Model          string    `json:"model,omitempty"`
	Orientation    int       `json:"orientation,omitempty"`
	TakenAt        time.Time `json:"taken_at,omitempty"`
	Width          int       `json:"width,omitempty"`
	Height         int       `json:"height,omitempty"`
	FNumber        string    `json:"f_number,omitempty"`
	ExposureTime   string    `json:"exposure_time,omitempty"`
	ISOSpeed       int       `json:"iso_speed,omitempty"`
	FocalLength    string    `json:"focal_length,omitempty"`
	Latitude       float64   `json:"latitude,omitempty"`
	Longitude      float64   `json:"longitude,omitempty"`
	HasGPS         bool      `json:"has_gps,omitempty"`
	Location       string    `json:"location_address,omitempty"`
	VideoCodec     string    `json:"video_codec,omitempty"`
	VideoFrameRate float64   `json:"video_frame_rate,omitempty"`
}

const (
	MediaKindImage = "image"
	MediaKindVideo = "video"
)

// Album 相册模型
type Album struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	CoverPhotoID   *int64     `json:"cover_photo_id"` // nil 时自动取最新图片
	CoverUUID      string     `json:"cover_uuid"`     // 封面图片 UUID，查询时填充，前端用于显示缩略图
	SourceKind     string     `json:"source_kind,omitempty"`
	SourceRelPath  string     `json:"source_rel_path,omitempty"`
	CreatedBy      int64      `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	PhotoCount     int        `json:"photo_count"` // 非数据库字段，查询时聚合
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
	DeletedBy      *int64     `json:"deleted_by,omitempty"`
	DeletedGroupID string     `json:"-"`
}

// FolderAlbumTrashEntry 表示一次文件夹相册删除操作在回收站中的聚合条目。
// 同一目录树下的父文件夹、子文件夹和媒体共用一个 GroupID，避免回收站重复展示。
type FolderAlbumTrashEntry struct {
	GroupID       string    `json:"group_id"`
	AlbumID       int64     `json:"album_id"`
	Name          string    `json:"name"`
	SourceRelPath string    `json:"source_rel_path,omitempty"`
	CoverUUID     string    `json:"cover_uuid,omitempty"`
	DeletedAt     time.Time `json:"deleted_at"`
	PhotoCount    int       `json:"photo_count"`
	FolderCount   int       `json:"folder_count"`
}

// VideoPlaybackPreference 保存单个视频的播放偏好。
type VideoPlaybackPreference struct {
	PhotoID    int64                   `json:"photo_id"`
	Volume     float64                 `json:"volume"`
	Muted      bool                    `json:"muted"`
	ResumeTime int64                   `json:"resume_time,omitempty"`
	Bookmarks  []VideoPlaybackBookmark `json:"bookmarks,omitempty"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

type VideoPlaybackBookmark struct {
	Slot int    `json:"slot"`
	Time int64  `json:"time"`
	Name string `json:"name,omitempty"`
}

// AlbumPhoto 相册与图片的关联（多对多）
type AlbumPhoto struct {
	AlbumID int64     `json:"album_id"`
	PhotoID int64     `json:"photo_id"`
	AddedAt time.Time `json:"added_at"`
}

// ShareLink 分享链接模型
type ShareLink struct {
	ID        int64      `json:"id"`
	Token     string     `json:"token"`     // 随机生成的访问 token
	Type      string     `json:"type"`      // "photo" 或 "album"
	TargetID  int64      `json:"target_id"` // 图片ID 或 相册ID
	CreatedBy int64      `json:"created_by"`
	ExpiresAt *time.Time `json:"expires_at"` // nil 表示永不过期
	CreatedAt time.Time  `json:"created_at"`
}

// ShareType 分享类型常量
const (
	ShareTypePhoto = "photo"
	ShareTypeAlbum = "album"
)

// PhotoPage 图片分页结果（游标分页）
type PhotoPage struct {
	Photos     []*Photo `json:"photos"`
	NextCursor string   `json:"next_cursor"` // 空字符串表示没有更多
	HasMore    bool     `json:"has_more"`
	Total      int      `json:"total,omitempty"`
}

type TimelineLocateResult struct {
	Photos             []*Photo `json:"photos"`
	TargetIndex        int      `json:"target_index"`
	HasBefore          bool     `json:"has_before"`
	HasAfter           bool     `json:"has_after"`
	MissingBeforeCount int      `json:"missing_before_count,omitempty"`
	PrevCursor         string   `json:"prev_cursor,omitempty"`
	NextCursor         string   `json:"next_cursor,omitempty"`
}

// LibraryScanSnapshot 保存一次成功扫描后的轻量状态，用于资源库目录树未变化时快速跳过全量扫描。
type LibraryScanSnapshot struct {
	RootPath             string            `json:"root_path"`
	RootModUnixNano      int64             `json:"root_mod_unix_nano"`
	DirectoryModTimes    map[string]int64  `json:"directory_mod_times,omitempty"`
	DirectoryEntryHashes map[string]string `json:"directory_entry_hashes,omitempty"`
	DirectoryCount       int               `json:"directory_count,omitempty"`
	DirectoryModHash     string            `json:"directory_mod_hash,omitempty"`
	MediaFileHash        string            `json:"media_file_hash,omitempty"`
	FileCount            int               `json:"file_count"`
	CompletedAt          time.Time         `json:"completed_at"`
}

// ListPhotosParams 查询图片参数
type ListPhotosParams struct {
	UserID       int64  // 必填，用户隔离
	Cursor       string // 游标，空表示从头开始
	Limit        int    // 每页数量，默认30
	Reverse      bool   // true 时按拍摄时间正序（旧到新）
	OnlyTrashed  bool   // true 时查询回收站
	OnlyFavorite bool   // true 时仅查询个人收藏
	SkipTotal    bool   // true 时跳过 COUNT(*)，用于后续分页降低大库滚动成本
	MediaKind    string // image / video，空表示全部
}

type LocateTimelineParams struct {
	UserID    int64
	PhotoID   int64
	Limit     int
	Reverse   bool
	MediaKind string
}

type LocateAlbumParams struct {
	AlbumID   int64
	UserID    int64
	PhotoID   int64
	Limit     int
	MediaKind string
	Sort      string
}

// SearchPhotosParams 查询媒体搜索结果参数。
type SearchPhotosParams struct {
	UserID       int64
	Query        string
	Cursor       string
	Limit        int
	MediaKind    string
	OnlyFavorite bool
	IncludeTotal bool
}

// RandomPhotosParams 查询乱序相册媒体参数。
type RandomPhotosParams struct {
	UserID    int64
	Seed      int64
	Cursor    string
	Limit     int
	SkipTotal bool
	MediaKind string
}

// SourceMediaInfo 是启动扫描使用的轻量索引，避免为每个源文件做一次完整查询。
type SourceMediaInfo struct {
	ID            int64
	SourceRelPath string
	Size          int64
	SourceModUnix int64
}

// ListAlbumPhotosParams 查询相册内图片参数
type ListAlbumPhotosParams struct {
	AlbumID   int64
	UserID    int64
	Cursor    string
	Limit     int
	MediaKind string
	Sort      string
}
