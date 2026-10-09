package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrPortableLibraryOccupied = errors.New("resource library is in use by another Gallery")

// Availability is advisory; opening a working copy still acquires the lease.
func PortableLibraryOccupied(source string) bool {
	source = filepath.Clean(source)
	portableCopies.Lock()
	defer portableCopies.Unlock()
	if portableCopies.items[source] != nil {
		return false
	}
	if _, err := os.Stat(source + ".lease"); err != nil {
		return false
	}
	lease, err := acquirePortableLease(source)
	if err != nil {
		return true
	}
	_ = lease.Close()
	return false
}

// Hold a SQLite rollback-journal exclusive lock for the entire working-copy
// lifetime. The OS releases it on process death; the file is never deleted.
func acquirePortableLease(source string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", source+".lease")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec(`PRAGMA busy_timeout=0`); err == nil {
		_, err = db.Exec(`BEGIN EXCLUSIVE`)
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w (working-copy lease): %v", ErrPortableLibraryOccupied, err)
	}
	return db, nil
}
