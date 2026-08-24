package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/catalog"
)

const (
	stateSchema     = 1
	maxStateBytes   = 256 << 10
	maxIncidents    = 100
	maxSummaryDays  = 90
	writeCooldown   = 5 * time.Minute
	summaryBatchAge = 7 * 24 * time.Hour
)

type DailySummary struct {
	Date       string `json:"date"`
	SubjectID  string `json:"subjectId"`
	Checks     uint64 `json:"checks"`
	Successes  uint64 `json:"successes"`
	Failures   uint64 `json:"failures"`
	LatencySum uint64 `json:"latencySumMs,omitempty"`
}

type persistedState struct {
	Schema    int                     `json:"schema"`
	SavedAt   time.Time               `json:"savedAt"`
	Incidents []catalog.Incident      `json:"incidents"`
	Daily     map[string]DailySummary `json:"daily,omitempty"`
}

type Store struct {
	mu        sync.Mutex
	saveMu    sync.Mutex
	path      string
	incidents []catalog.Incident
	daily     map[string]DailySummary
	dirty     bool
	urgent    bool
	lastWrite time.Time
	writes    uint64
	lastError error
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, daily: make(map[string]DailySummary)}
	if path == "" {
		return store, nil
	}
	if err := store.load(path); err != nil {
		if previousErr := store.load(path + ".previous"); previousErr != nil {
			if errors.Is(err, os.ErrNotExist) && errors.Is(previousErr, os.ErrNotExist) {
				return store, nil
			}
			return nil, fmt.Errorf("load history: %w", err)
		}
	}
	return store, nil
}

func (s *Store) load(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw) > maxStateBytes {
		return fmt.Errorf("state exceeds %d bytes", maxStateBytes)
	}
	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if state.Schema != stateSchema {
		return fmt.Errorf("unsupported state schema %d", state.Schema)
	}
	s.incidents = append([]catalog.Incident(nil), state.Incidents...)
	if state.Daily != nil {
		s.daily = state.Daily
	}
	s.lastWrite = state.SavedAt
	return nil
}

func (s *Store) Open(subjectID, subjectType, name, reason string, at time.Time) {
	s.mu.Lock()
	for _, incident := range s.incidents {
		if incident.SubjectID == subjectID && incident.ResolvedAt == nil {
			s.mu.Unlock()
			return
		}
	}
	incident := catalog.Incident{
		ID: fmt.Sprintf("%s-%d", subjectID, at.UnixNano()), SubjectID: subjectID,
		SubjectType: subjectType, Name: name, Reason: compact(reason, 160), StartedAt: at,
	}
	s.incidents = append([]catalog.Incident{incident}, s.incidents...)
	if len(s.incidents) > maxIncidents {
		s.incidents = s.incidents[:maxIncidents]
	}
	s.dirty = true
	s.urgent = true
	s.mu.Unlock()
	s.flushIfAllowed(at)
}

func (s *Store) Resolve(subjectID string, at time.Time) {
	s.mu.Lock()
	changed := false
	for index := range s.incidents {
		if s.incidents[index].SubjectID == subjectID && s.incidents[index].ResolvedAt == nil {
			resolved := at
			s.incidents[index].ResolvedAt = &resolved
			changed = true
			break
		}
	}
	if changed {
		s.dirty = true
		s.urgent = true
	}
	s.mu.Unlock()
	if changed {
		s.flushIfAllowed(at)
	}
}

func (s *Store) Observe(subjectID string, online bool, latencyMS int64, at time.Time) {
	date := at.Local().Format("2006-01-02")
	key := date + "\x00" + subjectID
	s.mu.Lock()
	summary := s.daily[key]
	summary.Date = date
	summary.SubjectID = subjectID
	summary.Checks++
	if online {
		summary.Successes++
		if latencyMS > 0 {
			summary.LatencySum += uint64(latencyMS)
		}
	} else {
		summary.Failures++
	}
	s.daily[key] = summary
	s.trimDailyLocked()
	s.dirty = true
	s.mu.Unlock()
}

func (s *Store) trimDailyLocked() {
	dates := make([]string, 0)
	seen := map[string]bool{}
	for _, summary := range s.daily {
		if !seen[summary.Date] {
			seen[summary.Date] = true
			dates = append(dates, summary.Date)
		}
	}
	if len(dates) <= maxSummaryDays {
		return
	}
	sort.Strings(dates)
	cutoff := dates[len(dates)-maxSummaryDays]
	for key, summary := range s.daily {
		if summary.Date < cutoff {
			delete(s.daily, key)
		}
	}
}

func (s *Store) Incidents() []catalog.Incident {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]catalog.Incident{}, s.incidents...)
}

func (s *Store) PersistenceWrites() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

func (s *Store) FlushDue(now time.Time) {
	s.mu.Lock()
	due := s.dirty && ((s.urgent && now.Sub(s.lastWrite) >= writeCooldown) || now.Sub(s.lastWrite) >= summaryBatchAge)
	s.mu.Unlock()
	if due {
		_ = s.save(now)
	}
}

func (s *Store) flushIfAllowed(now time.Time) {
	s.mu.Lock()
	allowed := s.path != "" && (s.lastWrite.IsZero() || now.Sub(s.lastWrite) >= writeCooldown)
	s.mu.Unlock()
	if allowed {
		_ = s.save(now)
	}
}

func (s *Store) Close() error {
	s.mu.Lock()
	dirty := s.dirty
	s.mu.Unlock()
	if !dirty {
		return nil
	}
	return s.save(time.Now().UTC())
}

func (s *Store) save(now time.Time) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	if s.path == "" || !s.dirty {
		s.mu.Unlock()
		return nil
	}
	state := persistedState{
		Schema: stateSchema, SavedAt: now, Incidents: append([]catalog.Incident(nil), s.incidents...),
		Daily: make(map[string]DailySummary, len(s.daily)),
	}
	for key, summary := range s.daily {
		state.Daily[key] = summary
	}
	s.mu.Unlock()

	raw, err := json.Marshal(state)
	if err != nil {
		return s.recordError(err)
	}
	for len(raw) > maxStateBytes && len(state.Incidents) > 1 {
		state.Incidents = state.Incidents[:len(state.Incidents)-1]
		raw, err = json.Marshal(state)
		if err != nil {
			return s.recordError(err)
		}
	}
	if len(raw) > maxStateBytes {
		return s.recordError(fmt.Errorf("history state exceeds %d bytes", maxStateBytes))
	}
	if err := atomicWrite(s.path, raw, 0o600); err != nil {
		return s.recordError(err)
	}
	s.mu.Lock()
	s.lastWrite = now
	s.dirty = false
	s.urgent = false
	s.writes++
	s.lastError = nil
	s.mu.Unlock()
	return nil
}

func (s *Store) recordError(err error) error {
	s.mu.Lock()
	s.lastError = err
	s.mu.Unlock()
	return err
}

func atomicWrite(path string, raw []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".history-*.new")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := copyFile(path, path+".previous", mode); err != nil {
			return err
		}
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryHandle.Close()
	return directoryHandle.Sync()
}

func copyFile(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(target), ".previous-*.new")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, input); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, target)
}

func compact(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
