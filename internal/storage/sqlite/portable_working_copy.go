package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	sqlitedriver "modernc.org/sqlite"
)

type portableCopyCoordinator struct {
	source  string
	cache   string
	pending string
	db      *sql.DB
	lease   *sql.DB
	refs    int
	stop    chan struct{}
	done    chan struct{}
	syncNow chan struct{}
	syncIO  sync.Mutex
	// data_version is compared on the same persistent monitor connection.
	syncedVersion int64
}

var portableCopies = struct {
	sync.Mutex
	items map[string]*portableCopyCoordinator
	gates map[string]*sync.Mutex
}{items: make(map[string]*portableCopyCoordinator), gates: make(map[string]*sync.Mutex)}

// Only coordinate opening/closing the same source; disk I/O must not hold the
// process-wide registry mutex and block unrelated libraries.
func portableSourceGate(source string) *sync.Mutex {
	portableCopies.Lock()
	defer portableCopies.Unlock()
	gate := portableCopies.gates[source]
	if gate == nil {
		gate = &sync.Mutex{}
		portableCopies.gates[source] = gate
	}
	return gate
}

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
	return db, nil
}

func (s *DB) SyncPortable() error {
	if s == nil || s.portableCopy == nil {
		return nil
	}
	return s.portableCopy.sync()
}

// SchedulePortableSync records that the working copy has newer data, then
// lets the coordinator persist it without blocking the user-facing mutation.
// The marker also makes the working copy recoverable after an unexpected exit.
func (s *DB) SchedulePortableSync() error {
	if s == nil || s.portableCopy == nil {
		return nil
	}
	c := s.portableCopy
	if err := os.WriteFile(c.pending, []byte("pending\n"), 0600); err != nil {
		return fmt.Errorf("record pending library database sync: %w", err)
	}
	select {
	case c.syncNow <- struct{}{}:
	default:
	}
	return nil
}

func (s *DB) WorkingPath() string {
	if s == nil || s.portableCopy == nil {
		return ""
	}
	return s.portableCopy.cache
}

func acquirePortableCopy(source string) (*portableCopyCoordinator, error) {
	gate := portableSourceGate(source)
	gate.Lock()
	defer gate.Unlock()
	portableCopies.Lock()
	existing := portableCopies.items[source]
	portableCopies.Unlock()
	if existing != nil {
		existing.refs++
		return existing, nil
	}
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		return nil, err
	}
	lease, err := acquirePortableLease(source)
	if err != nil {
		return nil, err
	}
	owned := false
	defer func() {
		if !owned {
			_ = lease.Close()
		}
	}()
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
	// Another server process must never replace a database we have open.
	dir := filepath.Join(root, "EchoGallery", "database", key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	cache := filepath.Join(dir, "metadata.sqlite")
	pending := filepath.Join(dir, "sync-pending")
	if _, err := os.Stat(pending); err == nil {
		// A prior shutdown could not persist this working copy. The lease proves
		// no other EchoGallery process currently owns the source database.
		if _, err := os.Stat(cache); err != nil {
			return nil, fmt.Errorf("pending library sync has no recoverable working copy: %w", err)
		}
		if err := snapshotSQLiteDatabase(cache, source); err != nil {
			return nil, fmt.Errorf("recover pending library database sync: %w", err)
		}
		if err := os.Remove(pending); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("clear recovered library sync marker: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect pending library sync: %w", err)
	}
	if err := snapshotSQLiteDatabase(source, cache); err != nil {
		return nil, fmt.Errorf("create SSD database working copy: %w", err)
	}
	monitor, err := sql.Open("sqlite", cache)
	if err != nil {
		return nil, err
	}
	monitor.SetMaxOpenConns(1)
	monitor.SetMaxIdleConns(1)
	// Set working-copy journal mode before the baseline. This local storage
	// detail is not a content change that needs copying back to the library.
	if _, err := monitor.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = monitor.Close()
		return nil, err
	}
	var version int64
	if err := monitor.QueryRow(`PRAGMA data_version`).Scan(&version); err != nil {
		_ = monitor.Close()
		return nil, err
	}
	c := &portableCopyCoordinator{source: source, cache: cache, pending: pending, db: monitor, lease: lease, refs: 1, syncedVersion: version, stop: make(chan struct{}), done: make(chan struct{}), syncNow: make(chan struct{}, 1)}
	portableCopies.Lock()
	portableCopies.items[source] = c
	portableCopies.Unlock()
	owned = true
	go c.run()
	return c, nil
}

func (c *portableCopyCoordinator) run() {
	defer close(c.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = c.sync()
		case <-c.syncNow:
			_ = c.sync()
		case <-c.stop:
			return
		}
	}
}

func (c *portableCopyCoordinator) sync() error {
	c.syncIO.Lock()
	defer c.syncIO.Unlock()
	var version int64
	if err := c.db.QueryRow(`PRAGMA data_version`).Scan(&version); err != nil {
		return err
	}
	if version == c.syncedVersion {
		if err := os.Remove(c.pending); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clear library database sync marker: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(c.pending, []byte("pending\n"), 0600); err != nil {
		return fmt.Errorf("record pending library database sync: %w", err)
	}
	if err := snapshotSQLiteDatabase(c.cache, c.source); err != nil {
		return err // Keep the old version and the lease so a retry persists changes.
	}
	c.syncedVersion = version
	var latestVersion int64
	if err := c.db.QueryRow(`PRAGMA data_version`).Scan(&latestVersion); err != nil {
		return err
	}
	if latestVersion != version {
		select {
		case c.syncNow <- struct{}{}:
		default:
		}
		return nil
	}
	if err := os.Remove(c.pending); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear library database sync marker: %w", err)
	}
	return nil
}

func releasePortableCopy(c *portableCopyCoordinator) error {
	if c == nil {
		return nil
	}
	gate := portableSourceGate(c.source)
	gate.Lock()
	defer gate.Unlock()
	if c.refs > 1 {
		c.refs--
		return nil
	}
	// On failure retain the coordinator and lease for a safe retry.
	if err := c.sync(); err != nil {
		return err
	}
	portableCopies.Lock()
	delete(portableCopies.items, c.source)
	portableCopies.Unlock()
	close(c.stop)
	<-c.done
	closeErr := c.db.Close()
	leaseErr := c.lease.Close()
	if closeErr != nil {
		return closeErr
	}
	return leaseErr
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
	if _, err := db.Exec(`PRAGMA busy_timeout=10000`); err != nil {
		_ = db.Close()
		return err
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		_ = db.Close()
		return err
	}
	err = conn.Raw(func(raw any) error {
		backuper, ok := raw.(interface {
			NewBackup(string) (*sqlitedriver.Backup, error)
		})
		if !ok {
			return fmt.Errorf("SQLite driver does not support online backups")
		}
		backup, err := backuper.NewBackup(tmp)
		if err != nil {
			return err
		}
		var stepErr error
		for {
			more, err := backup.Step(-1)
			if err != nil {
				stepErr = err
				break
			}
			if !more {
				break
			}
		}
		finishErr := backup.Finish()
		if stepErr != nil {
			return stepErr
		}
		return finishErr
	})
	connErr := conn.Close()
	closeErr := db.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if connErr != nil {
		_ = os.Remove(tmp)
		return connErr
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
