package api

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"echogallery/internal/config"
	imgpkg "echogallery/internal/image"
)

const portableLibraryLogoName = "logo.png"

func portableLibraryLogoPath(library config.Library) string {
	return config.LibraryLogoPath(library.Path)
}

func regularLibraryLogo(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func copyLegacyLibraryLogo(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".library-logo-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, input); err != nil {
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

// resolveLibraryLogoPath uses the library-owned file first and copies a
// server-local logo on first access, preserving that old file for older builds.
func resolveLibraryLogoPath(cfg *config.Config, library config.Library) (string, string) {
	portablePath := portableLibraryLogoPath(library)
	if regularLibraryLogo(portablePath) {
		return portablePath, portableLibraryLogoName
	}
	asset := strings.TrimSpace(library.LogoAsset)
	if cfg == nil || asset == "" || asset == portableLibraryLogoName {
		return "", ""
	}
	assetDir, err := cfg.LibraryAssetsDir()
	if err != nil {
		return "", ""
	}
	legacyPath := filepath.Join(assetDir, filepath.Base(asset))
	if !regularLibraryLogo(legacyPath) {
		return "", ""
	}
	if err := copyLegacyLibraryLogo(legacyPath, portablePath); err != nil {
		log.Printf("警告: 复制资源库 %s 的旧 Logo 失败: %v", library.Name, err)
		return legacyPath, filepath.Base(asset)
	}
	return portablePath, portableLibraryLogoName
}

func savePortableLibraryLogo(library config.Library, source io.ReadSeeker, mimeType string) error {
	target := portableLibraryLogoPath(library)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".library-logo-*.png")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	if err := imgpkg.SaveSquareLibraryLogo(source, mimeType, tmpPath, imgpkg.DefaultLibraryLogoEdge); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("保存资源库 Logo 失败: %w", err)
	}
	return nil
}
