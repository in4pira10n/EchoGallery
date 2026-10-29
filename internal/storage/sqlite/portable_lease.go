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
func PortableLibraryOccupied(source string) (bool, error) {
	source = filepath.Clean(source)
	portableCopies.Lock()
	defer portableCopies.Unlock()
	if portableCopies.items[source] != nil {
		return false, nil
	}
	if _, err := os.Stat(source + ".lease"); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect library lease: %w", err)
	}
	lease, err := acquirePortableLease(source)
	if err != nil {
		return errors.Is(err, ErrPortableLibraryOccupied), err
	}
	_ = lease.Close()
	return false, nil
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
		var coded interface{ Code() int }
		if errors.As(err, &coded) && (coded.Code()&0xff == 5 || coded.Code()&0xff == 6) {
			return nil, fmt.Errorf("%w (working-copy lease): %v", ErrPortableLibraryOccupied, err)
		}
		return nil, fmt.Errorf("open working-copy lease: %w", err)
	}
	return db, nil
}
