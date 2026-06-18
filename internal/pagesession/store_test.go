package pagesession

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestRegister_AllowsSamePageRefresh(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if _, err := store.Register("alice", "page-a"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if _, err := store.Register("alice", "page-a"); err != nil {
		t.Fatalf("same page refresh should succeed: %v", err)
	}
}

func TestRegister_ReplacesDifferentPageForSameUser(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if _, err := store.Register("alice", "page-a"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if _, err := store.Register("alice", "page-b"); err != nil {
		t.Fatalf("second register should replace previous page session: %v", err)
	}
	if err := store.Validate("alice", "page-a"); err == nil {
		t.Fatal("expected previous page session to become invalid")
	} else if _, ok := err.(*ConflictError); !ok {
		t.Fatalf("expected ConflictError for replaced page session, got %T", err)
	}
	if err := store.Validate("alice", "page-b"); err != nil {
		t.Fatalf("expected replacement page session to stay valid: %v", err)
	}
}

func TestValidate_ReturnsConflictWhenOtherPageOwnsUser(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if _, err := store.Register("alice", "page-a"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	err := store.Validate("alice", "page-b")
	if err == nil {
		t.Fatal("expected conflict")
	}
	if _, ok := err.(*ConflictError); !ok {
		t.Fatalf("expected ConflictError, got %T", err)
	}
}

func TestRegister_MovesReusedPageSessionToNewUser(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if _, err := store.Register("alice", "page-a"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if _, err := store.Register("bob", "page-a"); err != nil {
		t.Fatalf("reused page id should move to new user: %v", err)
	}
	if err := store.Validate("alice", "page-a"); err == nil {
		t.Fatal("expected old user mapping to be removed")
	} else if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows for old mapping, got %v", err)
	}
	info, err := store.ActiveSession("page-a")
	if err != nil {
		t.Fatalf("active session lookup failed: %v", err)
	}
	if info == nil || info.Username != "bob" {
		t.Fatalf("expected reused page session to belong to bob, got %#v", info)
	}
}

func TestRelease_RemovesSession(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if _, err := store.Register("alice", "page-a"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := store.Release("alice", "page-a"); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	err := store.Validate("alice", "page-a")
	if err == nil {
		t.Fatal("expected missing session after release")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows after release, got %v", err)
	}
}

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	dir := t.TempDir()
	store, err := New(filepath.Join(dir, "page-sessions.db"))
	if err != nil {
		t.Fatalf("failed to create page session store: %v", err)
	}
	cleanup := func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close page session store: %v", err)
		}
	}
	return store, cleanup
}
