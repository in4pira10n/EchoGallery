package api

import (
	"fmt"
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
	cfg            *config.Config
	managedDataDir string
	trashDir       string
	thumbDir       string

	mu       sync.Mutex
	services map[string]videoRegistrar
}

func NewLibraryRuntimeProvider(cfg *config.Config) (*libraryRuntimeProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	managedDataDir, err := cfg.ManagedDataDir()
	if err != nil {
		return nil, err
	}
	trashDir, err := cfg.TrashPath()
	if err != nil {
		return nil, err
	}
	thumbDir, err := cfg.ThumbnailStoragePath()
	if err != nil {
		return nil, err
	}
	return &libraryRuntimeProvider{
		cfg:            cfg,
		managedDataDir: managedDataDir,
		trashDir:       trashDir,
		thumbDir:       thumbDir,
		services:       make(map[string]videoRegistrar),
	}, nil
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
	repo, err := sqlite.New(dbPath)
	if err != nil {
		return nil, err
	}
	photoService := service.NewPhotoService(repo, library.Path, p.managedDataDir, p.trashDir)
	photoService.SetThumbnailRoot(p.thumbDir)
	photoService.SetThumbnailLibraryID(library.ID)
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
	return photoService, nil
}
