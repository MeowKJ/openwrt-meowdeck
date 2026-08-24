package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

type DeviceState string

const (
	DeviceCandidate DeviceState = "candidate"
	DeviceOnline    DeviceState = "online"
	DeviceOffline   DeviceState = "offline"
)

type ServiceState string

const (
	ServiceCandidate   ServiceState = "candidate"
	ServiceEstablished ServiceState = "established"
	ServiceOnline      ServiceState = "online"
	ServiceOffline     ServiceState = "offline"
)

type Device struct {
	ID           string      `json:"id"`
	Domain       string      `json:"domain"`
	Interface    string      `json:"interface"`
	MAC          string      `json:"mac"`
	IPv4         string      `json:"ipv4"`
	Name         string      `json:"name"`
	Router       bool        `json:"router,omitempty"`
	State        DeviceState `json:"state"`
	FirstSeen    time.Time   `json:"firstSeen"`
	LastSeen     time.Time   `json:"lastSeen"`
	SeenCycles   int         `json:"-"`
	MissedCycles int         `json:"-"`
}

type Service struct {
	ID            string       `json:"id"`
	DeviceID      string       `json:"deviceId"`
	Scheme        string       `json:"scheme"`
	Port          int          `json:"port"`
	Path          string       `json:"path"`
	Type          string       `json:"type"`
	Name          string       `json:"name"`
	URL           string       `json:"url"`
	State         ServiceState `json:"state"`
	FirstSeen     time.Time    `json:"firstSeen"`
	LastSeen      time.Time    `json:"lastSeen"`
	DiscoveryHits int          `json:"-"`
	FailureCount  int          `json:"-"`
	SuccessCount  int          `json:"-"`
	LatencyMS     int64        `json:"latencyMs,omitempty"`
}

type Incident struct {
	ID          string     `json:"id"`
	SubjectID   string     `json:"subjectId"`
	SubjectType string     `json:"subjectType"`
	Name        string     `json:"name"`
	Reason      string     `json:"reason"`
	StartedAt   time.Time  `json:"startedAt"`
	ResolvedAt  *time.Time `json:"resolvedAt,omitempty"`
}

type Snapshot struct {
	Version                    string     `json:"version"`
	GeneratedAt                time.Time  `json:"generatedAt"`
	IPv4Only                   bool       `json:"ipv4Only"`
	LastDiscoveryAt            time.Time  `json:"lastDiscoveryAt"`
	DiscoveryError             string     `json:"discoveryError,omitempty"`
	PersistenceWritesSinceBoot uint64     `json:"persistenceWritesSinceBoot"`
	Devices                    []Device   `json:"devices"`
	Services                   []Service  `json:"services"`
	Incidents                  []Incident `json:"incidents"`
}

func DeviceID(domain, mac string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	hardware, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil {
		return "", fmt.Errorf("parse MAC: %w", err)
	}
	canonical := strings.ToLower(hardware.String())
	sum := sha256.Sum256([]byte(domain + "\x00" + canonical))
	return "dev-" + hex.EncodeToString(sum[:8]), nil
}

func ServiceID(deviceID, scheme string, port int, path string) string {
	if path == "" {
		path = "/"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s", deviceID, scheme, port, path)))
	return "svc-" + hex.EncodeToString(sum[:8])
}

func SortSnapshot(snapshot *Snapshot) {
	sort.Slice(snapshot.Devices, func(i, j int) bool {
		if snapshot.Devices[i].State != snapshot.Devices[j].State {
			return snapshot.Devices[i].State < snapshot.Devices[j].State
		}
		return strings.ToLower(snapshot.Devices[i].Name) < strings.ToLower(snapshot.Devices[j].Name)
	})
	sort.Slice(snapshot.Services, func(i, j int) bool {
		if snapshot.Services[i].DeviceID != snapshot.Services[j].DeviceID {
			return snapshot.Services[i].DeviceID < snapshot.Services[j].DeviceID
		}
		if snapshot.Services[i].Port != snapshot.Services[j].Port {
			return snapshot.Services[i].Port < snapshot.Services[j].Port
		}
		return snapshot.Services[i].Path < snapshot.Services[j].Path
	})
}
