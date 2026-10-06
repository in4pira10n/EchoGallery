package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type portableCopyCoordinator struct {
	source string
	cache  string
	db     *sql.DB
	refs   int
	stop   chan struct{}
	done   chan struct{}
	mu     sync.Mutex
	syncIO sync.Mutex
	dirty  bool
}

var portableCopies = struct {
	sync.Mutex
	items map[string]*portableCopyCoordinator
}{items: make(map[string]*portableCopyCoordinator)}

// NewPortableWorkingCopy runs a portable library database from the OS cache
// directory and keeps .echogallery/metadata.sqlite as its durable copy.
func NewPortableWorkingCopy(sourcePath string) (*DB, error) {
	sourcePath = filepath.Clean(sourcePath)
	coordinator, err := acquirePortableCopy(sourcePath)
	if err != nil {
		return nil, err
	}
	db, err := newDirect(coordinator.cache)
	if err != nil {
		releasePortableCopy(coordinator)
		return nil, err
	}
	db.portableCopy = coordinator
	coordinator.markDirty()
	return db, nil
}

func (s *DB) SyncPortable() error {
	if s == nil || s.portableCopy == nil {
		return nil
	}
	s.portableCopy.markDirty()
	return s.portableCopy.sync(true)
}

func (s *DB) WorkingPath() string {
	if s == nil || s.portableCopy == nil {
		return ""
	}
	return s.portableCopy.cache
}

func acquirePortableCopy(source string) (*portableCopyCoordinator, error) {
	portableCopies.Lock()
	defer portableCopies.Unlock()
	if existing := portableCopies.items[source]; existing != nil {
		existing.refs++
		return existing, nil
	}
	if _, err := os.Stat(source); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
			return nil, err
		}
		created, err := newDirect(source)
		if err != nil {
			return nil, err
		}
		if err := created.Close(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve system cache directory: %w", err)
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))[:24]
	dir := filepath.Join(root, "EchoGallery", "database", key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	cache := filepath.Join(dir, "metadata.sqlite")
	if err := snapshotSQLiteDatabase(source, cache); err != nil {
		return nil, fmt.Errorf("create SSD database working copy: %w", err)
	}
	monitor, err := sql.Open("sqlite", cache)
	if err != nil {
		return nil, err
	}
	monitor.SetMaxOpenConns(1)
	monitor.SetMaxIdleConns(1)
	c := &portableCopyCoordinator{source: source, cache: cache, db: monitor, refs: 1, stop: make(chan struct{}), done: make(chan struct{})}
	portableCopies.items[source] = c
	go c.run()
	return c, nil
}

func (c *portableCopyCoordinator) markDirty() {
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()
}

func (c *portableCopyCoordinator) run() {
	defer close(c.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var lastVersion int64
	_ = c.db.QueryRow(`PRAGMA data_version`).Scan(&lastVersion)
	for {
		select {
		case <-ticker.C:
			var version int64
			if err := c.db.QueryRow(`PRAGMA data_version`).Scan(&version); err == nil && version != lastVersion {
				lastVersion = version
				c.markDirty()
			}
			_ = c.sync(false)
		case <-c.stop:
			return
		}
	}
}

func (c *portableCopyCoordinator) sync(force bool) error {
	c.syncIO.Lock()
	defer c.syncIO.Unlock()
	c.mu.Lock()
	if !force && !c.dirty {
		c.mu.Unlock()
		return nil
	}
	c.dirty = false
	c.mu.Unlock()
	if err := snapshotSQLiteDatabase(c.cache, c.source); err != nil {
		c.markDirty()
		return err
	}
	return nil
}

func releasePortableCopy(c *portableCopyCoordinator) error {
	if c == nil {
		return nil
	}
	portableCopies.Lock()
	c.refs--
	if c.refs > 0 {
		portableCopies.Unlock()
		return nil
	}
	delete(portableCopies.items, c.source)
	portableCopies.Unlock()
	close(c.stop)
	<-c.done
	err := c.sync(true)
	closeErr := c.db.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func snapshotSQLiteDatabase(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	tmp := target + fmt.Sprintf(".snapshot-%d", time.Now().UnixNano())
	_ = os.Remove(tmp)
	db, err := sql.Open("sqlite", source)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`VACUUM INTO '` + strings.ReplaceAll(tmp, "'", "''") + `'`)
	closeErr := db.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return replaceDatabaseFile(tmp, target)
}

func replaceDatabaseFile(next, target string) error {
	backup := target + ".previous"
	_ = os.Remove(backup)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			_ = os.Remove(next)
			return err
		}
	}
	if err := os.Rename(next, target); err != nil {
		_ = os.Rename(backup, target)
		_ = os.Remove(next)
		return err
	}
	_ = os.Remove(backup)
	_ = os.Remove(target + "-wal")
	_ = os.Remove(target + "-shm")
	return nil
}
