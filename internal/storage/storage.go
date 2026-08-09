// Package storage loads and saves the habit file.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fhightower/habit-warrior/internal/model"
)

// EnvPath overrides the default data file location.
const EnvPath = "HABIT_WARRIOR_DATA"

// DefaultPath is $HABIT_WARRIOR_DATA, else $XDG_DATA_HOME/habit-warrior/habits.json,
// else ~/.local/share/habit-warrior/habits.json.
func DefaultPath() (string, error) {
	if p := os.Getenv(EnvPath); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot locate home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "habit-warrior", "habits.json"), nil
}

// Load reads the store at path. A missing file is an empty store, not an
// error. A corrupt file is an error: it is never silently replaced.
func Load(path string) (*model.Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return model.NewStore(), nil
		}
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if len(data) == 0 {
		return model.NewStore(), nil
	}

	var s model.Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s is not valid habit data (refusing to overwrite it): %w", path, err)
	}
	if s.Version > model.Version {
		return nil, fmt.Errorf("%s was written by a newer version of hw (file version %d, this build understands %d)", path, s.Version, model.Version)
	}
	if s.Version == 0 {
		s.Version = model.Version
	}
	if s.Habits == nil {
		s.Habits = []model.Habit{}
	}
	// Guard against a hand-edited file whose next_id would reissue a live ID.
	for i := range s.Habits {
		if s.Habits[i].Done == nil {
			s.Habits[i].Done = []string{}
		}
		if s.Habits[i].ID >= s.NextID {
			s.NextID = s.Habits[i].ID + 1
		}
	}
	return &s, nil
}

// Save writes the store to path by way of a temporary file and a rename, so a
// crash or full disk cannot leave a half-written data file behind.
func Save(path string, s *model.Store) error {
	s.Version = model.Version
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode habits: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".habits-*.json")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot flush %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("cannot set permissions on %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("cannot replace %s: %w", path, err)
	}
	return nil
}
