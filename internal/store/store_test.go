package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
)

func TestObservePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	st := comin.Status{StorePath: "/nix/store/a", DeployStatus: comin.StatusDone}
	if err := s.Observe("ogaki", st, t0); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.LastSeen("ogaki")
	if !ok || got.Status != st || !got.Time.Equal(t0) {
		t.Fatalf("LastSeen after reopen = %+v, %v", got, ok)
	}
}

func TestObserveThrottlesUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	st := comin.Status{StorePath: "/nix/store/a", DeployStatus: comin.StatusDone}
	onDisk := func() Seen {
		t.Helper()
		s2, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := s2.LastSeen("ogaki")
		return got
	}
	if err := s.Observe("ogaki", st, t0); err != nil {
		t.Fatal(err)
	}

	// unchanged and soon after: memory updates, the file doesn't
	t1 := t0.Add(time.Minute)
	if err := s.Observe("ogaki", st, t1); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LastSeen("ogaki"); !got.Time.Equal(t1) {
		t.Errorf("in-memory time = %v, want %v", got.Time, t1)
	}
	if got := onDisk(); !got.Time.Equal(t0) {
		t.Errorf("file time = %v, want %v (unchanged status within %v)", got.Time, t0, seenSaveInterval)
	}

	// unchanged but past the interval: written
	t2 := t0.Add(seenSaveInterval)
	if err := s.Observe("ogaki", st, t2); err != nil {
		t.Fatal(err)
	}
	if got := onDisk(); !got.Time.Equal(t2) {
		t.Errorf("file time = %v, want %v", got.Time, t2)
	}

	// changed: written right away
	st.DeployStatus = comin.StatusRunning
	t3 := t2.Add(time.Second)
	if err := s.Observe("ogaki", st, t3); err != nil {
		t.Fatal(err)
	}
	if got := onDisk(); got.Status != st {
		t.Errorf("file status = %+v, want %+v", got.Status, st)
	}
}
