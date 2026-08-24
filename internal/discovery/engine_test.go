package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/catalog"
)

type sequenceSource struct {
	steps [][]Observation
	index int
}

func (s *sequenceSource) Discover(context.Context) ([]Observation, error) {
	if s.index >= len(s.steps) {
		return s.steps[len(s.steps)-1], nil
	}
	result := s.steps[s.index]
	s.index++
	return result, nil
}

type fakeScanner struct {
	service catalog.Service
	health  HealthResult
	found   bool
}

func (s *fakeScanner) Scan(_ context.Context, device catalog.Device) ([]catalog.Service, error) {
	if !s.found {
		return nil, nil
	}
	service := s.service
	service.DeviceID = device.ID
	service.ID = catalog.ServiceID(device.ID, "http", 80, "/")
	service.URL = "http://" + device.IPv4 + "/"
	return []catalog.Service{service}, nil
}

func (s *fakeScanner) Health(context.Context, catalog.Service) HealthResult { return s.health }

type memoryRecorder struct {
	incidents []catalog.Incident
	observed  int
}

func (r *memoryRecorder) Open(id, kind, name, reason string, at time.Time) {
	for _, incident := range r.incidents {
		if incident.SubjectID == id && incident.ResolvedAt == nil {
			return
		}
	}
	r.incidents = append([]catalog.Incident{{
		ID: id, SubjectID: id, SubjectType: kind, Name: name, Reason: reason, StartedAt: at,
	}}, r.incidents...)
}
func (r *memoryRecorder) Resolve(id string, at time.Time) {
	for index := range r.incidents {
		if r.incidents[index].SubjectID == id && r.incidents[index].ResolvedAt == nil {
			resolved := at
			r.incidents[index].ResolvedAt = &resolved
		}
	}
}
func (r *memoryRecorder) Observe(string, bool, int64, time.Time) { r.observed++ }
func (r *memoryRecorder) Incidents() []catalog.Incident {
	return append([]catalog.Incident(nil), r.incidents...)
}
func (r *memoryRecorder) PersistenceWrites() uint64 { return 0 }
func (r *memoryRecorder) FlushDue(time.Time)        {}
func (r *memoryRecorder) Close() error              { return nil }

func testObservation() Observation {
	return Observation{Domain: "lan", Interface: "br-lan", MAC: "aa:bb:cc:dd:ee:ff", IPv4: "192.168.8.20", Name: "nas"}
}

func TestCandidateServiceDoesNotCreateIncident(t *testing.T) {
	source := &sequenceSource{steps: [][]Observation{{testObservation()}, {testObservation()}}}
	scanner := &fakeScanner{found: true, service: catalog.Service{Name: "Temporary", Scheme: "http", Port: 80, Path: "/"}}
	recorder := &memoryRecorder{}
	engine := NewEngine("test", source, scanner, recorder)
	now := time.Now().UTC()
	if err := engine.Step(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	scanner.found = false
	if err := engine.Step(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(recorder.incidents) != 0 {
		t.Fatal("candidate service disappearance must not create an incident")
	}
	if len(engine.Snapshot().Services) != 0 {
		t.Fatal("disappeared candidate service should be removed")
	}
}

func TestEstablishedServiceFailureAndRecoveryThresholds(t *testing.T) {
	observation := testObservation()
	source := &sequenceSource{steps: [][]Observation{{observation}, {observation}, {observation}, {observation}, {observation}, {observation}, {observation}}}
	scanner := &fakeScanner{
		found: true, health: HealthResult{Online: true, LatencyMS: 2},
		service: catalog.Service{Name: "Home Assistant", Scheme: "http", Port: 8123, Path: "/"},
	}
	recorder := &memoryRecorder{}
	engine := NewEngine("test", source, scanner, recorder)
	now := time.Now().UTC()
	_ = engine.Step(context.Background(), now)
	_ = engine.Step(context.Background(), now.Add(time.Minute))
	scanner.health = HealthResult{Reason: "timeout"}
	_ = engine.Step(context.Background(), now.Add(2*time.Minute))
	_ = engine.Step(context.Background(), now.Add(3*time.Minute))
	if len(recorder.incidents) != 0 {
		t.Fatal("two failures must not create an incident")
	}
	_ = engine.Step(context.Background(), now.Add(4*time.Minute))
	if len(recorder.incidents) != 1 || recorder.incidents[0].ResolvedAt != nil {
		t.Fatal("third failure should create one active incident")
	}
	scanner.health = HealthResult{Online: true, LatencyMS: 3}
	_ = engine.Step(context.Background(), now.Add(5*time.Minute))
	if recorder.incidents[0].ResolvedAt != nil {
		t.Fatal("one success must not recover an incident")
	}
	_ = engine.Step(context.Background(), now.Add(6*time.Minute))
	if recorder.incidents[0].ResolvedAt == nil {
		t.Fatal("second success should recover an incident")
	}
}

func TestDeviceOfflineSuppressesChildIncident(t *testing.T) {
	observation := testObservation()
	source := &sequenceSource{steps: [][]Observation{{observation}, {observation}, {}, {}, {}}}
	scanner := &fakeScanner{
		found: true, health: HealthResult{Reason: "timeout"},
		service: catalog.Service{Name: "Plex", Scheme: "http", Port: 32400, Path: "/"},
	}
	recorder := &memoryRecorder{}
	engine := NewEngine("test", source, scanner, recorder)
	now := time.Now().UTC()
	for index := 0; index < 5; index++ {
		_ = engine.Step(context.Background(), now.Add(time.Duration(index)*time.Minute))
	}
	activeDevices, activeServices := 0, 0
	for _, incident := range recorder.incidents {
		if incident.ResolvedAt == nil && incident.SubjectType == "device" {
			activeDevices++
		}
		if incident.ResolvedAt == nil && incident.SubjectType == "service" {
			activeServices++
		}
	}
	if activeDevices != 1 || activeServices != 0 {
		t.Fatalf("expected one parent incident and no active child incidents: %#v", recorder.incidents)
	}
}
