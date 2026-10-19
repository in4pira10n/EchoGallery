package danmaku

import "time"

type Page struct {
	Page     int    `json:"page"`
	CID      int64  `json:"cid"`
	Part     string `json:"part"`
	Duration int    `json:"duration"`
	File     string `json:"file"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type Video struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Owner       string `json:"owner"`
	OwnerAvatar string `json:"owner_avatar,omitempty"`
	Cover       string `json:"cover,omitempty"`
	URL         string `json:"source_url"`
	Pages       []Page `json:"pages"`
}

type Metadata struct {
	Video
	MediaID      int64     `json:"media_id,omitempty"`
	MediaUUID    string    `json:"media_uuid,omitempty"`
	MatchedPage  int       `json:"matched_page,omitempty"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type Task struct {
	ID          string    `json:"task_id"`
	Status      string    `json:"status"`
	BVID        string    `json:"bvid,omitempty"`
	Title       string    `json:"title,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	CurrentPage int       `json:"current_page,omitempty"`
	TotalPages  int       `json:"total_pages,omitempty"`
	Completed   int       `json:"completed"`
	Failed      int       `json:"failed"`
	Skipped     int       `json:"skipped"`
	Message     string    `json:"message,omitempty"`
	ErrorKey    string    `json:"error_key,omitempty"`
	MessageKey  string    `json:"message_key,omitempty"`
	Error       string    `json:"error,omitempty"`
	MediaID     int64     `json:"media_id,omitempty"`
	MediaUUID   string    `json:"media_uuid,omitempty"`
	StoragePath string    `json:"storage_path,omitempty"`
	Pages       []Page    `json:"pages,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
}

const (
	StatusQueued      = "queued"
	StatusResolving   = "resolving"
	StatusDownloading = "downloading"
	StatusCompleted   = "completed"
	StatusPartial     = "partial"
	StatusFailed      = "failed"
	StatusCancelled   = "cancelled"
)
