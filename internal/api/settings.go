package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"

	"echogallery/internal/config"
	imgpkg "echogallery/internal/image"
	"echogallery/internal/service"
	"echogallery/internal/storage"
	"echogallery/internal/update"
)

type settingsResponse struct {
	CurrentUsername               string            `json:"current_username,omitempty"`
	Role                          string            `json:"role"`
	CanWrite                      bool              `json:"can_write"`
	CanAdmin                      bool              `json:"can_admin"`
	CanRoot                       bool              `json:"can_root"`
	Port                          int               `json:"port"`
	ActiveLibraryID               string            `json:"active_library_id,omitempty"`
	StoragePath                   string            `json:"storage_path"`
	Libraries                     []libraryResponse `json:"libraries"`
	ThumbnailDir                  string            `json:"thumbnail_dir"`
	ThumbnailSize                 int               `json:"thumbnail_size"`
	TrashDir                      string            `json:"trash_dir"`
	UseSystemPlayer               bool              `json:"use_system_player"`
	JWTSecretMasked               string            `json:"jwt_secret_masked"`
	Users                         []string          `json:"users"`
	Theme                         string            `json:"theme"`
	GridSize                      int               `json:"grid_size"`
	GridGap                       int               `json:"grid_gap"`
	ThumbRadius                   int               `json:"thumb_radius"`
	SidebarAutoHide               bool              `json:"sidebar_auto_hide"`
	SlideshowMode                 string            `json:"slideshow_mode"`
	SlideshowLoop                 bool              `json:"slideshow_loop"`
	SlideshowInterval             int               `json:"slideshow_interval"`
	LightboxZoom                  int               `json:"lightbox_zoom"`
	ExperimentalAutoplayVideo     bool              `json:"experimental_autoplay_video"`
	VideoAutoplayNext             bool              `json:"video_autoplay_next"`
	VideoSectionMinMinutes        int               `json:"video_section_min_minutes"`
	ExperimentalPrefetchNeighbors bool              `json:"experimental_prefetch_neighbors"`
	ExperimentalRestoreLastView   bool              `json:"experimental_restore_last_view"`
	ContinueLastVideoPosition     bool              `json:"continue_last_video_position"`
	WarmEnabled                   bool              `json:"warm_enabled"`
	ThrottledVideoSeek            bool              `json:"throttled_video_seek"`
	VideoSeekThrottleMS           int               `json:"video_seek_throttle_ms"`
	VideoVolumeSwipeSensitivity   int               `json:"video_volume_swipe_sensitivity"`
	VideoVolumeMinPercent         int               `json:"video_volume_min_percent"`
	VideoVolumeMaxPercent         int               `json:"video_volume_max_percent"`
	LowResourceMode               bool              `json:"low_resource_mode"`
	PlayerKeymap                  string            `json:"player_keymap"`
	PWAIconURL                    string            `json:"pwa_icon_url,omitempty"`
	LocalUpdateConfigText         string            `json:"local_update_config_text,omitempty"`
}

type settingsUpdateRequest struct {
	Port                          int              `json:"port"`
	ActiveLibraryID               string           `json:"active_library_id,omitempty"`
	StoragePath                   string           `json:"storage_path"`
	Libraries                     []config.Library `json:"libraries"`
	ThumbnailDir                  string           `json:"thumbnail_dir"`
	ThumbnailSize                 int              `json:"thumbnail_size"`
	TrashDir                      string           `json:"trash_dir"`
	UseSystemPlayer               bool             `json:"use_system_player"`
	Theme                         string           `json:"theme"`
	GridSize                      int              `json:"grid_size"`
	GridGap                       int              `json:"grid_gap"`
	ThumbRadius                   int              `json:"thumb_radius"`
	SidebarAutoHide               bool             `json:"sidebar_auto_hide"`
	SlideshowMode                 string           `json:"slideshow_mode"`
	SlideshowLoop                 bool             `json:"slideshow_loop"`
	SlideshowInterval             int              `json:"slideshow_interval"`
	LightboxZoom                  int              `json:"lightbox_zoom"`
	ExperimentalAutoplayVideo     bool             `json:"experimental_autoplay_video"`
	VideoAutoplayNext             bool             `json:"video_autoplay_next"`
	VideoSectionMinMinutes        int              `json:"video_section_min_minutes"`
	ExperimentalPrefetchNeighbors bool             `json:"experimental_prefetch_neighbors"`
	ExperimentalRestoreLastView   bool             `json:"experimental_restore_last_view"`
	ContinueLastVideoPosition     bool             `json:"continue_last_video_position"`
	WarmEnabled                   bool             `json:"warm_enabled"`
	ThrottledVideoSeek            bool             `json:"throttled_video_seek"`
	VideoSeekThrottleMS           int              `json:"video_seek_throttle_ms"`
	VideoVolumeSwipeSensitivity   int              `json:"video_volume_swipe_sensitivity"`
	VideoVolumeMinPercent         int              `json:"video_volume_min_percent"`
	VideoVolumeMaxPercent         int              `json:"video_volume_max_percent"`
	LowResourceMode               bool             `json:"low_resource_mode"`
	PlayerKeymap                  string           `json:"player_keymap"`
}

type refreshLibraryLogosRequest struct {
	ReplaceExisting bool `json:"replace_existing"`
}

