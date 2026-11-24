// Package store persists pin records, so the dashboard doesn't depend on
// ntfy's cache retention.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/asymmetric/deploy-dashboard/internal/ntfy"
)

// Store keeps the latest pin record per host and the ntfy cursor in a JSON file.
type Store struct {
	path string

	mu   sync.Mutex
	data data
}

type data struct {
	Cursor string                    `json:"cursor"`
	Latest map[string]ntfy.PinRecord `json:"latest"`
}

// Open loads path, or starts empty if it doesn't exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: data{Latest: map[string]ntfy.PinRecord{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, err
	}
	if s.data.Latest == nil {
		s.data.Latest = map[string]ntfy.PinRecord{}
	}
	return s, nil
}

// Cursor returns the id of the last ntfy message seen.
func (s *Store) Cursor() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Cursor
}

// Latest returns the most recent record for each host.
func (s *Store) Latest() map[string]ntfy.PinRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.data.Latest)
}

// Add records rs (in publish order) and writes the file atomically.
func (s *Store) Add(rs []ntfy.PinRecord) error {
	if len(rs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rs {
		s.data.Latest[r.Host] = r
		s.data.Cursor = r.MsgID
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
