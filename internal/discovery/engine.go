package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/catalog"
)

const (
	deviceEstablishCycles  = 2
	deviceOfflineCycles    = 3
	serviceEstablishCycles = 2
	serviceFailureCycles   = 3
	serviceRecoveryCycles  = 2
)

type IncidentRecorder interface {
	Open(subjectID, subjectType, name, reason string, at time.Time)
	Resolve(subjectID string, at time.Time)
	Observe(subjectID string, online bool, latencyMS int64, at time.Time)
	Incidents() []catalog.Incident
	PersistenceWrites() uint64
	FlushDue(time.Time)
	Close() error
}

type Engine struct {
	mu              sync.RWMutex
	version         string
	source          DeviceSource
	scanner         ServiceScanner
	history         IncidentRecorder
	devices         map[string]catalog.Device
	services        map[string]catalog.Service
	lastDiscoveryAt time.Time
	lastFullScan    time.Time
	discoveryError  string
}

func NewEngine(version string, source DeviceSource, scanner ServiceScanner, history IncidentRecorder) *Engine {
	return &Engine{
		version: version, source: source, scanner: scanner, history: history,
		devices: make(map[string]catalog.Device), services: make(map[string]catalog.Service),
	}
}

func (e *Engine) Run(ctx context.Context) {
	if err := e.Step(ctx, time.Now().UTC()); err != nil {
		slog.Warn("automatic discovery failed", "error", err)
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	defer e.history.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := e.Step(ctx, now.UTC()); err != nil {
				slog.Warn("automatic discovery failed", "error", err)
			}
		}
	}
}

func (e *Engine) Step(ctx context.Context, now time.Time) error {
	observations, err := e.source.Discover(ctx)
	if err != nil {
		e.mu.Lock()
		e.discoveryError = "无法读取允许的 LAN 接口状态"
		e.mu.Unlock()
		return err
	}
	e.mu.Lock()
	e.discoveryError = ""
	seenDevices := make(map[string]bool, len(observations))
	newOrCandidate := make(map[string]bool)
	for _, observation := range observations {
		if net.ParseIP(observation.IPv4).To4() == nil {
			continue
		}
		id, err := catalog.DeviceID(observation.Domain, observation.MAC)
		if err != nil {
			continue
		}
		seenDevices[id] = true
		device, exists := e.devices[id]
		if !exists {
			name := strings.TrimSpace(observation.Name)
			if name == "" {
				name = observation.IPv4
			}
			device = catalog.Device{
				ID: id, Domain: observation.Domain, Interface: observation.Interface,
				MAC: strings.ToLower(observation.MAC), IPv4: observation.IPv4, Name: name,
				State: catalog.DeviceCandidate, FirstSeen: now, SeenCycles: 1, Router: observation.Router,
			}
			newOrCandidate[id] = true
		} else {
			wasOffline := device.State == catalog.DeviceOffline
			device.SeenCycles++
			device.MissedCycles = 0
			device.IPv4 = observation.IPv4
			device.Interface = observation.Interface
			if observation.Name != "" {
				device.Name = observation.Name
			}
			if device.SeenCycles >= deviceEstablishCycles {
				device.State = catalog.DeviceOnline
			}
			if wasOffline && device.State == catalog.DeviceOnline {
				e.history.Resolve(device.ID, now)
			}
			if device.State == catalog.DeviceCandidate {
				newOrCandidate[id] = true
			}
		}
		device.LastSeen = now
		e.devices[id] = device
	}
	for id, device := range e.devices {
		if seenDevices[id] {
			continue
		}
		device.MissedCycles++
		device.SeenCycles = 0
		if device.MissedCycles >= deviceOfflineCycles && device.State != catalog.DeviceOffline {
			device.State = catalog.DeviceOffline
			e.history.Open(device.ID, "device", device.Name, "device not observed on allowed LAN interface", now)
			for serviceID, service := range e.services {
				if service.DeviceID == device.ID {
					e.history.Resolve(serviceID, now)
					service.State = catalog.ServiceOffline
					e.services[serviceID] = service
				}
			}
		}
		e.devices[id] = device
	}
	fullScan := e.lastFullScan.IsZero() || now.Sub(e.lastFullScan) >= 10*time.Minute
	devices := make([]catalog.Device, 0, len(e.devices))
	for id, device := range e.devices {
		if device.State != catalog.DeviceOffline && (fullScan || newOrCandidate[id] || e.hasCandidateService(id)) {
			devices = append(devices, device)
		}
	}
	e.lastDiscoveryAt = now
	if fullScan {
		e.lastFullScan = now
	}
	e.mu.Unlock()

	for _, device := range devices {
		services, scanErr := e.scanner.Scan(ctx, device)
		if scanErr != nil {
			continue
		}
		e.mergeServices(device, services, now)
	}
	e.checkEstablished(ctx, now)
	e.history.FlushDue(now)
	return nil
}

