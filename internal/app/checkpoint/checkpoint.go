package checkpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const dirName = "checkpoints"

func Key(creatorID int64, assetType string, isGroup bool) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d\n%s\n%t\n", creatorID, assetType, isGroup)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

type Store struct {
	path string

	mu      sync.RWMutex
	results map[int64]int64
}

func Load(key string) (*Store, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return loadFromDir(filepath.Join(dir, dirName), key)
}

func loadFromDir(dir, key string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	s := &Store{
		path:    filepath.Join(dir, "reupload-"+key+".json"),
		results: make(map[int64]int64),
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.results); err != nil {
		s.results = make(map[int64]int64)
		return s, fmt.Errorf("checkpoint %s is unreadable, starting fresh: %w", s.path, err)
	}
	return s, nil
}

func (s *Store) Get(oldID int64) (int64, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	newID, ok := s.results[oldID]
	return newID, ok
}

func (s *Store) Record(oldID, newID int64) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[oldID] = newID
	return s.flushLocked()
}

func (s *Store) flushLocked() error {
	data, err := json.Marshal(s.results)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), "reupload-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *Store) Count() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.results)
}

func (s *Store) Cleanup() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
