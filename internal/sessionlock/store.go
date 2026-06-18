package sessionlock

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"echogallery/internal/config"
)

const (
	defaultTTL         = 90 * time.Second
	defaultScopeBrowse = "browse"
	roleAdmin          = "admin"
	roleVisitor        = "visitor"
)

type Info struct {
	LibraryID     string
	LibraryName   string
	LibraryPath   string
	OwnerUsername string
	OwnerRole     string
	SessionID     string
	Scope         string
	HeartbeatAt   string
	CreatedAt     string
}

type ConflictError struct {
	Info Info
}

func (e *ConflictError) Error() string {
	name := strings.TrimSpace(e.Info.LibraryName)
	if name == "" {
		name = strings.TrimSpace(e.Info.LibraryID)
	}
	if name == "" {
		name = "resource library"
	}
	owner := strings.TrimSpace(e.Info.OwnerUsername)
	if owner == "" {
		owner = "another admin"
	}
	return fmt.Sprintf("%s is currently in use by %s", name, owner)
}

type Store struct {
	db  *sql.DB
	ttl time.Duration
}

func New(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("lock database path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable WAL for lock database: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set busy timeout for lock database: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS library_session_locks (
			library_id     TEXT NOT NULL,
			session_id     TEXT NOT NULL,
			scope          TEXT NOT NULL,
			library_name   TEXT NOT NULL DEFAULT '',
			library_path   TEXT NOT NULL DEFAULT '',
			owner_username TEXT NOT NULL,
			owner_role     TEXT NOT NULL DEFAULT 'admin',
			heartbeat_at   TEXT NOT NULL,
			created_at     TEXT NOT NULL,
			PRIMARY KEY (library_id, session_id, scope)
		);
		CREATE INDEX IF NOT EXISTS idx_library_session_locks_library
			ON library_session_locks(library_id, heartbeat_at);
		CREATE INDEX IF NOT EXISTS idx_library_session_locks_session
			ON library_session_locks(session_id, heartbeat_at);
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate lock database: %w", err)
	}
	if err := migrateLegacySchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to upgrade lock database: %w", err)
	}
	return &Store{db: db, ttl: defaultTTL}, nil
}

func migrateLegacySchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	hasOwnerRole, err := tableHasColumn(db, "library_session_locks", "owner_role")
	if err != nil {
		return err
	}
	if !hasOwnerRole {
		if _, err := db.Exec(`ALTER TABLE library_session_locks ADD COLUMN owner_role TEXT NOT NULL DEFAULT 'admin'`); err != nil {
			return err
		}
	}
	_, err = db.Exec(`UPDATE library_session_locks SET owner_role = 'admin' WHERE TRIM(COALESCE(owner_role, '')) = ''`)
	return err
}

func tableHasColumn(db *sql.DB, tableName string, columnName string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(columnName)) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) cleanupStale() error {
	if s == nil || s.db == nil {
		return nil
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339Nano)
	_, err := s.db.Exec(`DELETE FROM library_session_locks WHERE heartbeat_at < ?`, cutoff)
	return err
}

func (s *Store) scanInfo(row *sql.Row) (*Info, error) {
	var info Info
	if err := row.Scan(
		&info.LibraryID,
		&info.LibraryName,
		&info.LibraryPath,
		&info.OwnerUsername,
		&info.OwnerRole,
		&info.SessionID,
		&info.Scope,
		&info.HeartbeatAt,
		&info.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &info, nil
}

func normalizeScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return defaultScopeBrowse
	}
	return scope
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case roleVisitor:
		return roleVisitor
	default:
		return roleAdmin
	}
}

func shouldConflict(scope, requesterRole string, existing Info, requesterUsername, requesterSessionID string) bool {
	if strings.TrimSpace(existing.SessionID) == strings.TrimSpace(requesterSessionID) {
		return false
	}
	if strings.TrimSpace(existing.OwnerUsername) == strings.TrimSpace(requesterUsername) {
		return false
	}
	scope = normalizeScope(scope)
	requesterRole = normalizeRole(requesterRole)
	existingRole := normalizeRole(existing.OwnerRole)
	existingScope := normalizeScope(existing.Scope)
	if scope != defaultScopeBrowse || existingScope != defaultScopeBrowse {
		return true
	}
	if requesterRole == roleVisitor || existingRole == roleVisitor {
		return false
	}
	return true
}

