package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableLeaseAvailabilityDoesNotConfuseFilesWithLocks(t *testing.T) {
	source := filepath.Join(t.TempDir(), "metadata.sqlite")
	if occupied, err := PortableLibraryOccupied(source); occupied || err != nil {
		t.Fatalf("missing lease: %v %v", occupied, err)
	}
	lease, err := acquirePortableLease(source)
	if err != nil {
		t.Fatal(err)
	}
	if occupied, err := PortableLibraryOccupied(source); !occupied || !errors.Is(err, ErrPortableLibraryOccupied) {
		t.Fatalf("live lease: %v %v", occupied, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if occupied, err := PortableLibraryOccupied(source); occupied || err != nil {
		t.Fatalf("unlocked lease file: %v %v", occupied, err)
	}
	if _, err := os.Stat(source + ".lease"); err != nil {
		t.Fatal("availability check deleted lease file")
	}
	if err := os.WriteFile(source+".lease", []byte("this is not a SQLite database"), 0644); err != nil {
		t.Fatal(err)
	}
	if occupied, err := PortableLibraryOccupied(source); occupied || err == nil || errors.Is(err, ErrPortableLibraryOccupied) {
		t.Fatalf("corrupt lease misreported as occupied: %v %v", occupied, err)
	}
}

func TestPortableLeaseOpenFailureIsNotOccupancy(t *testing.T) {
	source := filepath.Join(t.TempDir(), "missing", "metadata.sqlite")
	lease, err := acquirePortableLease(source)
	if lease != nil {
		lease.Close()
		t.Fatal("unexpected lease")
	}
	if err == nil || errors.Is(err, ErrPortableLibraryOccupied) {
		t.Fatalf("I/O failure misreported as contention: %v", err)
	}
}
