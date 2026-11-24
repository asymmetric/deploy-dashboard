// Package store persists pin records, so the dashboard doesn't depend on
// ntfy's cache retention, and the last comin status seen per host, so a host
// that is offline (e.g. a laptop) still shows what it last ran.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/ntfy"
)

// seenSaveInterval bounds how often an unchanged comin status is written
// back, only to refresh its timestamp.
const seenSaveInterval = 5 * time.Minute

// Seen is the last successful scrape of a host's comin exporter.
type Seen struct {
	Status comin.Status `json:"status"`
	Time   time.Time    `json:"time"`
}

// Store keeps the latest pin record per host, the ntfy cursor and the last
// comin status per host in a JSON file.
type Store struct {
	path string

	mu   sync.Mutex
	data data
	// savedSeen is when each host's Seen was last written to disk.
	savedSeen map[string]time.Time
}

type data struct {
	Cursor string                    `json:"cursor"`
	Latest map[string]ntfy.PinRecord `json:"latest"`
	Seen   map[string]Seen           `json:"seen"`
}

// Open loads path, or starts empty if it doesn't exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path, savedSeen: map[string]time.Time{}}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	}
	if s.data.Latest == nil {
		s.data.Latest = map[string]ntfy.PinRecord{}
	}
	if s.data.Seen == nil {
		s.data.Seen = map[string]Seen{}
	}
	for host, seen := range s.data.Seen {
		s.savedSeen[host] = seen.Time
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

// Add records rs (in publish order) and writes the file.
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
	return s.save()
}

// LastSeen returns the last comin status scraped from host.
func (s *Store) LastSeen(host string) (Seen, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen, ok := s.data.Seen[host]
	return seen, ok
}

// Observe records a successful scrape of host at t. It writes the file when
// the status changed, or at most every seenSaveInterval otherwise.
func (s *Store) Observe(host string, st comin.Status, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.data.Seen[host]
	s.data.Seen[host] = Seen{Status: st, Time: t}
	if ok && prev.Status == st && t.Sub(s.savedSeen[host]) < seenSaveInterval {
		return nil
	}
	if err := s.save(); err != nil {
		return err
	}
	s.savedSeen[host] = t
	return nil
}

// save writes the file atomically. The caller holds s.mu.
func (s *Store) save() error {
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
