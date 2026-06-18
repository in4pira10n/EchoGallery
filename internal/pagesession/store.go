package pagesession

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const defaultTTL = 90 * time.Second

type Info struct {
	Username    string
	SessionID   string
	HeartbeatAt string
	CreatedAt   string
}

type ConflictError struct {
	Info Info
}

func (e *ConflictError) Error() string {
	username := strings.TrimSpace(e.Info.Username)
	if username == "" {
		username = "user"
	}
	return fmt.Sprintf("%s already has an active page session", username)
}

type Store struct {
	db  *sql.DB
	ttl time.Duration
}

func New(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("page session database path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create page session database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open page session database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable WAL for page session database: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set busy timeout for page session database: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS page_sessions (
			username     TEXT NOT NULL,
			session_id   TEXT NOT NULL,
			heartbeat_at TEXT NOT NULL,
			created_at   TEXT NOT NULL,
			PRIMARY KEY (username, session_id)
		);
		CREATE INDEX IF NOT EXISTS idx_page_sessions_username
			ON page_sessions(username, heartbeat_at);
		CREATE INDEX IF NOT EXISTS idx_page_sessions_session
			ON page_sessions(session_id, heartbeat_at);
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate page session database: %w", err)
	}
	return &Store{db: db, ttl: defaultTTL}, nil
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
	_, err := s.db.Exec(`DELETE FROM page_sessions WHERE heartbeat_at < ?`, cutoff)
	return err
}

func (s *Store) scanInfo(row *sql.Row) (*Info, error) {
	var info Info
	if err := row.Scan(&info.Username, &info.SessionID, &info.HeartbeatAt, &info.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &info, nil
}

func (s *Store) ActiveOther(username, sessionID string) (*Info, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	username = strings.TrimSpace(username)
	sessionID = strings.TrimSpace(sessionID)
	if username == "" {
		return nil, nil
	}
	if err := s.cleanupStale(); err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339Nano)
	return s.scanInfo(s.db.QueryRow(`
		SELECT username, session_id, heartbeat_at, created_at
		FROM page_sessions
		WHERE username = ?
		  AND session_id <> ?
		  AND heartbeat_at >= ?
		ORDER BY heartbeat_at DESC
		LIMIT 1
	`, username, sessionID, cutoff))
}

func (s *Store) ActiveSession(sessionID string) (*Info, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if err := s.cleanupStale(); err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339Nano)
	return s.scanInfo(s.db.QueryRow(`
		SELECT username, session_id, heartbeat_at, created_at
		FROM page_sessions
		WHERE session_id = ?
		  AND heartbeat_at >= ?
		ORDER BY heartbeat_at DESC
		LIMIT 1
	`, sessionID, cutoff))
}

func (s *Store) Register(username, sessionID string) (*Info, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	username = strings.TrimSpace(username)
	sessionID = strings.TrimSpace(sessionID)
	if username == "" || sessionID == "" {
		return nil, fmt.Errorf("page session requires username and session_id")
	}
	if err := s.cleanupStale(); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.Exec(`
		DELETE FROM page_sessions
		WHERE username = ? AND session_id <> ?
	`, username, sessionID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		DELETE FROM page_sessions
		WHERE session_id = ? AND username <> ?
	`, sessionID, username); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		INSERT INTO page_sessions(username, session_id, heartbeat_at, created_at)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(username, session_id) DO UPDATE SET
			heartbeat_at = excluded.heartbeat_at
	`, username, sessionID, now, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Info{
		Username:    username,
		SessionID:   sessionID,
		HeartbeatAt: now,
		CreatedAt:   now,
	}, nil
}

func (s *Store) Validate(username, sessionID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	username = strings.TrimSpace(username)
	sessionID = strings.TrimSpace(sessionID)
	if username == "" || sessionID == "" {
		return fmt.Errorf("page session requires username and session_id")
	}
	if err := s.cleanupStale(); err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339Nano)
	info, err := s.scanInfo(s.db.QueryRow(`
		SELECT username, session_id, heartbeat_at, created_at
		FROM page_sessions
		WHERE username = ?
		  AND session_id = ?
		  AND heartbeat_at >= ?
		LIMIT 1
	`, username, sessionID, cutoff))
	if err != nil {
		return err
	}
	if info == nil {
		if conflict, err := s.ActiveOther(username, sessionID); err != nil {
			return err
		} else if conflict != nil {
			return &ConflictError{Info: *conflict}
		}
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Release(username, sessionID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`
		DELETE FROM page_sessions
		WHERE username = ? AND session_id = ?
	`, strings.TrimSpace(username), strings.TrimSpace(sessionID))
	return err
}

func (s *Store) ReleaseSession(sessionID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM page_sessions WHERE session_id = ?`, strings.TrimSpace(sessionID))
	return err
}
