package sessionlock

import (
	"path/filepath"
	"testing"

	"echogallery/internal/config"
)

func TestLockedByOther_IgnoresLocksFromSameSession(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	library := config.Library{ID: "lib_a", Name: "Library A", Path: "/tmp/library-a"}
	if _, err := store.Acquire(library, "admin-a", "admin", "session-shared", "browse"); err != nil {
		t.Fatalf("failed to acquire initial lock: %v", err)
	}

	conflict, err := store.LockedByOther(library.ID, "admin-b", "admin", "session-shared", "browse")
	if err != nil {
		t.Fatalf("LockedByOther returned error: %v", err)
	}
	if conflict != nil {
		t.Fatalf("expected same-session lock to be ignored, got conflict from %q", conflict.OwnerUsername)
	}

	if _, err := store.Acquire(library, "admin-b", "admin", "session-shared", "browse"); err != nil {
		t.Fatalf("expected same-session acquire to succeed, got: %v", err)
	}
}

func TestLockedByOther_BlocksDifferentSession(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	library := config.Library{ID: "lib_a", Name: "Library A", Path: "/tmp/library-a"}
	if _, err := store.Acquire(library, "admin-a", "admin", "session-a", "browse"); err != nil {
		t.Fatalf("failed to acquire initial lock: %v", err)
	}

	conflict, err := store.LockedByOther(library.ID, "admin-b", "admin", "session-b", "browse")
	if err != nil {
		t.Fatalf("LockedByOther returned error: %v", err)
	}
	if conflict == nil {
		t.Fatalf("expected different-session conflict, got nil")
	}
	if conflict.OwnerUsername != "admin-a" {
		t.Fatalf("expected owner admin-a, got %q", conflict.OwnerUsername)
	}
}

func TestLockedLibraryIDsExcept_IgnoresCurrentSession(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	libraryA := config.Library{ID: "lib_a", Name: "Library A", Path: "/tmp/library-a"}
	libraryB := config.Library{ID: "lib_b", Name: "Library B", Path: "/tmp/library-b"}
	if _, err := store.Acquire(libraryA, "admin-a", "admin", "session-shared", "browse"); err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	if _, err := store.Acquire(libraryB, "admin-c", "admin", "session-other", "browse"); err != nil {
		t.Fatalf("failed to acquire second lock: %v", err)
	}

	locked, err := store.LockedLibraryIDsExcept("admin-b", "session-shared")
	if err != nil {
		t.Fatalf("LockedLibraryIDsExcept returned error: %v", err)
	}
	if _, exists := locked[libraryA.ID]; exists {
		t.Fatalf("expected same-session library to be ignored")
	}
	if _, exists := locked[libraryB.ID]; !exists {
		t.Fatalf("expected different-session library to remain locked")
	}
}

func TestAcquire_AllowsSharedVisitorBrowse(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	library := config.Library{ID: "lib_a", Name: "Library A", Path: "/tmp/library-a"}
	if _, err := store.Acquire(library, "visitor-a", "visitor", "session-a", "browse"); err != nil {
		t.Fatalf("failed to acquire first visitor lock: %v", err)
	}
	if _, err := store.Acquire(library, "visitor-b", "visitor", "session-b", "browse"); err != nil {
		t.Fatalf("expected second visitor browse to share library: %v", err)
	}
}

func TestAcquire_AllowsAdminWhenVisitorBrowsing(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	library := config.Library{ID: "lib_a", Name: "Library A", Path: "/tmp/library-a"}
	if _, err := store.Acquire(library, "visitor-a", "visitor", "session-a", "browse"); err != nil {
		t.Fatalf("failed to acquire visitor lock: %v", err)
	}
	if _, err := store.Acquire(library, "admin-b", "admin", "session-b", "browse"); err != nil {
		t.Fatalf("expected admin browse to ignore visitor browse, got: %v", err)
	}
}

func TestAcquire_AllowsVisitorWhenAdminBrowsing(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	library := config.Library{ID: "lib_a", Name: "Library A", Path: "/tmp/library-a"}
	if _, err := store.Acquire(library, "admin-a", "admin", "session-a", "browse"); err != nil {
		t.Fatalf("failed to acquire admin lock: %v", err)
	}
	if _, err := store.Acquire(library, "visitor-b", "visitor", "session-b", "browse"); err != nil {
		t.Fatalf("expected visitor browse to ignore admin browse, got: %v", err)
	}
}

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	dir := t.TempDir()
	store, err := New(filepath.Join(dir, "locks.db"))
	if err != nil {
		t.Fatalf("failed to create lock store: %v", err)
	}
	cleanup := func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close lock store: %v", err)
		}
	}
	return store, cleanup
}
