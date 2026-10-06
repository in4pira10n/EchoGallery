package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

const configTransferVersion = 1

type configTransferManifest struct {
	Version         int             `json:"version"`
	ExportedAt      string          `json:"exported_at"`
	Config          config.Config   `json:"config"`
	BrowserSettings json.RawMessage `json:"browser_settings,omitempty"`
}

func addTransferFile(writer *zip.Writer, name, path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) || (err == nil && !info.Mode().IsRegular()) {
		return nil
	}
	if err != nil {
		return err
	}
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	header.Method = zip.Deflate
	output, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return err
}

func addTransferDirectory(writer *zip.Writer, prefix, root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return addTransferFile(writer, filepath.Join(prefix, relative), path)
	})
}

func buildConfigTransferBundle(cfg *config.Config) ([]byte, error) {
	return buildConfigTransferBundleWithBrowser(cfg, nil)
}

func buildConfigTransferBundleWithBrowser(cfg *config.Config, browserSettings json.RawMessage) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不可用")
	}
	clone := *cfg
	clone.AppDataDir = ""
	clone.ThumbnailDir = ""
	clone.TrashDir = ""
	clone.BatchScan = config.BatchTaskState{}
	clone.BatchThumbnails = config.BatchTaskState{}
	manifest := configTransferManifest{Version: configTransferVersion, ExportedAt: time.Now().Format(time.RFC3339), Config: clone}
	if len(browserSettings) > 0 && json.Valid(browserSettings) {
		manifest.BrowserSettings = append(json.RawMessage(nil), browserSettings...)
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	manifestFile, err := writer.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err := manifestFile.Write(manifestJSON); err != nil {
		return nil, err
	}
	if err := addTransferDirectory(writer, "profiles", filepath.Join(cfg.AppDataDir, "profiles")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := addTransferDirectory(writer, "workshop", filepath.Join(cfg.AppDataDir, "workshop")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if content, err := loadPlayerKeymapContent(); err == nil && strings.TrimSpace(content) != "" {
		file, createErr := writer.Create("player-keymap.conf")
		if createErr != nil {
			return nil, createErr
		}
		if _, createErr = file.Write([]byte(content)); createErr != nil {
			return nil, createErr
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func transferZipFiles(data []byte) (configTransferManifest, map[string][]byte, error) {
	var manifest configTransferManifest
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return manifest, nil, fmt.Errorf("配置包格式无效: %w", err)
	}
	files := make(map[string][]byte)
	var total int64
	for _, file := range reader.File {
		name := filepath.ToSlash(filepath.Clean(file.Name))
		if name == "." || strings.HasPrefix(name, "../") || filepath.IsAbs(name) || file.FileInfo().IsDir() {
			continue
		}
		if file.UncompressedSize64 > 16<<20 {
			return manifest, nil, fmt.Errorf("配置包文件过大: %s", name)
		}
		total += int64(file.UncompressedSize64)
		if total > 64<<20 {
			return manifest, nil, fmt.Errorf("配置包解压后超过 64 MB")
		}
		input, err := file.Open()
		if err != nil {
			return manifest, nil, err
		}
		content, err := io.ReadAll(input)
		_ = input.Close()
		if err != nil {
			return manifest, nil, err
		}
		files[name] = content
	}
	manifestJSON, ok := files["manifest.json"]
	if !ok {
		return manifest, nil, fmt.Errorf("配置包缺少 manifest.json")
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return manifest, nil, fmt.Errorf("配置清单无效: %w", err)
	}
	if manifest.Version != configTransferVersion {
		return manifest, nil, fmt.Errorf("不支持的配置包版本: %d", manifest.Version)
	}
	return manifest, files, nil
}

func writeTransferAsset(root, relative string, content []byte) error {
	target := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".config-import-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}

func remapTransferredLibraries(imported, current []config.Library) []config.Library {
	currentByID := make(map[string]config.Library, len(current))
	for _, library := range current {
		currentByID[strings.ToLower(strings.TrimSpace(library.ID))] = library
	}
	result := append([]config.Library(nil), imported...)
	for index := range result {
		if local, ok := currentByID[strings.ToLower(strings.TrimSpace(result[index].ID))]; ok {
			result[index].Path = local.Path
			result[index].Status = local.Status
			continue
		}
		if _, err := os.Stat(result[index].Path); err != nil {
			result[index].Status = config.LibraryStatusMissing
		}
	}
	return result
}

func importConfigTransferBundle(cfg *config.Config, data []byte) error {
	return importConfigTransferBundleWithSave(cfg, data, func(next *config.Config) error { return next.Save() }, savePlayerKeymapContent)
}

func importConfigTransferBundleWithSave(cfg *config.Config, data []byte, save func(*config.Config) error, saveKeymap func(string) error) error {
	manifest, files, err := transferZipFiles(data)
	if err != nil {
		return err
	}
	backup, err := buildConfigTransferBundle(cfg)
	if err != nil {
		return fmt.Errorf("创建导入前备份失败: %w", err)
	}
	backupDir := filepath.Join(cfg.AppDataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, "before-import-"+time.Now().Format("20060102-150405")+".zip")
	if err := os.WriteFile(backupPath, backup, 0600); err != nil {
		return err
	}
	next := manifest.Config
	next.AppDataDir = cfg.AppDataDir
	next.Libraries = remapTransferredLibraries(next.Libraries, cfg.Libraries)
	next.BatchScan = config.BatchTaskState{}
	next.BatchThumbnails = config.BatchTaskState{}
	if len(next.Users) == 0 {
		return fmt.Errorf("配置包没有用户账户")
	}
	for name, content := range files {
		switch {
		case strings.HasPrefix(name, "profiles/"):
			if err := writeTransferAsset(cfg.AppDataDir, name, content); err != nil {
				return err
			}
		case strings.HasPrefix(name, "workshop/"):
			if err := writeTransferAsset(cfg.AppDataDir, name, content); err != nil {
				return err
			}
		}
	}
	if content := files["player-keymap.conf"]; len(content) > 0 {
		if saveKeymap == nil {
			return fmt.Errorf("快捷键配置保存器不可用")
		}
		if err := saveKeymap(string(content)); err != nil {
			return err
		}
	}
	// Profiles keep stable library IDs; remap their absolute paths locally.
	for _, user := range next.Users {
		profile, err := config.LoadProfile(&next, user.Username)
		if err != nil {
			continue
		}
		if library, ok := next.ResolveUserLibrarySelection(user.Username, profile.ActiveLibraryID, ""); ok {
			profile.ActiveLibraryID = library.ID
			profile.StoragePath = library.Path
		}
		profile.ThumbnailDir = cfg.ThumbnailDir
		profile.TrashDir = cfg.TrashDir
		if err := config.SaveProfile(&next, user.Username, profile); err != nil {
			return err
		}
	}
	if err := persistPortableLibraryMetadata(&next); err != nil {
		return err
	}
	if save == nil {
		return fmt.Errorf("配置保存器不可用")
	}
	if err := save(&next); err != nil {
		return err
	}
	*cfg = next
	return nil
}

func handleExportConfigTransfer(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		browserSettings := json.RawMessage(c.Query("browser_settings"))
		if len(browserSettings) > 32<<10 {
			c.JSON(400, gin.H{"error": "浏览器设置过大"})
			return
		}
		bundle, err := buildConfigTransferBundleWithBrowser(cfg, browserSettings)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", `attachment; filename="echogallery-config.zip"`)
		c.Data(200, "application/zip", bundle)
	}
}

func handleImportConfigTransfer(cfg *config.Config, restart func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(400, gin.H{"error": "请选择配置包"})
			return
		}
		if file.Size <= 0 || file.Size > 64<<20 {
			c.JSON(400, gin.H{"error": "配置包大小必须在 64 MB 以内"})
			return
		}
		input, err := file.Open()
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		defer input.Close()
		data, err := io.ReadAll(io.LimitReader(input, (64<<20)+1))
		if err != nil || len(data) > 64<<20 {
			c.JSON(400, gin.H{"error": "读取配置包失败"})
			return
		}
		manifest, _, inspectErr := transferZipFiles(data)
		if inspectErr != nil {
			c.JSON(400, gin.H{"error": inspectErr.Error()})
			return
		}
		if err := importConfigTransferBundle(cfg, data); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{
			"message": "配置已导入，应用需要重启", "requires_restart": true,
			"browser_settings": manifest.BrowserSettings, "auto_restarting": restart != nil,
		})
		if restart != nil {
			go func() {
				time.Sleep(250 * time.Millisecond)
				if err := restart(); err != nil {
					fmt.Fprintf(os.Stderr, "配置导入后重启失败: %v\n", err)
				}
			}()
		}
	}
}