func (s *Store) LockedByOther(libraryID, ownerUsername, ownerRole, sessionID, scope string) (*Info, error) {
	if s == nil || s.db == nil || strings.TrimSpace(libraryID) == "" {
		return nil, nil
	}
	if err := s.cleanupStale(); err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT library_id, library_name, library_path, owner_username, owner_role, session_id, scope, heartbeat_at, created_at
		FROM library_session_locks
		WHERE library_id = ?
		  AND heartbeat_at >= ?
		ORDER BY heartbeat_at DESC
	`, strings.TrimSpace(libraryID), cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var info Info
		if err := rows.Scan(
			&info.LibraryID,
			&info.LibraryName,
			&info.LibraryPath,
			&info.OwnerUsername,
			&info.OwnerRole,
			&info.SessionID,
			&info.Scope,
			&info.HeartbeatAt,
			&info.CreatedAt,
		); err != nil {
			return nil, err
		}
		if shouldConflict(scope, ownerRole, info, ownerUsername, sessionID) {
			return &info, nil
		}
	}
	return nil, rows.Err()
}

func (s *Store) Acquire(library config.Library, ownerUsername, ownerRole, sessionID, scope string) (*Info, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	libraryID := strings.TrimSpace(library.ID)
	ownerUsername = strings.TrimSpace(ownerUsername)
	ownerRole = normalizeRole(ownerRole)
	sessionID = strings.TrimSpace(sessionID)
	scope = normalizeScope(scope)
	if libraryID == "" || ownerUsername == "" || sessionID == "" {
		return nil, fmt.Errorf("library lock requires library_id, owner username, and session id")
	}
	if err := s.cleanupStale(); err != nil {
		return nil, err
	}
	if conflict, err := s.LockedByOther(libraryID, ownerUsername, ownerRole, sessionID, scope); err != nil {
		return nil, err
	} else if conflict != nil {
		return nil, &ConflictError{Info: *conflict}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`
		INSERT INTO library_session_locks(
			library_id, session_id, scope, library_name, library_path, owner_username, owner_role, heartbeat_at, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(library_id, session_id, scope) DO UPDATE SET
			library_name = excluded.library_name,
			library_path = excluded.library_path,
			owner_username = excluded.owner_username,
			owner_role = excluded.owner_role,
			heartbeat_at = excluded.heartbeat_at
	`, libraryID, sessionID, scope, strings.TrimSpace(library.Name), strings.TrimSpace(library.Path), ownerUsername, ownerRole, now, now); err != nil {
		return nil, err
	}
	return &Info{
		LibraryID:     libraryID,
		LibraryName:   strings.TrimSpace(library.Name),
		LibraryPath:   strings.TrimSpace(library.Path),
		OwnerUsername: ownerUsername,
		OwnerRole:     ownerRole,
		SessionID:     sessionID,
		Scope:         scope,
		HeartbeatAt:   now,
		CreatedAt:     now,
	}, nil
}

func (s *Store) Release(libraryID, sessionID, scope string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`
		DELETE FROM library_session_locks
		WHERE library_id = ? AND session_id = ? AND scope = ?
	`, strings.TrimSpace(libraryID), strings.TrimSpace(sessionID), normalizeScope(scope))
	return err
}

func (s *Store) ReleaseSession(sessionID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM library_session_locks WHERE session_id = ?`, strings.TrimSpace(sessionID))
	return err
}

func (s *Store) ActiveForLibrary(libraryID string) ([]Info, error) {
	if s == nil || s.db == nil || strings.TrimSpace(libraryID) == "" {
		return nil, nil
	}
	if err := s.cleanupStale(); err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT library_id, library_name, library_path, owner_username, owner_role, session_id, scope, heartbeat_at, created_at
		FROM library_session_locks
		WHERE library_id = ?
		  AND heartbeat_at >= ?
		ORDER BY heartbeat_at DESC
	`, strings.TrimSpace(libraryID), cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Info, 0, 4)
	for rows.Next() {
		var info Info
		if err := rows.Scan(
			&info.LibraryID,
			&info.LibraryName,
			&info.LibraryPath,
			&info.OwnerUsername,
			&info.OwnerRole,
			&info.SessionID,
			&info.Scope,
			&info.HeartbeatAt,
			&info.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, info)
	}
	return items, rows.Err()
}

func (s *Store) LockedLibraryIDsExcept(ownerUsername, sessionID string) (map[string]Info, error) {
	result := map[string]Info{}
	if s == nil || s.db == nil {
		return result, nil
	}
	if err := s.cleanupStale(); err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT library_id, library_name, library_path, owner_username, owner_role, session_id, scope, heartbeat_at, created_at
		FROM library_session_locks
		WHERE heartbeat_at >= ?
		ORDER BY heartbeat_at DESC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var info Info
		if err := rows.Scan(
			&info.LibraryID,
			&info.LibraryName,
			&info.LibraryPath,
			&info.OwnerUsername,
			&info.OwnerRole,
			&info.SessionID,
			&info.Scope,
			&info.HeartbeatAt,
			&info.CreatedAt,
		); err != nil {
			return nil, err
		}
		if !shouldConflict(defaultScopeBrowse, roleAdmin, info, ownerUsername, sessionID) {
			continue
		}
		if _, exists := result[info.LibraryID]; !exists {
			result[info.LibraryID] = info
		}
	}
	return result, rows.Err()
}

func (s *Store) ReleaseScopeForSessionExcept(sessionID, scope, keepLibraryID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`
		DELETE FROM library_session_locks
		WHERE session_id = ?
		  AND scope = ?
		  AND library_id <> ?
	`, strings.TrimSpace(sessionID), normalizeScope(scope), strings.TrimSpace(keepLibraryID))
	return err
}

func (s *Store) CleanupObsolete(cfg *config.Config) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if err := s.cleanupStale(); err != nil {
		return 0, err
	}
	if cfg == nil || len(cfg.Libraries) == 0 {
		result, err := s.db.Exec(`DELETE FROM library_session_locks`)
		if err != nil {
			return 0, err
		}
		affected, _ := result.RowsAffected()
		return int(affected), nil
	}
	valid := make(map[string]struct{}, len(cfg.Libraries))
	for _, library := range cfg.Libraries {
		if id := strings.TrimSpace(library.ID); id != "" {
			valid[id] = struct{}{}
		}
	}
	rows, err := s.db.Query(`SELECT DISTINCT library_id FROM library_session_locks`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	totalDeleted := 0
	for rows.Next() {
		var libraryID string
		if err := rows.Scan(&libraryID); err != nil {
			return totalDeleted, err
		}
		if _, ok := valid[strings.TrimSpace(libraryID)]; ok {
			continue
		}
		result, err := s.db.Exec(`DELETE FROM library_session_locks WHERE library_id = ?`, strings.TrimSpace(libraryID))
		if err != nil {
			return totalDeleted, err
		}
		affected, _ := result.RowsAffected()
		totalDeleted += int(affected)
	}
	return totalDeleted, rows.Err()
}
