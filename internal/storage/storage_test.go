package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/model"
)

func TestLoadMissingFileIsAnEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "habits.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load of a missing file failed: %v", err)
	}
	if len(s.Habits) != 0 || s.NextID != 1 {
		t.Errorf("empty store = %+v", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "habits.json")
	today := hdate.New(2026, time.August, 6)

	s := model.NewStore()
	h, _ := s.Add("meditate", []string{"health"}, today)
	h.MarkDone(today)
	h.MarkDone(today.Add(-1))

	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Habits) != 1 {
		t.Fatalf("loaded %d habits", len(got.Habits))
	}
	loaded := got.Habits[0]
	if loaded.Name != "meditate" || loaded.ID != 1 || len(loaded.Done) != 2 {
		t.Errorf("round trip lost data: %+v", loaded)
	}
	if got.NextID != 2 || got.Version != model.Version {
		t.Errorf("store header = %+v", got)
	}
}

func TestSaveOverwritesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "habits.json")
	today := hdate.New(2026, time.August, 6)

	first := model.NewStore()
	first.Add("one", nil, today)
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}

	second := model.NewStore()
	second.Add("two", nil, today)
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Habits) != 1 || got.Habits[0].Name != "two" {
		t.Errorf("overwrite left %+v", got.Habits)
	}

	// The temporary file used for the atomic write must not survive.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want only habits.json", names)
	}
}

func TestLoadCorruptFileErrorsWithoutDestroyingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "habits.json")
	const junk = "{not json"
	if err := os.WriteFile(path, []byte(junk), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted corrupt data")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != junk {
		t.Error("Load modified the corrupt file instead of leaving it alone")
	}
}

func TestLoadEmptyFileIsAnEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "habits.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load of an empty file failed: %v", err)
	}
	if len(s.Habits) != 0 {
		t.Errorf("store = %+v", s)
	}
}

func TestLoadRepairsNextIDFromHandEditedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "habits.json")
	const handEdited = `{"version":1,"next_id":1,"habits":[{"id":7,"name":"run","created":"2026-08-01","done":null}]}`
	if err := os.WriteFile(path, []byte(handEdited), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.NextID != 8 {
		t.Errorf("NextID = %d, want 8 so the live ID is not reissued", s.NextID)
	}
	if s.Habits[0].Done == nil {
		t.Error("null done list was not normalized to an empty slice")
	}
}

func TestLoadRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "habits.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"next_id":1,"habits":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load accepted a file from a newer version of hw")
	}
}

func TestDefaultPathPrefersEnvThenXDG(t *testing.T) {
	t.Setenv(EnvPath, "/tmp/explicit.json")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/explicit.json" {
		t.Errorf("DefaultPath = %s, want the env override", got)
	}

	t.Setenv(EnvPath, "")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg")
	got, err = DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/tmp/xdg", "habit-warrior", "habits.json"); got != want {
		t.Errorf("DefaultPath = %s, want %s", got, want)
	}
}