func (e *Engine) hasCandidateService(deviceID string) bool {
	for _, service := range e.services {
		if service.DeviceID == deviceID && service.State == catalog.ServiceCandidate {
			return true
		}
	}
	return false
}

func (e *Engine) mergeServices(device catalog.Device, found []catalog.Service, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	foundIDs := make(map[string]bool, len(found))
	for _, candidate := range found {
		foundIDs[candidate.ID] = true
		current, exists := e.services[candidate.ID]
		if !exists {
			candidate.FirstSeen = now
			candidate.LastSeen = now
			candidate.State = catalog.ServiceCandidate
			candidate.DiscoveryHits = 1
			e.services[candidate.ID] = candidate
			continue
		}
		current.LastSeen = now
		current.Name = candidate.Name
		current.URL = candidate.URL
		current.Type = candidate.Type
		current.LatencyMS = candidate.LatencyMS
		current.DiscoveryHits++
		if current.DiscoveryHits >= serviceEstablishCycles && current.State == catalog.ServiceCandidate {
			current.State = catalog.ServiceEstablished
			current.SuccessCount = 1
		}
		e.services[current.ID] = current
	}
	for id, service := range e.services {
		if service.DeviceID == device.ID && service.State == catalog.ServiceCandidate && !foundIDs[id] {
			delete(e.services, id)
		}
	}
}

func (e *Engine) checkEstablished(ctx context.Context, now time.Time) {
	e.mu.RLock()
	services := make([]catalog.Service, 0, len(e.services))
	for _, service := range e.services {
		device := e.devices[service.DeviceID]
		if service.State != catalog.ServiceCandidate && device.State == catalog.DeviceOnline {
			services = append(services, service)
		}
	}
	e.mu.RUnlock()
	for _, service := range services {
		result := e.scanner.Health(ctx, service)
		e.mu.Lock()
		current, exists := e.services[service.ID]
		if !exists {
			e.mu.Unlock()
			continue
		}
		if result.Online {
			e.history.Observe(current.ID, true, result.LatencyMS, now)
			current.FailureCount = 0
			current.SuccessCount++
			current.LatencyMS = result.LatencyMS
			if current.State == catalog.ServiceOffline && current.SuccessCount >= serviceRecoveryCycles {
				current.State = catalog.ServiceOnline
				e.history.Resolve(current.ID, now)
			} else if current.State == catalog.ServiceEstablished {
				current.State = catalog.ServiceOnline
			}
		} else {
			e.history.Observe(current.ID, false, 0, now)
			current.SuccessCount = 0
			current.FailureCount++
			if current.FailureCount >= serviceFailureCycles && current.State != catalog.ServiceOffline {
				current.State = catalog.ServiceOffline
				e.history.Open(current.ID, "service", current.Name, result.Reason, now)
			}
		}
		e.services[current.ID] = current
		e.mu.Unlock()
	}
}

func (e *Engine) Snapshot() catalog.Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	snapshot := catalog.Snapshot{
		Version: e.version, GeneratedAt: time.Now().UTC(), IPv4Only: true,
		LastDiscoveryAt: e.lastDiscoveryAt, DiscoveryError: e.discoveryError,
		PersistenceWritesSinceBoot: e.history.PersistenceWrites(),
		Devices:                    make([]catalog.Device, 0, len(e.devices)), Services: make([]catalog.Service, 0, len(e.services)),
		Incidents: e.history.Incidents(),
	}
	for _, device := range e.devices {
		if device.State != catalog.DeviceOffline || time.Since(device.LastSeen) < 24*time.Hour {
			snapshot.Devices = append(snapshot.Devices, device)
		}
	}
	for _, service := range e.services {
		snapshot.Services = append(snapshot.Services, service)
	}
	catalog.SortSnapshot(&snapshot)
	return snapshot
}

func (e *Engine) Device(id string) (catalog.Device, []catalog.Service, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	device, ok := e.devices[id]
	if !ok {
		return catalog.Device{}, nil, false
	}
	services := make([]catalog.Service, 0)
	for _, service := range e.services {
		if service.DeviceID == id {
			services = append(services, service)
		}
	}
	sort.Slice(services, func(i, j int) bool { return services[i].ID < services[j].ID })
	return device, services, true
}

func ValidateObservation(observation Observation) error {
	if strings.TrimSpace(observation.Domain) == "" {
		return fmt.Errorf("network domain is required")
	}
	if net.ParseIP(observation.IPv4).To4() == nil {
		return fmt.Errorf("V1 requires an IPv4 address")
	}
	if _, err := net.ParseMAC(observation.MAC); err != nil {
		return fmt.Errorf("invalid MAC: %w", err)
	}
	return nil
}
