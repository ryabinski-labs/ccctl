// Package state persists slot state to ~/.ccctl/state.json atomically with mode 0600.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Version is the state file format this build reads and writes.
const Version = 1

type Slot struct {
	Slot       int       `json:"slot"`
	Task       string    `json:"task"`
	Cwd        string    `json:"cwd"`
	Repo       string    `json:"repo"`
	Worktree   string    `json:"worktree"`
	Branch     string    `json:"branch"`
	SessionID  string    `json:"session_id"`
	State      string    `json:"state"`
	LaunchedAt time.Time `json:"launched_at"`
}

type File struct {
	Version int    `json:"version"`
	Slots   []Slot `json:"slots"`
}

type Store struct {
	Path string
	// BeforeRename runs after the temp file is written and before it replaces
	// the real file; tests use it to simulate an interrupted save.
	BeforeRename func() error
	mu           sync.Mutex
}

func NewStore(dir string) *Store { return &Store{Path: filepath.Join(dir, "state.json")} }

// Load returns the saved state; a missing file is an empty state.
func (s *Store) Load() (File, error) {
	var f File
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{Version: Version}, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("read %s: %w", s.Path, err)
	}
	if f.Version > Version {
		return f, fmt.Errorf("State file version %d is newer than this ccctl supports.", f.Version)
	}
	return f, nil
}

// Save writes slots atomically: temp file, fsync, chmod 0600, rename.
func (s *Store) Save(slots []Slot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slots == nil {
		slots = []Slot{}
	}
	b, err := json.MarshalIndent(File{Version: Version, Slots: slots}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
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
	if s.BeforeRename != nil {
		if err := s.BeforeRename(); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), s.Path)
}
