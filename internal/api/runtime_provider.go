package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"echogallery/internal/config"
	"echogallery/internal/service"
	"echogallery/internal/storage/sqlite"
)

type requestRuntime struct {
	cfg       *config.Config
	library   config.Library
	profile   *config.Profile
	registrar videoRegistrar
}

type libraryRuntimeProvider struct {
	cfg *config.Config

	mu       sync.Mutex
	services map[string]videoRegistrar
	repos    map[string]*sqlite.DB
}

func NewLibraryRuntimeProvider(cfg *config.Config) (*libraryRuntimeProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	return &libraryRuntimeProvider{
		cfg:      cfg,
		services: make(map[string]videoRegistrar),
		repos:    make(map[string]*sqlite.DB),
	}, nil
}

func (p *libraryRuntimeProvider) Register(libraryID string, registrar videoRegistrar, repo *sqlite.DB) {
	if p == nil || registrar == nil || repo == nil || strings.TrimSpace(libraryID) == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.services[strings.TrimSpace(libraryID)] = registrar
	p.repos[strings.TrimSpace(libraryID)] = repo
}

func (p *libraryRuntimeProvider) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	repos := p.repos
	p.repos = make(map[string]*sqlite.DB)
	p.services = make(map[string]videoRegistrar)
	p.mu.Unlock()
	var firstErr error
	for _, repo := range repos {
		if err := repo.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (p *libraryRuntimeProvider) ForLibrary(baseCfg *config.Config, library config.Library) (videoRegistrar, error) {
	if p == nil {
		return nil, fmt.Errorf("runtime provider is not configured")
	}
	libraryID := strings.TrimSpace(library.ID)
	if libraryID == "" {
		return nil, fmt.Errorf("library id is required")
	}
	p.mu.Lock()
	if registrar := p.services[libraryID]; registrar != nil {
		p.mu.Unlock()
		return registrar, nil
	}
	p.mu.Unlock()

	dbPath, err := p.cfg.DatabasePathForStorage(library.Path)
	if err != nil {
		return nil, err
	}
	managedDataDir, err := p.cfg.ManagedDataDirForStorage(library.Path)
	if err != nil {
		return nil, err
	}
	thumbDir, err := p.cfg.ThumbnailStoragePathForStorage(library.Path)
	if err != nil {
		return nil, err
	}
	if err := config.ValidatePortableLibraryIdentity(library.Path, library.ID); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create library data directory: %w", err)
	}
	if err := os.MkdirAll(managedDataDir, 0755); err != nil {
		return nil, fmt.Errorf("create library media directory: %w", err)
	}
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return nil, fmt.Errorf("create library thumbnail directory: %w", err)
	}
	repo, err := sqlite.NewPortableWorkingCopy(dbPath)
	if err != nil {
		return nil, err
	}
	if err := repo.EnsureLibraryMetadata(library.ID, library.Name, library.AccentColor); err != nil {
		_ = repo.Close()
		return nil, err
	}
	if err := repo.SyncPortable(); err != nil {
		_ = repo.Close()
		return nil, err
	}
	photoService := service.NewPhotoService(repo, library.Path, managedDataDir, "")
	photoService.SetThumbnailRoot(thumbDir)
	photoService.SetThumbnailLibraryID(library.ID)
	if legacyRoot := strings.TrimSpace(p.cfg.ThumbnailDir); legacyRoot != "" {
		if info, statErr := os.Stat(legacyRoot); statErr == nil && info.IsDir() {
			photoService.SetLegacyThumbnailRoot(legacyRoot)
		}
	}
	if baseCfg != nil {
		photoService.SetThumbnailSize(baseCfg.ThumbnailSize)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if registrar := p.services[libraryID]; registrar != nil {
		_ = repo.Close()
		return registrar, nil
	}
	p.services[libraryID] = photoService
	p.repos[libraryID] = repo
	return photoService, nil
}
