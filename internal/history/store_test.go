package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsOnlyConfirmedIncidentBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 120; i++ {
		store.Observe("service", true, 3, now.Add(time.Duration(i)*time.Second))
	}
	if store.PersistenceWrites() != 0 {
		t.Fatal("raw observations must not immediately write flash")
	}
	store.Open("service", "service", "Service", "timeout", now.Add(time.Hour))
	if store.PersistenceWrites() != 1 {
		t.Fatal("confirmed incident should persist once")
	}
	store.Resolve("service", now.Add(time.Hour+time.Minute))
	if store.PersistenceWrites() != 1 {
		t.Fatal("recovery inside cooldown should be coalesced")
	}
	store.FlushDue(now.Add(time.Hour + writeCooldown + time.Second))
	if store.PersistenceWrites() != 2 {
		t.Fatal("coalesced recovery should persist after cooldown")
	}
}

func TestStoreFallsBackToPrevious(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	store.Open("device", "device", "NAS", "offline", now)
	store.Resolve("device", now.Add(writeCooldown+time.Second))
	store.FlushDue(now.Add(2*writeCooldown + time.Second))
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Incidents()) == 0 {
		t.Fatal("expected previous valid state to be loaded")
	}
}
