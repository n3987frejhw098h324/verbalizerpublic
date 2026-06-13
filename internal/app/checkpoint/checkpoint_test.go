package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyIgnoresIDSetAndDistinguishesBatch(t *testing.T) {
	if Key(100, "Sound", false) != Key(100, "Sound", false) {
		t.Fatal("key for the same creator/type/group should be stable")
	}

	if Key(100, "Sound", false) == Key(101, "Sound", false) {
		t.Fatal("different creators should produce different keys")
	}
	if Key(100, "Sound", false) == Key(100, "Mesh", false) {
		t.Fatal("different asset types should produce different keys")
	}
	if Key(100, "Sound", false) == Key(100, "Sound", true) {
		t.Fatal("group flag should affect the key")
	}
}

func TestRecordSurvivesReload(t *testing.T) {
	dir := t.TempDir()

	s, err := loadFromDir(dir, "k")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := s.Get(1); ok {
		t.Fatal("fresh store should have nothing")
	}
	if err := s.Record(1, 1001); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := s.Record(2, 1002); err != nil {
		t.Fatalf("record: %v", err)
	}

	reloaded, err := loadFromDir(dir, "k")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got, ok := reloaded.Get(1); !ok || got != 1001 {
		t.Fatalf("Get(1) = %d, %v; want 1001, true", got, ok)
	}
	if got, ok := reloaded.Get(2); !ok || got != 1002 {
		t.Fatalf("Get(2) = %d, %v; want 1002, true", got, ok)
	}
	if reloaded.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", reloaded.Count())
	}
}

func TestCleanupRemovesFile(t *testing.T) {
	dir := t.TempDir()
	s, _ := loadFromDir(dir, "k")
	if err := s.Record(1, 2); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := os.Stat(s.path); err != nil {
		t.Fatalf("checkpoint file should exist: %v", err)
	}
	if err := s.Cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(s.path); !os.IsNotExist(err) {
		t.Fatalf("checkpoint file should be gone, stat err = %v", err)
	}
}

func TestCorruptFileStartsFresh(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "reupload-k.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadFromDir(dir, "k")
	if err == nil {
		t.Fatal("expected an error reporting the corrupt file")
	}
	if s == nil || s.Count() != 0 {
		t.Fatal("corrupt file should yield a usable, empty store")
	}
	if err := s.Record(5, 6); err != nil {
		t.Fatalf("record after corrupt load: %v", err)
	}
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	if _, ok := s.Get(1); ok {
		t.Fatal("nil store Get should report missing")
	}
	if err := s.Record(1, 2); err != nil {
		t.Fatalf("nil store Record should be a no-op: %v", err)
	}
	if s.Count() != 0 {
		t.Fatal("nil store Count should be 0")
	}
	if err := s.Cleanup(); err != nil {
		t.Fatalf("nil store Cleanup should be a no-op: %v", err)
	}
}
