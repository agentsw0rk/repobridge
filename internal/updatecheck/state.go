package updatecheck

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"repobridge/internal/cache"
)

type State struct {
	CheckedAt  time.Time `json:"checkedAt"`
	Latest     string    `json:"latest,omitempty"`
	ReleaseURL string    `json:"releaseUrl,omitempty"`
}

type StateStore struct {
	HomeDir string
}

func DefaultStateStore() (StateStore, error) {
	home, err := cache.Home()
	if err != nil {
		return StateStore{}, err
	}
	return StateStore{HomeDir: home}, nil
}

func (s State) IsFresh(now time.Time, interval time.Duration) bool {
	if s.CheckedAt.IsZero() {
		return false
	}
	if s.CheckedAt.After(now) {
		return false
	}
	return now.Sub(s.CheckedAt) < interval
}

func (s StateStore) Read() (State, error) {
	content, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}

	var state State
	if err := json.Unmarshal(content, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s StateStore) Write(state State) error {
	if err := os.MkdirAll(filepath.Dir(s.path()), 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), append(content, '\n'), 0o644)
}

func (s StateStore) path() string {
	return filepath.Join(s.HomeDir, "update-check.json")
}
