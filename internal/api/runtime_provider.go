package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"echogallery/internal/config"
	"echogallery/internal/service"
	"echogallery/internal/sessionlock"
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
	initMu   sync.Mutex
	requests sync.RWMutex
	services map[string]videoRegistrar
	repos    map[string]*sqlite.DB
	sessions map[string]string
}

func NewLibraryRuntimeProvider(cfg *config.Config) (*libraryRuntimeProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	return &libraryRuntimeProvider{
		cfg:      cfg,
		services: make(map[string]videoRegistrar),
		repos:    make(map[string]*sqlite.DB),
		sessions: make(map[string]string),
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
	p.requests.Lock()
	defer p.requests.Unlock()
	p.initMu.Lock()
	defer p.initMu.Unlock()
	p.mu.Lock()
	repos := p.repos
	services := p.services
	p.repos = make(map[string]*sqlite.DB)
	p.services = make(map[string]videoRegistrar)
	p.sessions = make(map[string]string)
	p.mu.Unlock()
	var firstErr error
	for _, registrar := range services {
		if stopper, ok := registrar.(interface{ StopBackground() }); ok {
			stopper.StopBackground()
		}
	}
	for _, repo := range repos {
		if err := repo.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SyncBeforeRelease keeps the session lock until its durable snapshots succeed.
func (p *libraryRuntimeProvider) SyncBeforeRelease(locks *sessionlock.Store, sessionID, keepLibraryID string) error {
	if p == nil || locks == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	// Ordinary media requests do not need the exclusive shutdown barrier.
	p.mu.Lock()
	unchanged := keepLibraryID != "" && p.sessions[sessionID] == keepLibraryID
	p.mu.Unlock()
	if unchanged {
		return nil
	}
	p.requests.Lock()
	defer p.requests.Unlock()
	p.initMu.Lock()
	defer p.initMu.Unlock()
	p.mu.Lock()
	if keepLibraryID != "" && p.sessions[sessionID] == keepLibraryID {
		p.mu.Unlock()
		return nil
	}
	repos := make(map[string]*sqlite.DB, len(p.repos))
	for id, repo := range p.repos {
		repos[id] = repo
	}
	p.mu.Unlock()
	for id, repo := range repos {
		if id == keepLibraryID {
			continue
		}
		active, err := locks.ActiveForLibrary(id)
		if err != nil {
			return err
		}
		for _, lock := range active {
			if lock.SessionID == sessionID && lock.Scope == "browse" {
				// Another reader or a batch build still owns this runtime.
				shared := false
				for _, owner := range active {
					if owner.SessionID != sessionID || owner.Scope != "browse" {
						shared = true
					}
				}
				if !shared {
					p.mu.Lock()
					registrar := p.services[id]
					p.mu.Unlock()
					if stopper, ok := registrar.(interface{ StopBackground() }); ok {
						stopper.StopBackground()
					}
				}
				if err := repo.SyncPortable(); err != nil {
					return fmt.Errorf("sync library %s before release: %w", id, err)
				}
				if !shared {
					if err := repo.Close(); err != nil {
						return fmt.Errorf("close library %s: %w", id, err)
					}
					p.mu.Lock()
					delete(p.repos, id)
					delete(p.services, id)
					p.mu.Unlock()
				}
				break
			}
		}
	}
	return nil
}

func (p *libraryRuntimeProvider) beginRequest() func() {
	if p == nil {
		return func() {}
	}
	p.requests.RLock()
	return p.requests.RUnlock
}

func (p *libraryRuntimeProvider) rememberSession(sessionID, libraryID string) {
	if p == nil || sessionID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions == nil {
		p.sessions = make(map[string]string)
	}
	p.sessions[sessionID] = libraryID
}

func (p *libraryRuntimeProvider) ForLibrary(baseCfg *config.Config, library config.Library) (videoRegistrar, error) {
	if p == nil {
		return nil, fmt.Errorf("runtime provider is not configured")
	}
	p.initMu.Lock()
	defer p.initMu.Unlock()
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
