package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"echogallery/internal/config"
	"echogallery/internal/danmaku"
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

// A release blocks only requests and initialization for that library.
type libraryRuntimeGate struct {
	requests sync.RWMutex
	init     sync.Mutex
}

type libraryRuntimeProvider struct {
	cfg *config.Config

	mu            sync.Mutex
	requests      sync.RWMutex
	gates         map[string]*libraryRuntimeGate
	services      map[string]videoRegistrar
	repos         map[string]*sqlite.DB
	danmaku       map[string]*danmaku.Manager
	sessions      map[string]string
	asyncReleases map[string]bool
}

func NewLibraryRuntimeProvider(cfg *config.Config) (*libraryRuntimeProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	return &libraryRuntimeProvider{
		cfg:           cfg,
		gates:         make(map[string]*libraryRuntimeGate),
		services:      make(map[string]videoRegistrar),
		repos:         make(map[string]*sqlite.DB),
		danmaku:       make(map[string]*danmaku.Manager),
		sessions:      make(map[string]string),
		asyncReleases: make(map[string]bool),
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

func (p *libraryRuntimeProvider) Danmaku(libraryID string) *danmaku.Manager {
	if p == nil || strings.TrimSpace(libraryID) == "" {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if manager := p.danmaku[libraryID]; manager != nil {
		return manager
	}
	manager := danmaku.NewManager(danmaku.NewClient(nil))
	p.danmaku[libraryID] = manager
	return manager
}

func (p *libraryRuntimeProvider) Close() error {
	if p == nil {
		return nil
	}
	p.CancelBackground()
	for _, library := range p.cfg.Libraries {
		backgroundLibraryMetrics.cancel(normalizeMetricsPath(library.Path))
	}
	p.requests.Lock()
	defer p.requests.Unlock()
	p.mu.Lock()
	repos := make(map[string]*sqlite.DB, len(p.repos))
	services := make(map[string]videoRegistrar, len(p.services))
	for id, repo := range p.repos {
		repos[id] = repo
	}
	for id, registrar := range p.services {
		services[id] = registrar
	}
	danmakuManagers := p.danmaku
	p.danmaku = make(map[string]*danmaku.Manager)
	p.sessions = make(map[string]string)
	p.mu.Unlock()
	var firstErr error
	for _, registrar := range services {
		if stopper, ok := registrar.(interface{ StopBackground() }); ok {
			stopper.StopBackground()
		}
	}
	for _, manager := range danmakuManagers {
		manager.Stop()
	}
	for id, repo := range repos {
		if err := repo.Close(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue // Retain failed snapshots so shutdown/restart can retry safely.
		}
		p.mu.Lock()
		delete(p.repos, id)
		delete(p.services, id)
		p.mu.Unlock()
	}
	return firstErr
}

// CancelBackground signals every service before waiting for HTTP request gates.
// Otherwise a request waiting on a worker can prevent shutdown from cancelling it.
func (p *libraryRuntimeProvider) CancelBackground() {
	if p == nil {
		return
	}
	p.mu.Lock()
	services := make([]videoRegistrar, 0, len(p.services))
	for _, registrar := range p.services {
		services = append(services, registrar)
	}
	p.mu.Unlock()
	for _, registrar := range services {
		if stopper, ok := registrar.(interface{ CancelBackground() }); ok {
			stopper.CancelBackground()
		}
	}
}

func (p *libraryRuntimeProvider) libraryGate(libraryID string) *libraryRuntimeGate {
	libraryID = strings.TrimSpace(libraryID)
	p.mu.Lock()
	defer p.mu.Unlock()
	gate := p.gates[libraryID]
	if gate == nil {
		gate = &libraryRuntimeGate{}
		p.gates[libraryID] = gate
	}
	return gate
}

// SyncBeforeRelease keeps the session lock until its durable snapshots succeed.
func (p *libraryRuntimeProvider) SyncBeforeRelease(locks *sessionlock.Store, sessionID, keepLibraryID string) (resultErr error) {
	if p == nil || locks == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	p.mu.Lock()
	unchanged := keepLibraryID != "" && p.sessions[sessionID] == keepLibraryID
	previousLibrary := p.sessions[sessionID]
	if unchanged {
		p.mu.Unlock()
		return nil
	}
	repos := make(map[string]*sqlite.DB, len(p.repos))
	for id, repo := range p.repos {
		repos[id] = repo
	}
	p.mu.Unlock()
	trace := librarySwitchLogger("release-previous")
	defer func() { trace("finished", "error=", resultErr) }()
	// The global barrier protects shutdown only; unrelated libraries can run.
	p.requests.RLock()
	defer p.requests.RUnlock()
	for id, repo := range repos {
		if id == keepLibraryID {
			continue
		}
		// Check ownership before waiting on another library's release lock.
		active, err := locks.ActiveForLibrary(id)
		if err != nil {
			return err
		}
		owned := id == previousLibrary || len(active) == 0
		for _, owner := range active {
			owned = owned || (owner.SessionID == sessionID && owner.Scope == "browse")
		}
		if !owned {
			continue
		}
		if keepLibraryID != "" && isSMBLibraryID(p.cfg, id) {
			p.scheduleLibraryRelease(locks, sessionID, id, previousLibrary, repo, trace)
			continue
		}
		if err := p.releaseLibrary(locks, sessionID, id, previousLibrary, repo, trace); err != nil {
			return err
		}
	}
	return nil
}

func isSMBLibraryID(cfg *config.Config, id string) bool {
	if cfg == nil {
		return false
	}
	for _, library := range cfg.Libraries {
		if library.ID == id {
			return service.IsSMBPath(library.Path)
		}
	}
	return false
}

func (p *libraryRuntimeProvider) scheduleLibraryRelease(locks *sessionlock.Store, sessionID, id, previousLibrary string, repo *sqlite.DB, trace func(string, ...any)) {
	p.mu.Lock()
	if p.asyncReleases[id] {
		p.mu.Unlock()
		return
	}
	p.asyncReleases[id] = true
	p.mu.Unlock()
	go func() {
		defer func() {
			p.mu.Lock()
			delete(p.asyncReleases, id)
			p.mu.Unlock()
		}()
		if err := p.releaseLibrary(locks, sessionID, id, previousLibrary, repo, trace); err != nil {
			trace("async-release-failed", "library=", id, "error=", err)
		}
	}()
}

func (p *libraryRuntimeProvider) releaseLibrary(locks *sessionlock.Store, sessionID, id, previousLibrary string, repo *sqlite.DB, trace func(string, ...any)) error {
	for _, library := range p.cfg.Libraries {
		if library.ID == id {
			backgroundLibraryMetrics.cancel(normalizeMetricsPath(library.Path))
			break
		}
	}
	gate := p.libraryGate(id)
	trace("wait-requests", "library=", id)
	gate.requests.Lock()
	defer gate.requests.Unlock()
	trace("requests-drained", "library=", id)
	gate.init.Lock()
	defer gate.init.Unlock()
	trace("runtime-lock-acquired", "library=", id)
	p.mu.Lock()
	current, registrar := p.repos[id], p.services[id]
	p.mu.Unlock()
	if current != repo {
		return nil // Another release already closed this particular runtime.
	}
	active, err := locks.ActiveForLibrary(id)
	if err != nil {
		return err
	}
	owned, shared := id == previousLibrary || len(active) == 0, false
	for _, owner := range active {
		if owner.SessionID == sessionID && owner.Scope == "browse" {
			owned = true
		} else {
			shared = true
		}
	}
	if !owned {
		return nil
	}
	trace("lease-inspected", "library=", id, " shared=", shared, " owners=", len(active))
	if shared {
		trace("sync-start", "library=", id)
		if err := repo.SyncPortable(); err != nil {
			return fmt.Errorf("sync library %s before release: %w", id, err)
		}
		trace("sync-complete", "library=", id)
		return nil
	}
	if stopper, ok := registrar.(interface{ StopBackground() }); ok {
		trace("stop-background", "library=", id)
		stopper.StopBackground()
		trace("background-stopped", "library=", id)
	}
	// Close owns the final durable sync. Do not snapshot once here and again
	// inside Close. A failed sync leaves the runtime and lease available to retry.
	trace("close-start", "library=", id, " includes_final_sync=true")
	if err := repo.Close(); err != nil {
		return fmt.Errorf("close library %s: %w", id, err)
	}
	trace("close-complete", "library=", id)
	p.mu.Lock()
	manager := p.danmaku[id]
	delete(p.danmaku, id)
	delete(p.repos, id)
	delete(p.services, id)
	p.mu.Unlock()
	if manager != nil {
		manager.Stop()
	}
	return nil
}

func (p *libraryRuntimeProvider) beginRequest(libraryID string) func() {
	if p == nil {
		return func() {}
	}
	p.requests.RLock()
	gate := p.libraryGate(libraryID)
	gate.requests.RLock()
	return func() {
		gate.requests.RUnlock()
		p.requests.RUnlock()
	}
}

// Optional work must be cancellable even while a library release or process
// shutdown owns a writer gate. Blocking RLock here can deadlock the canceller.
func (p *libraryRuntimeProvider) beginBackgroundRequest(ctx context.Context, libraryID string) (func(), bool) {
	gate := p.libraryGate(libraryID)
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		if p.requests.TryRLock() {
			if gate.requests.TryRLock() {
				return func() { gate.requests.RUnlock(); p.requests.RUnlock() }, true
			}
			p.requests.RUnlock()
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false
		case <-timer.C:
		}
	}
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

// enterSessionLibrary records the library currently opened by a page session
// and reports whether this is a new library entry (or a switch).
func (p *libraryRuntimeProvider) enterSessionLibrary(sessionID, libraryID string) bool {
	if p == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	libraryID = strings.TrimSpace(libraryID)
	p.mu.Lock()
	defer p.mu.Unlock()
	previous := strings.TrimSpace(p.sessions[sessionID])
	p.sessions[sessionID] = libraryID
	return libraryID != "" && previous != libraryID
}

func (p *libraryRuntimeProvider) ForLibrary(baseCfg *config.Config, library config.Library) (videoRegistrar, error) {
	if p == nil {
		return nil, fmt.Errorf("runtime provider is not configured")
	}
	release := p.beginRequest(library.ID)
	defer release()
	return p.forLibrary(baseCfg, library)
}

// The caller holds beginRequest for the entire use of the returned runtime.
func (p *libraryRuntimeProvider) forLibrary(baseCfg *config.Config, library config.Library) (registrarResult videoRegistrar, resultErr error) {
	if p == nil {
		return nil, fmt.Errorf("runtime provider is not configured")
	}
	libraryID := strings.TrimSpace(library.ID)
	if libraryID == "" {
		return nil, fmt.Errorf("library id is required")
	}
	p.mu.Lock()
	ready := p.services[libraryID]
	p.mu.Unlock()
	if ready != nil {
		return ready, nil
	}
	if service.IsSMBPath(library.Path) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		err := service.CheckLibraryReachable(ctx, library.Path)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("SMB library unavailable: %w", err)
		}
	}
	gate := p.libraryGate(libraryID)
	gate.init.Lock()
	defer gate.init.Unlock()
	p.mu.Lock()
	if registrar := p.services[libraryID]; registrar != nil {
		p.mu.Unlock()
		return registrar, nil
	}
	p.mu.Unlock()

	trace := librarySwitchLogger("open-runtime")
	trace("begin", "library=", library.ID, " path=", library.Path)
	defer func() { trace("finished", "error=", resultErr) }()
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
	trace("validate-identity", "database=", dbPath)
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
	trace("sqlite-open-start")
	repo, err := sqlite.NewPortableWorkingCopy(dbPath)
	if err != nil {
		return nil, err
	}
	trace("sqlite-open-complete")
	trace("metadata-start")
	if err := repo.EnsureLibraryMetadata(library.ID, library.Name, library.AccentColor); err != nil {
		_ = repo.Close()
		return nil, err
	}
	trace("metadata-complete")
	trace("initial-sync-start")
	if err := repo.SyncPortable(); err != nil {
		_ = repo.Close()
		return nil, err
	}
	trace("initial-sync-complete")
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

// Read-only check keeps failed switches eligible for diagnostics on retry.
func (p *libraryRuntimeProvider) sessionLibraryChanged(sessionID, libraryID string) bool {
	if p == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessions[sessionID] != libraryID
}
