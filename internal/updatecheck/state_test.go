package updatecheck

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStateStoreRoundTrips(t *testing.T) {
	store := StateStore{HomeDir: t.TempDir()}
	now := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	state := State{
		CheckedAt:  now,
		Latest:     "v0.10.5",
		ReleaseURL: "https://github.com/agentsw0rk/repobridge/releases/tag/v0.10.5",
	}
	if err := store.Write(state); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := store.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !got.CheckedAt.Equal(now) || got.Latest != state.Latest || got.ReleaseURL != state.ReleaseURL {
		t.Fatalf("state = %#v, want %#v", got, state)
	}
	if filepath.Base(store.path()) != "update-check.json" {
		t.Fatalf("state path = %s", store.path())
	}
}

func TestStateStoreFreshness(t *testing.T) {
	now := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	fresh := State{CheckedAt: now.Add(-time.Hour)}
	stale := State{CheckedAt: now.Add(-25 * time.Hour)}
	future := State{CheckedAt: now.Add(time.Hour)}
	if !fresh.IsFresh(now, 24*time.Hour) {
		t.Fatal("fresh state reported stale")
	}
	if stale.IsFresh(now, 24*time.Hour) {
		t.Fatal("stale state reported fresh")
	}
	if future.IsFresh(now, 24*time.Hour) {
		t.Fatal("future state reported fresh")
	}
}