type refreshLibraryLogosResult struct {
	Scanned int      `json:"scanned"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

type libraryResponse struct {
	Index                 int    `json:"index"`
	ID                    string `json:"id,omitempty"`
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	LogoAsset             string `json:"logo_asset,omitempty"`
	LogoImageURL          string `json:"logo_image_url,omitempty"`
	AccentColor           string `json:"accent_color,omitempty"`
	Status                string `json:"status,omitempty"`
	Available             bool   `json:"available"`
	UnavailableReason     string `json:"unavailable_reason,omitempty"`
	LockedByUsername      string `json:"locked_by_username,omitempty"`
	CreatedAt             string `json:"created_at,omitempty"`
	LastScannedAt         string `json:"last_scanned_at,omitempty"`
	UnsupportedMediaCount int64  `json:"unsupported_media_count,omitempty"`
	TotalSize             int64  `json:"total_size,omitempty"`
	TotalSizeText         string `json:"total_size_text,omitempty"`
	PhotoCount            int64  `json:"photo_count,omitempty"`
	VideoCount            int64  `json:"video_count,omitempty"`
}

type LibraryAvailabilityState struct {
	Available     bool   `json:"available"`
	Status        string `json:"status,omitempty"`
	Reason        string `json:"reason,omitempty"`
	OwnerUsername string `json:"owner_username,omitempty"`
	Scope         string `json:"scope,omitempty"`
}

type userLibraryAccessResponse struct {
	Username          string   `json:"username"`
	Role              string   `json:"role"`
	AllowedLibraryIDs []string `json:"allowed_library_ids,omitempty"`
	DefaultLibraryID  string   `json:"default_library_id,omitempty"`
	CurrentLibraryID  string   `json:"current_library_id,omitempty"`
}

type userLibraryAccessUpdateRequest struct {
	Username          string   `json:"username"`
	AllowedLibraryIDs []string `json:"allowed_library_ids,omitempty"`
	DefaultLibraryID  string   `json:"default_library_id,omitempty"`
	CurrentLibraryID  string   `json:"current_library_id,omitempty"`
}

type LibraryAvailabilityHooks struct {
	FilterVisibleLibraries      func(cfg *config.Config, username string, libraries []config.Library) []config.Library
	DescribeLibraryAvailability func(cfg *config.Config, username string, libraries []config.Library) map[string]LibraryAvailabilityState
	ValidateLibrarySelection    func(cfg *config.Config, username string, libraryID string) error
}

type cachedLibraryStats struct {
	DBSize    int64
	DBModTime time.Time
	ExpiresAt time.Time
	Response  libraryResponse
}

var libraryStatsCache sync.Map

type RequestError struct {
	Status  int
	Reason  string
	Message string
}

func (e *RequestError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if strings.TrimSpace(e.Reason) != "" {
		return e.Reason
	}
	return "request failed"
}

func buildSettingsResponse(cfg *config.Config, profile *config.Profile, username string, hooks LibraryAvailabilityHooks) settingsResponse {
	if profile == nil {
		profile = config.DefaultProfileFromConfig(cfg)
	}
	role := currentUserRole(cfg, username)
	activeLibraryID := strings.TrimSpace(profile.ActiveLibraryID)
	storagePath := strings.TrimSpace(profile.StoragePath)
	if role != config.UserRoleRoot {
		if selected, ok := cfg.ResolveUserLibrarySelection(username, activeLibraryID, storagePath); ok {
			activeLibraryID = selected.ID
			storagePath = selected.Path
		}
	} else {
		activeLibraryID = ""
		storagePath = ""
	}
	users := make([]string, 0, len(cfg.Users))
	for _, user := range cfg.Users {
		users = append(users, user.Username)
	}
	masked := "未设置"
	if cfg.JWTSecret != "" {
		if len(cfg.JWTSecret) <= 8 {
			masked = "********"
		} else {
			masked = cfg.JWTSecret[:4] + "..." + cfg.JWTSecret[len(cfg.JWTSecret)-4:]
		}
	}
	localUpdateText := ""
	if path, err := config.LocalUpdatePath(); err == nil {
		if _, text, err := update.LoadOrCreateLocalUpdateConfig(path); err == nil {
			localUpdateText = text
		}
	}
	playerKeymap := profile.Preferences.PlayerKeymap
	if content, err := loadPlayerKeymapContent(); err == nil {
		playerKeymap = content
	}
	return settingsResponse{
		CurrentUsername:               map[bool]string{true: "root", false: username}[isRootConsoleSession(username)],
		Role:                          role,
		CanWrite:                      role == config.UserRoleAdmin,
		CanAdmin:                      role == config.UserRoleAdmin,
		CanRoot:                       role == config.UserRoleRoot,
		Port:                          cfg.Port,
		ActiveLibraryID:               activeLibraryID,
		StoragePath:                   storagePath,
		Libraries:                     buildLibraryResponses(cfg, username, hooks),
		ThumbnailDir:                  profile.ThumbnailDir,
		ThumbnailSize:                 512,
		TrashDir:                      profile.TrashDir,
		UseSystemPlayer:               profile.UseSystemPlayer,
		JWTSecretMasked:               masked,
		Users:                         users,
		Theme:                         profile.Preferences.Theme,
		GridSize:                      profile.Preferences.GridSize,
		GridGap:                       profile.Preferences.GridGap,
		ThumbRadius:                   profile.Preferences.ThumbRadius,
		SidebarAutoHide:               profile.Preferences.SidebarAutoHide,
		SlideshowMode:                 profile.Preferences.SlideshowMode,
		SlideshowLoop:                 profile.Preferences.SlideshowLoop,
		SlideshowInterval:             profile.Preferences.SlideshowInterval,
		LightboxZoom:                  profile.Preferences.LightboxZoom,
		ExperimentalAutoplayVideo:     profile.Preferences.ExperimentalAutoplayVideo,
		VideoAutoplayNext:             profile.Preferences.VideoAutoplayNext,
		VideoSectionMinMinutes:        profile.Preferences.VideoSectionMinMinutes,
		ExperimentalPrefetchNeighbors: profile.Preferences.ExperimentalPrefetchNeighbors,
		ExperimentalRestoreLastView:   profile.Preferences.ExperimentalRestoreLastView,
		ContinueLastVideoPosition:     profile.Preferences.ContinueLastVideoPosition,
		WarmEnabled:                   profile.Preferences.WarmEnabled,
		ThrottledVideoSeek:            profile.Preferences.ThrottledVideoSeek,
		VideoSeekThrottleMS:           profile.Preferences.VideoSeekThrottleMS,
		VideoVolumeSwipeSensitivity:   profile.Preferences.VideoVolumeSwipeSensitivity,
		VideoVolumeMinPercent:         profile.Preferences.VideoVolumeMinPercent,
		VideoVolumeMaxPercent:         profile.Preferences.VideoVolumeMaxPercent,
		LowResourceMode:               profile.Preferences.LowResourceMode,
		PlayerKeymap:                  playerKeymap,
		PWAIconURL:                    pwaIconURL(cfg.Workshop.FaviconAsset),
		LocalUpdateConfigText:         localUpdateText,
	}
}

type localUpdateConfigRequest struct {
	Text string `json:"text"`
}

func handleSaveLocalUpdateConfig(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req localUpdateConfigRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
			return
		}
		path, err := config.LocalUpdatePath()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_, text, err := update.SaveLocalUpdateConfig(path, req.Text)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"text": text})
	}
}

func handleCheckLocalUpdate(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		path, err := config.LocalUpdatePath()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		conf, _, err := update.LoadOrCreateLocalUpdateConfig(path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := update.CheckLocalPackage(conf)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func handleApplyLocalUpdate(cfg *config.Config, shutdown func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shutdown == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持网页更新重启"})
			return
		}
		path, err := config.LocalUpdatePath()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		conf, _, err := update.LoadOrCreateLocalUpdateConfig(path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		exe, err := os.Executable()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		wd, _ := os.Getwd()
		result, err := update.StartLocalUpdate(conf, update.StartOptions{
			CurrentPID:        os.Getpid(),
			CurrentExecutable: exe,
			RestartArgs:       append([]string(nil), os.Args[1:]...),
			WorkingDir:        wd,
			Environment:       append([]string(nil), os.Environ()...),
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
		if result.Restarting {
			go func() {
				time.Sleep(220 * time.Millisecond)
				_ = shutdown()
			}()
		}
	}
}

func buildLibraryStats(cfg *config.Config, library config.Library) libraryResponse {
	stats := libraryResponse{}
	dbPath, err := cfg.DatabasePathForStorage(library.Path)
	if err != nil {
		return stats
	}
	if info, err := os.Stat(dbPath); err == nil {
		if cached, ok := loadCachedLibraryStats(dbPath, info); ok {
			return cached
		}
		stats.CreatedAt = formatLibraryStatTime(info.ModTime())
	} else {
		return stats
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return stats
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return stats
	}
	if createdAt, ok := readLibraryCreatedAt(db); ok {
		stats.CreatedAt = formatLibraryStatTime(createdAt)
	}
	if snapshot, err := readLibraryScanSnapshot(db); err == nil && snapshot != nil && !snapshot.CompletedAt.IsZero() {
		stats.LastScannedAt = formatLibraryStatTime(snapshot.CompletedAt)
	}
	if totalSize, photoCount, videoCount, unsupportedCount, ok := readLibraryMediaStats(db); ok {
		stats.TotalSize = totalSize
		stats.TotalSizeText = formatLibraryBytes(totalSize)
		stats.PhotoCount = photoCount
		stats.VideoCount = videoCount
		stats.UnsupportedMediaCount = unsupportedCount
	}
	if info, err := os.Stat(dbPath); err == nil {
		storeCachedLibraryStats(dbPath, info, stats)
	}
	return stats
}

func loadCachedLibraryStats(dbPath string, info os.FileInfo) (libraryResponse, bool) {
	value, ok := libraryStatsCache.Load(dbPath)
	if !ok {
		return libraryResponse{}, false
	}
	cached, ok := value.(cachedLibraryStats)
	if !ok {
		libraryStatsCache.Delete(dbPath)
		return libraryResponse{}, false
	}
	if time.Now().After(cached.ExpiresAt) || cached.DBSize != info.Size() || !cached.DBModTime.Equal(info.ModTime()) {
		libraryStatsCache.Delete(dbPath)
		return libraryResponse{}, false
	}
	return cached.Response, true
}

func storeCachedLibraryStats(dbPath string, info os.FileInfo, stats libraryResponse) {
	libraryStatsCache.Store(dbPath, cachedLibraryStats{
		DBSize:    info.Size(),
		DBModTime: info.ModTime(),
		ExpiresAt: time.Now().Add(12 * time.Second),
		Response:  stats,
	})
}

func readLibraryCreatedAt(db *sql.DB) (time.Time, bool) {
	var raw string
	err := db.QueryRow(`SELECT uploaded_at FROM photos WHERE deleted_at IS NULL ORDER BY uploaded_at ASC LIMIT 1`).Scan(&raw)
	if err != nil || strings.TrimSpace(raw) == "" {
		return time.Time{}, false
	}
	value, err := parseLibraryTime(raw)
	return value, err == nil && !value.IsZero()
}

func readLibraryMediaStats(db *sql.DB) (int64, int64, int64, int64, bool) {
	var totalSize, photoCount, videoCount, unsupportedCount sql.NullInt64
	err := db.QueryRow(`
		SELECT
			COALESCE(SUM(size), 0),
			COALESCE(SUM(CASE WHEN media_kind = 'image' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN media_kind = 'video' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN lower(original_name) LIKE '%.wmv' OR lower(original_name) LIKE '%.wma' THEN 1 ELSE 0 END), 0)
		FROM photos
		WHERE deleted_at IS NULL`).Scan(&totalSize, &photoCount, &videoCount, &unsupportedCount)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	return totalSize.Int64, photoCount.Int64, videoCount.Int64, unsupportedCount.Int64, true
}

func readLibraryScanSnapshot(db *sql.DB) (*storage.LibraryScanSnapshot, error) {
	var value string
	if err := db.QueryRow(`SELECT value FROM app_meta WHERE key = ?`, "library_scan_snapshot").Scan(&value); err != nil {
		return nil, err
	}
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var snapshot storage.LibraryScanSnapshot
	if err := json.Unmarshal([]byte(value), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func parseLibraryTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("empty time")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
	}
	var lastErr error
	for _, layout := range layouts {
		value, err := time.Parse(layout, raw)
		if err == nil {
			return value, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func formatLibraryStatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format(time.RFC3339)
}

func formatLibraryBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, units[unit])
	}
	if value >= 10 {
		return fmt.Sprintf("%.1f %s", value, units[unit])
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func buildLibraryResponses(cfg *config.Config, username string, hooks LibraryAvailabilityHooks) []libraryResponse {
	libraries := cfg.VisibleLibrariesForUser(username)
	if hooks.FilterVisibleLibraries != nil {
		libraries = hooks.FilterVisibleLibraries(cfg, username, libraries)
	}
	availabilityByID := map[string]LibraryAvailabilityState{}
	if hooks.DescribeLibraryAvailability != nil {
		availabilityByID = hooks.DescribeLibraryAvailability(cfg, username, libraries)
	}
	resp := make([]libraryResponse, 0, len(libraries))
	for index, library := range libraries {
		globalIndex := findGlobalLibraryIndex(cfg, library)
		status := strings.TrimSpace(library.Status)
		if status == "" {
			status = config.DetectLibraryStatus(library.Path)
		}
		responseIndex := index
		if globalIndex >= 0 {
			responseIndex = globalIndex
		}
		item := libraryResponse{
			Index:       responseIndex,
			ID:          library.ID,
			Name:        library.Name,
			Path:        library.Path,
			LogoAsset:   library.LogoAsset,
			AccentColor: library.AccentColor,
			Status:      status,
			Available:   status != config.LibraryStatusMissing && status != config.LibraryStatusLocked,
		}
		stats := buildLibraryStats(cfg, library)
		item.CreatedAt = stats.CreatedAt
		item.LastScannedAt = stats.LastScannedAt
		item.UnsupportedMediaCount = stats.UnsupportedMediaCount
		item.TotalSize = stats.TotalSize
		item.TotalSizeText = stats.TotalSizeText
		item.PhotoCount = stats.PhotoCount
		item.VideoCount = stats.VideoCount
		if !item.Available && status == config.LibraryStatusMissing {
			item.UnavailableReason = "资源库路径当前不可用"
		}
		if availability, ok := availabilityByID[strings.TrimSpace(library.ID)]; ok {
			if strings.TrimSpace(availability.Status) != "" {
				item.Status = strings.TrimSpace(availability.Status)
			}
			if !availability.Available {
				item.Available = false
				item.UnavailableReason = strings.TrimSpace(availability.Reason)
				item.LockedByUsername = strings.TrimSpace(availability.OwnerUsername)
			}
		}
		if library.LogoAsset != "" {
			item.LogoImageURL = fmt.Sprintf("/api/settings/libraries/%d/logo?v=%s", responseIndex, library.LogoAsset)
		}
		resp = append(resp, item)
	}
	return resp
}

func preserveLibraryStatuses(existing []config.Library, next []config.Library) []config.Library {
	if len(next) == 0 {
		return next
	}
	byID := make(map[string]string, len(existing))
	byPath := make(map[string]string, len(existing))
	for _, library := range existing {
		status := strings.TrimSpace(library.Status)
		if status == "" {
			status = config.DetectLibraryStatus(library.Path)
		}
		if id := strings.TrimSpace(library.ID); id != "" {
			byID[id] = status
		}
		if path := config.NormalizeStoragePath(library.Path); path != "" {
			byPath[path] = status
		}
	}
	for index := range next {
		if strings.TrimSpace(next[index].Status) != "" {
			next[index].Status = strings.TrimSpace(next[index].Status)
			continue
		}
		if status, ok := byID[strings.TrimSpace(next[index].ID)]; ok {
			next[index].Status = status
			continue
		}
		if status, ok := byPath[config.NormalizeStoragePath(next[index].Path)]; ok {
			next[index].Status = status
			continue
		}
		next[index].Status = config.DetectLibraryStatus(next[index].Path)
	}
	return next
}

func preserveLibraryOwners(existing []config.Library, next []config.Library) []config.Library {
	if len(next) == 0 {
		return next
	}
	byID := make(map[string]string, len(existing))
	byPath := make(map[string]string, len(existing))
	for _, library := range existing {
		owner := strings.TrimSpace(library.OwnerUsername)
		if owner == "" {
			continue
		}
		if id := strings.TrimSpace(library.ID); id != "" {
			byID[id] = owner
		}
		if path := config.NormalizeStoragePath(library.Path); path != "" {
			byPath[path] = owner
		}
	}
	for index := range next {
		if strings.TrimSpace(next[index].OwnerUsername) != "" {
			continue
		}
		if owner, ok := byID[strings.TrimSpace(next[index].ID)]; ok {
			next[index].OwnerUsername = owner
			continue
		}
		if owner, ok := byPath[config.NormalizeStoragePath(next[index].Path)]; ok {
			next[index].OwnerUsername = owner
		}
	}
	return next
}

func buildUserLibraryAccessResponses(cfg *config.Config, role string) []userLibraryAccessResponse {
	if cfg == nil || role != config.UserRoleAdmin {
		return nil
	}
	rows := make([]userLibraryAccessResponse, 0, len(cfg.Users))
	for _, user := range cfg.Users {
		currentLibraryID := ""
		if profile, err := config.EnsureProfile(cfg, user.Username); err == nil {
			if selected, ok := cfg.ResolveUserLibrarySelection(user.Username, profile.ActiveLibraryID, profile.StoragePath); ok {
				currentLibraryID = selected.ID
			}
		}
		rows = append(rows, userLibraryAccessResponse{
			Username:          user.Username,
			Role:              config.NormalizeUserRole(user.Role),
			AllowedLibraryIDs: buildVisibleAllowedLibraryIDs(cfg, &user),
			DefaultLibraryID:  strings.TrimSpace(user.DefaultLibraryID),
			CurrentLibraryID:  currentLibraryID,
		})
	}
	return rows
}

func buildVisibleAllowedLibraryIDs(cfg *config.Config, user *config.User) []string {
	if cfg == nil || user == nil {
		return nil
	}
	if config.UserIsAdmin(user) {
		ids := make([]string, 0, len(cfg.Libraries))
		for _, library := range cfg.Libraries {
			if id := strings.TrimSpace(library.ID); id != "" {
				ids = append(ids, id)
			}
		}
		return ids
	}
	return append([]string(nil), user.AllowedLibraryIDs...)
}

func normalizedLibraryIDs(libraries []config.Library) []string {
	ids := make([]string, 0, len(libraries))
	seen := make(map[string]struct{}, len(libraries))
	for _, library := range libraries {
		id := strings.TrimSpace(library.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func normalizeAllowedLibraryIDs(libraries []config.Library, requested []string) []string {
	valid := make(map[string]struct{}, len(libraries))
	for _, library := range libraries {
		if id := strings.TrimSpace(library.ID); id != "" {
			valid[id] = struct{}{}
		}
	}
	selected := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := valid[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, id)
	}
	return selected
}

func normalizeDefaultLibraryID(allowedLibraryIDs []string, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" && slices.Contains(allowedLibraryIDs, requested) {
		return requested
	}
	if len(allowedLibraryIDs) > 0 {
		return allowedLibraryIDs[0]
	}
	return ""
}

func applyUserLibraryAccessUpdates(nextGlobal *config.Config, actingUsername string, actingProfile *config.Profile, updates []userLibraryAccessUpdateRequest) error {
	if nextGlobal == nil {
		return nil
	}
	if len(updates) == 0 {
		return nil
	}
	byUsername := make(map[string]userLibraryAccessUpdateRequest, len(updates))
	for _, update := range updates {
		username := strings.TrimSpace(update.Username)
		if username == "" {
			continue
		}
		byUsername[strings.ToLower(username)] = update
	}
	allLibraryIDs := normalizedLibraryIDs(nextGlobal.Libraries)
	for index := range nextGlobal.Users {
		user := &nextGlobal.Users[index]
		update, ok := byUsername[strings.ToLower(strings.TrimSpace(user.Username))]
		if !ok {
			continue
		}
		if config.UserIsAdmin(user) {
			user.AllowedLibraryIDs = append([]string(nil), allLibraryIDs...)
			user.DefaultLibraryID = normalizeDefaultLibraryID(allLibraryIDs, update.DefaultLibraryID)
			continue
		}
		user.AllowedLibraryIDs = normalizeAllowedLibraryIDs(nextGlobal.Libraries, update.AllowedLibraryIDs)
		user.DefaultLibraryID = normalizeDefaultLibraryID(user.AllowedLibraryIDs, update.DefaultLibraryID)
	}
	for index := range nextGlobal.Users {
		user := &nextGlobal.Users[index]
		update, ok := byUsername[strings.ToLower(strings.TrimSpace(user.Username))]
		if !ok {
			continue
		}
		var profile *config.Profile
		if strings.EqualFold(user.Username, actingUsername) && actingProfile != nil {
			profile = actingProfile
		} else {
			loadedProfile, err := config.EnsureProfile(nextGlobal, user.Username)
			if err != nil {
				return err
			}
			profile = loadedProfile
		}
		if profile == nil {
			continue
		}
		activeLibraryID := profile.ActiveLibraryID
		storagePath := profile.StoragePath
		if !strings.EqualFold(user.Username, actingUsername) {
			if requested := strings.TrimSpace(update.CurrentLibraryID); requested != "" {
				activeLibraryID = requested
				storagePath = ""
			}
		}
		if selected, ok := nextGlobal.ResolveUserLibrarySelection(user.Username, activeLibraryID, storagePath); ok {
			profile.ActiveLibraryID = selected.ID
			profile.StoragePath = selected.Path
		} else {
			profile.ActiveLibraryID = ""
			profile.StoragePath = ""
		}
		if strings.EqualFold(user.Username, actingUsername) {
			continue
		}
		if err := config.SaveProfile(nextGlobal, user.Username, profile); err != nil {
			return err
		}
	}
	return nil
}

func visibleProfileForUser(cfg *config.Config, username string) (*config.Profile, string, error) {
	if isRootConsoleSession(username) {
		return config.NewUserProfileTemplate(cfg), "", nil
	}
	ownProfile, err := config.EnsureProfile(cfg, username)
	if err != nil {
		return nil, "", err
	}
	return ownProfile, username, nil
}

func requestVisibleProfile(c *gin.Context, cfg *config.Config) (*config.Profile, string, bool) {
	username := currentUsername(c)
	if strings.TrimSpace(username) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return nil, "", false
	}
	profile, ownerUsername, err := visibleProfileForUser(cfg, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, "", false
	}
	return profile, ownerUsername, true
}

func handleGetSettings(cfg *config.Config, hooks LibraryAvailabilityHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := currentUsername(c)
		profile, _, ok := requestVisibleProfile(c, cfg)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, buildSettingsResponse(cfg, profile, username, hooks))
	}
}

func handleUpdateSettings(cfg *config.Config, hooks LibraryAvailabilityHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req settingsUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}

		username := currentUsername(c)
		prevProfile, ok := requestProfile(c, cfg)
		if !ok {
			return
		}
		role := currentUserRole(cfg, username)
		prevThumbnailDir := strings.TrimSpace(prevProfile.ThumbnailDir)
		prevThumbnailSize := prevProfile.ThumbnailSize
		nextProfile := &config.Profile{
			ActiveLibraryID: req.ActiveLibraryID,
			StoragePath:     req.StoragePath,
			ThumbnailDir:    req.ThumbnailDir,
			ThumbnailSize:   512,
			TrashDir:        req.TrashDir,
			UseSystemPlayer: req.UseSystemPlayer,
			Preferences: config.Preferences{
				Theme:                         req.Theme,
				GridSize:                      req.GridSize,
				GridGap:                       req.GridGap,
				ThumbRadius:                   req.ThumbRadius,
				SidebarAutoHide:               req.SidebarAutoHide,
				SlideshowMode:                 req.SlideshowMode,
				SlideshowLoop:                 req.SlideshowLoop,
				SlideshowInterval:             req.SlideshowInterval,
				LightboxZoom:                  req.LightboxZoom,
				ExperimentalAutoplayVideo:     req.ExperimentalAutoplayVideo,
				VideoAutoplayNext:             req.VideoAutoplayNext,
				VideoSectionMinMinutes:        req.VideoSectionMinMinutes,
				ExperimentalPrefetchNeighbors: req.ExperimentalPrefetchNeighbors,
				ExperimentalRestoreLastView:   req.ExperimentalRestoreLastView,
				ContinueLastVideoPosition:     req.ContinueLastVideoPosition,
				WarmEnabled:                   req.WarmEnabled,
				ThrottledVideoSeek:            req.ThrottledVideoSeek,
				VideoSeekThrottleMS:           req.VideoSeekThrottleMS,
				VideoVolumeSwipeSensitivity:   req.VideoVolumeSwipeSensitivity,
				VideoVolumeMinPercent:         req.VideoVolumeMinPercent,
				VideoVolumeMaxPercent:         req.VideoVolumeMaxPercent,
				LowResourceMode:               req.LowResourceMode,
				PlayerKeymap:                  req.PlayerKeymap,
			},
		}
		nextPort := req.Port
		if role != config.UserRoleAdmin {
			nextProfile.ActiveLibraryID = prevProfile.ActiveLibraryID
			nextProfile.StoragePath = prevProfile.StoragePath
			nextProfile.ThumbnailDir = prevProfile.ThumbnailDir
			nextProfile.ThumbnailSize = prevProfile.ThumbnailSize
			nextProfile.TrashDir = prevProfile.TrashDir
			nextProfile.UseSystemPlayer = prevProfile.UseSystemPlayer
			nextProfile.Preferences.LowResourceMode = prevProfile.Preferences.LowResourceMode
			nextProfile.Preferences.PlayerKeymap = prevProfile.Preferences.PlayerKeymap
			if err := config.SaveProfile(cfg, username, nextProfile); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			visibleProfile, _, err := visibleProfileForUser(cfg, username)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message":          "设置已保存",
				"requires_restart": false,
				"data":             buildSettingsResponse(cfg, visibleProfile, username, hooks),
			})
			return
		}
		requiresRestart := settingsProfileRequiresRestart(cfg, prevProfile, nextProfile, nextPort) || settingsLibrariesChanged(cfg.Libraries, req.Libraries)
		nextGlobal := *cfg
		nextGlobal.Port = nextPort
		nextGlobal.ActiveProfile = username
		nextGlobal.Users = append([]config.User(nil), cfg.Users...)
		nextGlobal.Libraries = append([]config.Library(nil), req.Libraries...)
		nextGlobal.Libraries = preserveLibraryOwners(cfg.Libraries, nextGlobal.Libraries)
		nextGlobal.Libraries = preserveLibraryStatuses(cfg.Libraries, nextGlobal.Libraries)
		if hooks.ValidateLibrarySelection != nil {
			if err := hooks.ValidateLibrarySelection(&nextGlobal, username, nextProfile.ActiveLibraryID); err != nil {
				var requestErr *RequestError
				if errors.As(err, &requestErr) {
					c.JSON(requestErr.Status, gin.H{
						"error":  requestErr.Error(),
						"reason": strings.TrimSpace(requestErr.Reason),
					})
					return
				}
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		if selected, ok := nextGlobal.ResolveUserLibrarySelection(username, nextProfile.ActiveLibraryID, nextProfile.StoragePath); ok {
			nextProfile.ActiveLibraryID = selected.ID
			nextProfile.StoragePath = selected.Path
		} else {
			nextProfile.ActiveLibraryID = ""
			nextProfile.StoragePath = ""
		}
		if err := savePlayerKeymapContent(nextProfile.Preferences.PlayerKeymap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := config.SaveProfile(&nextGlobal, username, nextProfile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		nextGlobal.ApplyProfile(nextProfile)
		if err := nextGlobal.Save(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cfg.Port = nextGlobal.Port
		cfg.ActiveProfile = nextGlobal.ActiveProfile
		cfg.Users = append([]config.User(nil), nextGlobal.Users...)
		cfg.Libraries = append([]config.Library(nil), nextGlobal.Libraries...)
		cfg.ApplyProfile(nextProfile)
		service.SetLowResourceMode(nextProfile.Preferences.LowResourceMode)
		if prevThumbnailSize != nextProfile.ThumbnailSize && prevThumbnailDir != "" {
			_ = os.RemoveAll(prevThumbnailDir)
			_ = os.MkdirAll(prevThumbnailDir, 0755)
		}
		message := "设置已保存"
		if role == config.UserRoleAdmin {
			message = "设置已保存，涉及服务端行为的变更在重启后完全生效"
		}
		if prevThumbnailSize != nextProfile.ThumbnailSize {
			message = "设置已保存，缩略图缓存已重置，后续浏览会按新尺寸重新生成"
		}
		c.JSON(http.StatusOK, gin.H{
			"message":          message,
			"requires_restart": requiresRestart,
			"data":             buildSettingsResponse(cfg, nextProfile, username, hooks),
		})
	}
}

func requestProfile(c *gin.Context, cfg *config.Config) (*config.Profile, bool) {
	username := currentUsername(c)
	if strings.TrimSpace(username) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return nil, false
	}
	profile, err := config.EnsureProfile(cfg, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return profile, true
}

func saveRequestProfile(c *gin.Context, cfg *config.Config, profile *config.Profile) bool {
	username := currentUsername(c)
	if err := config.SaveProfile(cfg, username, profile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	if cfg.ActiveProfile == username || cfg.ActiveProfile == "" {
		cfg.ApplyProfile(profile)
	}
	return true
}

func handleRestartApp(restart func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if restart == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持网页重启"})
			return
		}
		if err := restart(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "服务正在重启"})
	}
}

func handleShutdownApp(shutdown func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shutdown == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持全局退出"})
			return
		}
		if err := shutdown(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "正在退出 EchoGallery"})
	}
}

func handleUploadLibraryLogo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := currentUsername(c)
		_, globalIndex, err := visibleLibraryByIndex(cfg, username, c.Param("index"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少上传文件"})
			return
		}
		ext := strings.ToLower(filepath.Ext(file.Filename))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 png/jpg/jpeg/gif/webp"})
			return
		}

		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := os.MkdirAll(assetDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		fileName := fmt.Sprintf("library-%d-%d.png", globalIndex, time.Now().UnixNano())
		destPath := filepath.Join(assetDir, fileName)
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上传文件失败"})
			return
		}
		defer src.Close()
		if err := imgpkg.SaveSquareLibraryLogo(src, imgpkg.DetectMimeType(file.Filename), destPath, imgpkg.DefaultLibraryLogoEdge); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		nextCfg := *cfg
		nextCfg.Libraries = append([]config.Library(nil), cfg.Libraries...)
		previous := nextCfg.Libraries[globalIndex].LogoAsset
		nextCfg.Libraries[globalIndex].LogoAsset = fileName
		if err := nextCfg.Save(); err != nil {
			_ = os.Remove(destPath)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cfg.Libraries = append([]config.Library(nil), nextCfg.Libraries...)

		if previous != "" && previous != fileName {
			_ = os.Remove(filepath.Join(assetDir, previous))
		}
		profile, ok := requestProfile(c, cfg)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "资源库头像已上传",
			"data":    buildSettingsResponse(cfg, profile, username, LibraryAvailabilityHooks{}),
		})
	}
}

func handleDeleteLibraryLogo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := currentUsername(c)
		_, globalIndex, err := visibleLibraryByIndex(cfg, username, c.Param("index"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		nextCfg := *cfg
		nextCfg.Libraries = append([]config.Library(nil), cfg.Libraries...)
		previous := nextCfg.Libraries[globalIndex].LogoAsset
		nextCfg.Libraries[globalIndex].LogoAsset = ""
		if err := nextCfg.Save(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cfg.Libraries = append([]config.Library(nil), nextCfg.Libraries...)

		if previous != "" {
			_ = os.Remove(filepath.Join(assetDir, previous))
		}
		profile, ok := requestProfile(c, cfg)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "资源库图像已移除",
			"data":    buildSettingsResponse(cfg, profile, username, LibraryAvailabilityHooks{}),
		})
	}
}

func handleUploadPWAIcon(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少上传文件"})
			return
		}
		ext := strings.ToLower(filepath.Ext(file.Filename))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 png/jpg/jpeg/gif/webp"})
			return
		}
		assetDir, err := cfg.WorkshopAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := os.MkdirAll(assetDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fileName := fmt.Sprintf("pwa-icon-%d.png", time.Now().UnixNano())
		destPath := filepath.Join(assetDir, fileName)
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上传文件失败"})
			return
		}
		defer src.Close()
		if err := imgpkg.SaveSquareLibraryLogo(src, imgpkg.DetectMimeType(file.Filename), destPath, imgpkg.DefaultLibraryLogoEdge); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		nextCfg := *cfg
		previous := strings.TrimSpace(nextCfg.Workshop.FaviconAsset)
		nextCfg.Workshop.FaviconAsset = fileName
		if err := nextCfg.Save(); err != nil {
			_ = os.Remove(destPath)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cfg.Workshop = nextCfg.Workshop
		if previous != "" && previous != fileName {
			_ = os.Remove(filepath.Join(assetDir, filepath.Base(previous)))
		}
		c.JSON(http.StatusOK, gin.H{
			"message":  "PWA 图标已上传",
			"icon_url": pwaIconURL(fileName),
		})
	}
}

func handleServePWAIcon(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileName := strings.TrimSpace(cfg.Workshop.FaviconAsset)
		if fileName == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "PWA 图标尚未上传"})
			return
		}
		assetDir, err := cfg.WorkshopAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.File(filepath.Join(assetDir, filepath.Base(fileName)))
	}
}

func pwaIconURL(fileName string) string {
	if strings.TrimSpace(fileName) == "" {
		return ""
	}
	return "/api/settings/pwa/icon?v=" + url.QueryEscape(filepath.Base(fileName))
}

func handleRefreshLibraryLogos(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req refreshLibraryLogosRequest
		_ = c.ShouldBindJSON(&req)

		profile, ok := requestProfile(c, cfg)
		if !ok {
			return
		}
		result := refreshLibraryLogosResult{}
		libraries := cfg.VisibleLibrariesForUser(currentUsername(c))
		if len(libraries) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"message": "没有可更新的资源库头像",
				"result":  result,
				"data":    buildSettingsResponse(cfg, profile, currentUsername(c), LibraryAvailabilityHooks{}),
			})
			return
		}

		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := os.MkdirAll(assetDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		username := currentUsername(c)
		for _, library := range libraries {
			result.Scanned++
			if strings.TrimSpace(library.Path) == "" {
				result.Skipped++
				continue
			}
			if strings.TrimSpace(library.LogoAsset) != "" && !req.ReplaceExisting && libraryLogoAssetExists(cfg, library.LogoAsset) {
				result.Skipped++
				continue
			}
			updatedProfile, _, err := ensureLibraryLogoAsset(cfg, username, library.ID, req.ReplaceExisting, rng)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", library.Name, err))
				continue
			}
			if updatedProfile != nil {
				profile = updatedProfile
				result.Updated++
			} else {
				result.Skipped++
			}
		}

		message := fmt.Sprintf("资源库头像刷新完成：更新 %d 个，跳过 %d 个，失败 %d 个", result.Updated, result.Skipped, result.Failed)
		c.JSON(http.StatusOK, gin.H{
			"message": message,
			"result":  result,
			"data":    buildSettingsResponse(cfg, profile, currentUsername(c), LibraryAvailabilityHooks{}),
		})
	}
}

func settingsProfileRequiresRestart(cfg *config.Config, prevProfile *config.Profile, nextProfile *config.Profile, nextPort int) bool {
	if cfg == nil || prevProfile == nil || nextProfile == nil {
		return false
	}
	if normalizeSettingsPort(cfg.Port) != normalizeSettingsPort(nextPort) {
		return true
	}
	if config.NormalizeStoragePath(prevProfile.StoragePath) != config.NormalizeStoragePath(nextProfile.StoragePath) {
		return true
	}
	if normalizeSettingsLibraryID(prevProfile.ActiveLibraryID) != normalizeSettingsLibraryID(nextProfile.ActiveLibraryID) {
		return true
	}
	if config.NormalizeStoragePath(prevProfile.ThumbnailDir) != config.NormalizeStoragePath(nextProfile.ThumbnailDir) {
		return true
	}
	if prevProfile.ThumbnailSize != nextProfile.ThumbnailSize {
		return true
	}
	if config.NormalizeStoragePath(prevProfile.TrashDir) != config.NormalizeStoragePath(nextProfile.TrashDir) {
		return true
	}
	return false
}

func settingsLibrariesChanged(prevLibraries []config.Library, nextLibraries []config.Library) bool {
	return !slices.EqualFunc(prevLibraries, nextLibraries, func(a, b config.Library) bool {
		return normalizeSettingsLibraryID(a.ID) == normalizeSettingsLibraryID(b.ID) &&
			strings.TrimSpace(a.Name) == strings.TrimSpace(b.Name) &&
			config.NormalizeStoragePath(a.Path) == config.NormalizeStoragePath(b.Path) &&
			strings.TrimSpace(a.LogoAsset) == strings.TrimSpace(b.LogoAsset) &&
			strings.TrimSpace(a.AccentColor) == strings.TrimSpace(b.AccentColor) &&
			strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status)
	})
}

func normalizeSettingsPort(port int) int {
	if port <= 0 {
		return 8080
	}
	return port
}

func normalizeSettingsLibraryID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func libraryLogoAssetExists(cfg *config.Config, asset string) bool {
	if cfg == nil || strings.TrimSpace(asset) == "" {
		return false
	}
	assetDir, err := cfg.LibraryAssetsDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(assetDir, filepath.Base(strings.TrimSpace(asset))))
	return err == nil
}

func ensureLibraryLogoAsset(cfg *config.Config, username string, libraryID string, replaceExisting bool, rng *rand.Rand) (*config.Profile, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("资源库索引无效")
	}
	index := findGlobalLibraryIndexByID(cfg, libraryID)
	if index < 0 {
		return nil, "", fmt.Errorf("资源库索引无效")
	}
	library := cfg.Libraries[index]
	previousAsset := strings.TrimSpace(library.LogoAsset)
	if previousAsset != "" && !replaceExisting && libraryLogoAssetExists(cfg, previousAsset) {
		return nil, previousAsset, nil
	}
	sourcePath, err := randomLibraryLogoThumbnailSource(cfg, library, rng)
	if err != nil {
		return nil, "", err
	}
	assetDir, err := cfg.LibraryAssetsDir()
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(assetDir, 0755); err != nil {
		return nil, "", err
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	fileName := fmt.Sprintf("library-%d-%d.png", index, time.Now().UnixNano())
	destPath := filepath.Join(assetDir, fileName)
	if err := imgpkg.SaveSquareLibraryLogo(file, imgpkg.DetectMimeType(sourcePath), destPath, imgpkg.DefaultLibraryLogoEdge); err != nil {
		_ = os.Remove(destPath)
		return nil, "", err
	}

	nextCfg := *cfg
	nextCfg.Libraries = append([]config.Library(nil), cfg.Libraries...)
	nextCfg.Libraries[index].LogoAsset = fileName
	if err := nextCfg.Save(); err != nil {
		_ = os.Remove(destPath)
		return nil, "", err
	}
	cfg.Libraries = append([]config.Library(nil), nextCfg.Libraries...)
	if previousAsset != "" && previousAsset != fileName {
		_ = os.Remove(filepath.Join(assetDir, filepath.Base(previousAsset)))
	}
	_ = username
	return nil, fileName, nil
}

func randomLibraryLogoThumbnailSource(cfg *config.Config, library config.Library, rng *rand.Rand) (string, error) {
	root := strings.TrimSpace(cfg.ThumbnailDir)
	libraryID := normalizeSettingsLibraryID(library.ID)
	if root == "" || libraryID == "" {
		return "", fmt.Errorf("未找到可用缩略图")
	}
	return randomLibraryLogoSource(filepath.Join(config.NormalizeStoragePath(root), libraryID), rng)
}

func randomLibraryLogoSource(root string, rng *rand.Rand) (string, error) {
	var selected string
	seen := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".thumbnails" || name == ".DS_Store" || strings.HasPrefix(name, ".echogallery") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isLibraryLogoCandidate(path) {
			return nil
		}
		seen++
		if rng.Intn(seen) == 0 {
			selected = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", fmt.Errorf("未找到可用图片")
	}
	return selected, nil
}

func isLibraryLogoCandidate(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(base, ".preview.webp") || strings.HasSuffix(base, ".build-preview.webp") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func handleServeLibraryLogo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := currentUsername(c)
		library, globalIndex, err := visibleLibraryByIndex(cfg, username, c.Param("index"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		fileName := strings.TrimSpace(library.LogoAsset)
		if fileName == "" || !libraryLogoAssetExists(cfg, fileName) {
			updatedProfile, refreshedAsset, refreshErr := ensureLibraryLogoAsset(cfg, username, library.ID, false, rand.New(rand.NewSource(time.Now().UnixNano())))
			if refreshErr != nil {
				if os.IsNotExist(refreshErr) || strings.Contains(refreshErr.Error(), "未找到可用图片") {
					c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "资源库头像生成失败"})
				return
			}
			_ = updatedProfile
			fileName = refreshedAsset
			if globalIndex >= 0 && globalIndex < len(cfg.Libraries) {
				library = cfg.Libraries[globalIndex]
			}
		}
		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.File(filepath.Join(assetDir, filepath.Base(fileName)))
	}
}

func visibleLibraryByIndex(cfg *config.Config, username string, rawIndex string) (config.Library, int, error) {
	index, err := parseLibraryIndex(rawIndex, len(cfg.Libraries))
	if err != nil {
		return config.Library{}, -1, err
	}
	library := cfg.Libraries[index]
	if !userCanAccessLibrary(cfg, username, library) {
		return config.Library{}, -1, fmt.Errorf("资源库索引无效")
	}
	return library, index, nil
}

func userCanAccessLibrary(cfg *config.Config, username string, target config.Library) bool {
	for _, library := range cfg.VisibleLibrariesForUser(username) {
		if sameLibraryIdentity(library, target) {
			return true
		}
	}
	return false
}

func sameLibraryIdentity(a config.Library, b config.Library) bool {
	aID := normalizeSettingsLibraryID(a.ID)
	bID := normalizeSettingsLibraryID(b.ID)
	if aID != "" && bID != "" {
		return aID == bID
	}
	aPath := config.NormalizeStoragePath(a.Path)
	bPath := config.NormalizeStoragePath(b.Path)
	return aPath != "" && aPath == bPath
}

func findGlobalLibraryIndex(cfg *config.Config, library config.Library) int {
	if index := findGlobalLibraryIndexByID(cfg, library.ID); index >= 0 {
		return index
	}
	normalizedPath := config.NormalizeStoragePath(library.Path)
	for index, candidate := range cfg.Libraries {
		if config.NormalizeStoragePath(candidate.Path) == normalizedPath && normalizedPath != "" {
			return index
		}
	}
	return -1
}

func findGlobalLibraryIndexByID(cfg *config.Config, id string) int {
	normalized := normalizeSettingsLibraryID(id)
	for index, library := range cfg.Libraries {
		if normalizeSettingsLibraryID(library.ID) == normalized {
			return index
		}
	}
	return -1
}

func parseLibraryIndex(raw string, total int) (int, error) {
	index := -1
	if _, err := fmt.Sscanf(raw, "%d", &index); err != nil || index < 0 || index >= total {
		return 0, fmt.Errorf("资源库索引无效")
	}
	return index, nil
}

func handleRefreshVideoThumbnails(cfg *config.Config, registrar interface {
	RefreshVideoThumbnails(userID int64) (*service.VideoThumbnailRefreshResult, error)
}) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		result, err := registrar.RefreshVideoThumbnails(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "视频缩略图刷新完成",
			"data":    result,
		})
	}
}

func handleBackfillPhotoEXIF(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		backfiller, ok := registrar.(interface {
			StartEXIFBackfill(userID int64) (service.EXIFBackfillStatus, error)
		})
		if !ok || backfiller == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持媒体元数据修正"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		status, err := backfiller.StartEXIFBackfill(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildEXIFBackfillStatusResponse(status))
	}
}

func handleGetBackfillPhotoEXIFStatus(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		backfiller, ok := registrar.(interface {
			GetEXIFBackfillStatus(userID int64) service.EXIFBackfillStatus
		})
		if !ok || backfiller == nil {
			c.JSON(http.StatusOK, exifBackfillStatusResponse{Status: "idle", Message: "当前没有 EXIF 修正任务"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildEXIFBackfillStatusResponse(backfiller.GetEXIFBackfillStatus(userID)))
	}
}

func handleCancelBackfillPhotoEXIF(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		backfiller, ok := registrar.(interface {
			CancelEXIFBackfill(userID int64) (service.EXIFBackfillStatus, error)
		})
		if !ok || backfiller == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持取消媒体元数据修正"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		status, err := backfiller.CancelEXIFBackfill(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildEXIFBackfillStatusResponse(status))
	}
}

type exifBackfillStatusResponse struct {
	Status         string   `json:"status"`
	Message        string   `json:"message"`
	Done           int      `json:"done"`
	Total          int      `json:"total"`
	Scanned        int      `json:"scanned"`
	Updated        int      `json:"updated"`
	Skipped        int      `json:"skipped"`
	Failed         int      `json:"failed"`
	Errors         []string `json:"errors,omitempty"`
	StartedAt      string   `json:"started_at,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
	FinishedAt     string   `json:"finished_at,omitempty"`
	ElapsedSeconds int64    `json:"elapsed_seconds"`
	ETASeconds     int64    `json:"eta_seconds"`
	Percent        float64  `json:"percent"`
	Error          string   `json:"error,omitempty"`
}

func buildEXIFBackfillStatusResponse(status service.EXIFBackfillStatus) exifBackfillStatusResponse {
	return exifBackfillStatusResponse{
		Status:         status.Status,
		Message:        status.Message,
		Done:           status.Done,
		Total:          status.Total,
		Scanned:        status.Scanned,
		Updated:        status.Updated,
		Skipped:        status.Skipped,
		Failed:         status.Failed,
		Errors:         append([]string(nil), status.Errors...),
		StartedAt:      formatEXIFBackfillTime(status.StartedAt),
		UpdatedAt:      formatEXIFBackfillTime(status.UpdatedAt),
		FinishedAt:     formatEXIFBackfillTime(status.FinishedAt),
		ElapsedSeconds: status.ElapsedSeconds,
		ETASeconds:     status.ETASeconds,
		Percent:        status.Percent,
		Error:          status.Error,
	}
}

func formatEXIFBackfillTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
