package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"echogallery/internal/config"
	"echogallery/internal/update"
	"github.com/gin-gonic/gin"
)

type updateGallery struct {
	update.TargetStatus
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

func updateGalleryID(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}

// Read identity only: config.Load would migrate profiles and write to another
// installation. Prefer the signed-in account's star, then the active library.
func readUpdateGalleryIdentity(binary, username string) (string, string) {
	dir := filepath.Dir(binary)
	name := filepath.Base(dir)
	f, err := os.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		return name, ""
	}
	defer f.Close()
	var metadata struct {
		Workshop        config.Workshop  `json:"workshop"`
		Libraries       []config.Library `json:"libraries"`
		Users           []config.User    `json:"users"`
		ActiveLibraryID string           `json:"active_library_id"`
		StoragePath     string           `json:"storage_path"`
	}
	if json.NewDecoder(io.LimitReader(f, 4*1024*1024)).Decode(&metadata) != nil {
		return name, ""
	}
	if metadata.Workshop.AppName != "" {
		name = metadata.Workshop.AppName
	}
	id := metadata.ActiveLibraryID
	for _, user := range metadata.Users {
		if user.Username == username && user.DefaultLibraryID != "" {
			id = user.DefaultLibraryID
			break
		}
	}
	var library config.Library
	for _, item := range metadata.Libraries {
		if item.ID == id {
			library = item
			break
		}
	}
	if library.Path == "" && len(metadata.Libraries) > 0 {
		library = metadata.Libraries[0]
	}
	if library.Path == "" {
		library.Path = metadata.StoragePath
	}
	if library.Path == "" {
		return name, ""
	}
	if !filepath.IsAbs(library.Path) {
		library.Path = filepath.Join(dir, library.Path)
	}
	if library.Name != "" && metadata.Workshop.AppName == "" {
		name = library.Name
	}
	logo := config.LibraryLogoPath(library.Path)
	if regularLibraryLogo(logo) {
		return name, logo
	}
	if asset := filepath.Base(library.LogoAsset); asset != "" && asset != "." && asset != portableLibraryLogoName {
		legacy := filepath.Join(dir, "echogallery-data", "libraries", asset)
		if regularLibraryLogo(legacy) {
			return name, legacy
		}
	}
	return name, ""
}

func updateGalleryRows(targets []update.TargetStatus, username string) []updateGallery {
	rows := make([]updateGallery, 0, len(targets))
	for _, target := range targets {
		name, avatar := readUpdateGalleryIdentity(target.Path, username)
		row := updateGallery{TargetStatus: target, ID: updateGalleryID(target.Path), Name: name}
		if avatar != "" {
			row.AvatarURL = "/api/settings/local-update/avatar?id=" + url.QueryEscape(row.ID)
		}
		rows = append(rows, row)
	}
	return rows
}

func handleScanLocalUpdate(c *gin.Context) {
	path, err := config.LocalUpdatePath()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	conf, _, err := update.LoadOrCreateLocalUpdateConfig(path)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	targets, warnings := update.DiscoverTargets(conf)
	c.JSON(http.StatusOK, gin.H{"config": conf, "targets": updateGalleryRows(targets, currentUsername(c)), "warnings": warnings, "current_version": update.Version, "version_code": update.CurrentVersionCode(), "version_name": update.VersionName})
}

func handleLocalUpdateAvatar(c *gin.Context) {
	path, err := config.LocalUpdatePath()
	if err != nil {
		c.Status(404)
		return
	}
	conf, _, err := update.LoadOrCreateLocalUpdateConfig(path)
	if err != nil {
		c.Status(404)
		return
	}
	targets, _ := update.DiscoverTargets(conf)
	for _, target := range targets {
		if !target.Exists || updateGalleryID(target.Path) != strings.TrimSpace(c.Query("id")) {
			continue
		}
		_, avatar := readUpdateGalleryIdentity(target.Path, currentUsername(c))
		if avatar != "" {
			c.Header("Cache-Control", "private, no-cache")
			c.Header("X-Content-Type-Options", "nosniff")
			c.File(avatar)
			return
		}
	}
	c.Status(http.StatusNotFound)
}
