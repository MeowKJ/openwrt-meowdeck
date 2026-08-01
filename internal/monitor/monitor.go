package monitor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/config"
)

type State string

const (
	StateOnline   State = "online"
	StateDegraded State = "degraded"
	StateOffline  State = "offline"
	StateDisabled State = "disabled"
	StateChecking State = "checking"
)

type Point struct {
	At        time.Time `json:"at"`
	State     State     `json:"state"`
	LatencyMS int64     `json:"latencyMs,omitempty"`
}

type ServiceStatus struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Subdomain   string  `json:"subdomain,omitempty"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Icon        string  `json:"icon"`
	URL         string  `json:"url,omitempty"`
	State       State   `json:"state"`
	LatencyMS   int64   `json:"latencyMs,omitempty"`
	LastChecked string  `json:"lastChecked,omitempty"`
	Message     string  `json:"message,omitempty"`
	Editable    bool    `json:"editable"`
	History     []Point `json:"history"`
}

type Snapshot struct {
	Version         string          `json:"version"`
	GeneratedAt     time.Time       `json:"generatedAt"`
	Hostname        string          `json:"hostname"`
	IntervalSeconds int             `json:"intervalSeconds"`
	Services        []ServiceStatus `json:"services"`
}

type result struct {
	state     State
	latencyMS int64
	message   string
}

type Manager struct {
	mu       sync.RWMutex
	cfg      config.Config
	version  string
	statuses map[string]ServiceStatus
	http     *http.Client
}

func New(cfg config.Config, version string) *Manager {
	m := &Manager{
		cfg:      cfg,
		version:  version,
		statuses: make(map[string]ServiceStatus, len(cfg.Services)),
		http: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	for _, service := range cfg.Services {
		m.statuses[service.ID] = initialStatus(cfg.Hostname, service)
	}
	return m
}

func initialStatus(hostname string, service config.Service) ServiceStatus {
	state := StateChecking
	message := "等待首次检查"
	if service.Disabled {
		state = StateDisabled
		message = "待接入"
	}
	serviceURL := ""
	if service.URL != "" {
		serviceURL = "/" + service.Slug
		if service.Subdomain != "" {
			serviceURL = "http://" + service.Subdomain + "." + hostname
		}
	}
	return ServiceStatus{
		ID: service.ID, Slug: service.Slug, Subdomain: service.Subdomain,
		Name: service.Name, Description: service.Description,
		Category: service.Category, Icon: service.Icon, URL: serviceURL,
		State: state, Message: message, Editable: !service.Protected, History: []Point{},
	}
}

func (m *Manager) Run(ctx context.Context) {
	m.CheckNow(ctx)
	ticker := time.NewTicker(time.Duration(m.cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CheckNow(ctx)
		}
	}
}

func (m *Manager) CheckNow(ctx context.Context) {
	m.mu.RLock()
	services := append([]config.Service(nil), m.cfg.Services...)
	timeoutSeconds := m.cfg.TimeoutSeconds
	m.mu.RUnlock()
	var wg sync.WaitGroup
	for _, service := range services {
		service := service
		if service.Disabled {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
			defer cancel()
			started := time.Now()
			res := m.check(probeCtx, service)
			if res.latencyMS == 0 {
				res.latencyMS = time.Since(started).Milliseconds()
			}
			m.record(service.ID, res)
		}()
	}
	wg.Wait()
}

func (m *Manager) ReplaceServices(services []config.Service) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.Services = append([]config.Service(nil), services...)
	next := make(map[string]ServiceStatus, len(services))
	for _, service := range services {
		if current, exists := m.statuses[service.ID]; exists {
			base := initialStatus(m.cfg.Hostname, service)
			base.State = current.State
			base.LatencyMS = current.LatencyMS
			base.LastChecked = current.LastChecked
			base.Message = current.Message
			base.History = append([]Point{}, current.History...)
			next[service.ID] = base
			continue
		}
		next[service.ID] = initialStatus(m.cfg.Hostname, service)
	}
	m.statuses = next
}

func (m *Manager) Services() []config.Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]config.Service(nil), m.cfg.Services...)
}

func (m *Manager) check(ctx context.Context, service config.Service) result {
	switch service.Probe.Type {
	case "http":
		return m.checkHTTP(ctx, service)
	case "tcp":
		return checkTCP(ctx, service.Probe.Target)
	case "ping":
		m.mu.RLock()
		timeoutSeconds := m.cfg.TimeoutSeconds
		m.mu.RUnlock()
		return checkCommand(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSeconds), service.Probe.Target)
	case "process":
		return checkCommand(ctx, "pidof", service.Probe.Target)
	default:
		return result{state: StateOffline, message: "不支持的检查类型"}
	}
}

func (m *Manager) checkHTTP(ctx context.Context, service config.Service) result {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.Probe.Target, nil)
	if err != nil {
		return result{state: StateOffline, message: err.Error()}
	}
	started := time.Now()
	response, err := m.http.Do(request)
	if err != nil {
		return result{state: StateOffline, message: compactError(err)}
	}
	defer response.Body.Close()

	expected := service.Probe.ExpectedStatus
	if len(expected) == 0 {
		if response.StatusCode >= 200 && response.StatusCode < 500 {
			return result{state: StateOnline, latencyMS: time.Since(started).Milliseconds(), message: response.Status}
		}
	} else {
		for _, status := range expected {
			if response.StatusCode == status {
				return result{state: StateOnline, latencyMS: time.Since(started).Milliseconds(), message: response.Status}
			}
		}
	}
	return result{state: StateDegraded, latencyMS: time.Since(started).Milliseconds(), message: response.Status}
}

func checkTCP(ctx context.Context, target string) result {
	started := time.Now()
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return result{state: StateOffline, message: compactError(err)}
	}
	_ = connection.Close()
	return result{state: StateOnline, latencyMS: time.Since(started).Milliseconds(), message: "端口可达"}
}

func checkCommand(ctx context.Context, name string, args ...string) result {
	started := time.Now()
	if err := exec.CommandContext(ctx, name, args...).Run(); err != nil {
		return result{state: StateOffline, message: compactError(err)}
	}
	return result{state: StateOnline, latencyMS: time.Since(started).Milliseconds(), message: "运行中"}
}

func compactError(err error) string {
	if err == nil {
		return ""
	}
	if err == context.DeadlineExceeded {
		return "检查超时"
	}
	return fmt.Sprintf("%v", err)
}

func (m *Manager) record(id string, res result) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.statuses[id]
	if !ok {
		return
	}
	now := time.Now().UTC()
	status.State = res.state
	status.LatencyMS = res.latencyMS
	status.LastChecked = now.Format(time.RFC3339)
	status.Message = res.message
	status.History = append(status.History, Point{At: now, State: res.state, LatencyMS: res.latencyMS})
	if len(status.History) > m.cfg.HistorySize {
		status.History = append([]Point(nil), status.History[len(status.History)-m.cfg.HistorySize:]...)
	}
	m.statuses[id] = status
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	services := make([]ServiceStatus, 0, len(m.cfg.Services))
	for _, service := range m.cfg.Services {
		status := m.statuses[service.ID]
		status.History = append([]Point{}, status.History...)
		services = append(services, status)
	}
	return Snapshot{
		Version: m.version, GeneratedAt: time.Now().UTC(), Hostname: m.cfg.Hostname,
		IntervalSeconds: m.cfg.IntervalSeconds, Services: services,
	}
}
