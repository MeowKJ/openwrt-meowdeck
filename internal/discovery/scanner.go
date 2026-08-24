package discovery

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/catalog"
)

var titlePattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

type HealthResult struct {
	Online    bool
	LatencyMS int64
	Reason    string
}

type ServiceScanner interface {
	Scan(context.Context, catalog.Device) ([]catalog.Service, error)
	Health(context.Context, catalog.Service) HealthResult
}

type HTTPScanner struct {
	Ports       []int
	Concurrency int
	DialTimeout time.Duration
	HTTPTimeout time.Duration
}

func NewHTTPScanner() *HTTPScanner {
	return &HTTPScanner{
		Ports:       []int{80, 443, 3000, 5000, 8000, 8080, 8123, 8443, 9000, 9090, 32400},
		Concurrency: 8, DialTimeout: 300 * time.Millisecond, HTTPTimeout: time.Second,
	}
}

func (s *HTTPScanner) Scan(ctx context.Context, device catalog.Device) ([]catalog.Service, error) {
	if net.ParseIP(device.IPv4).To4() == nil {
		return nil, nil
	}
	concurrency := s.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	semaphore := make(chan struct{}, concurrency)
	results := make(chan []catalog.Service, len(s.Ports))
	var group sync.WaitGroup
	for _, port := range s.Ports {
		port := port
		group.Add(1)
		go func() {
			defer group.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			services := s.scanPort(ctx, device, port)
			if len(services) > 0 {
				results <- services
			}
		}()
	}
	group.Wait()
	close(results)
	services := make([]catalog.Service, 0)
	for batch := range results {
		services = append(services, batch...)
	}
	return services, nil
}

func (s *HTTPScanner) scanPort(ctx context.Context, device catalog.Device, port int) []catalog.Service {
	schemes := []string{"http", "https"}
	if port == 443 || port == 8443 {
		schemes = []string{"https", "http"}
	}
	for _, scheme := range schemes {
		root, ok := s.inspect(ctx, device, scheme, port, "/")
		if !ok {
			continue
		}
		services := []catalog.Service{root}
		if device.Router && port == 80 {
			for _, rule := range []struct {
				path     string
				name     string
				typeName string
			}{
				{path: "/cgi-bin/luci/", name: "LuCI", typeName: "luci"},
				{path: "/cgi-bin/luci/admin/services/openclash", name: "OpenClash", typeName: "openclash"},
			} {
				service, found := s.inspect(ctx, device, scheme, port, rule.path)
				if found && service.Name != root.Name {
					service.Name = rule.name
					service.Type = rule.typeName
					services = append(services, service)
				}
			}
		}
		return services
	}
	return nil
}

func (s *HTTPScanner) inspect(ctx context.Context, device catalog.Device, scheme string, port int, requestPath string) (catalog.Service, bool) {
	address := net.JoinHostPort(device.IPv4, fmt.Sprintf("%d", port))
	probeURL := (&url.URL{Scheme: scheme, Host: address, Path: requestPath}).String()
	started := time.Now()
	client := s.client()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return catalog.Service{}, false
	}
	request.Header.Set("User-Agent", "MeowDeck/1 service-discovery")
	response, err := client.Do(request)
	if err != nil {
		return catalog.Service{}, false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 500 {
		return catalog.Service{}, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return catalog.Service{}, false
	}
	name, typeName := classifyService(port, body, response.Header)
	if name == "" {
		name = device.Name
		if name == "" {
			name = device.IPv4
		}
		if port != 80 && port != 443 {
			name = fmt.Sprintf("%s:%d", name, port)
		}
	}
	return catalog.Service{
		ID: catalog.ServiceID(device.ID, scheme, port, requestPath), DeviceID: device.ID,
		Scheme: scheme, Port: port, Path: requestPath, Type: typeName, Name: name, URL: probeURL,
		State: catalog.ServiceCandidate, FirstSeen: time.Now().UTC(), LastSeen: time.Now().UTC(),
		DiscoveryHits: 1, LatencyMS: time.Since(started).Milliseconds(),
	}, true
}

func (s *HTTPScanner) client() *http.Client {
	transport := &http.Transport{
		DialContext:     (&net.Dialer{Timeout: s.DialTimeout}).DialContext,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // LAN services commonly use self-signed certificates.
	}
	return &http.Client{
		Timeout:       s.HTTPTimeout,
		Transport:     transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (s *HTTPScanner) Health(ctx context.Context, service catalog.Service) HealthResult {
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.URL, nil)
	if err != nil {
		return HealthResult{Reason: "invalid service URL"}
	}
	request.Header.Set("User-Agent", "MeowDeck/1 health-check")
	response, err := s.client().Do(request)
	if err != nil {
		return HealthResult{Reason: compactReason(err)}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 500 {
		return HealthResult{Reason: response.Status}
	}
	return HealthResult{Online: true, LatencyMS: time.Since(started).Milliseconds(), Reason: response.Status}
}

func classifyService(port int, body []byte, header http.Header) (string, string) {
	lower := strings.ToLower(string(body))
	switch {
	case port == 8123 || strings.Contains(lower, "home assistant"):
		return "Home Assistant", "home-assistant"
	case port == 32400 || strings.Contains(lower, "plex"):
		return "Plex", "plex"
	case strings.Contains(lower, "openwrt") || strings.Contains(lower, "luci"):
		return "LuCI", "luci"
	}
	match := titlePattern.FindSubmatch(body)
	if len(match) == 2 {
		name := strings.Join(strings.Fields(html.UnescapeString(string(match[1]))), " ")
		name = truncateRunes(name, 80)
		return name, "http"
	}
	if server := strings.TrimSpace(header.Get("Server")); server != "" {
		server = truncateRunes(server, 80)
		return server, "http"
	}
	return "", "http"
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func compactReason(err error) string {
	if err == nil {
		return ""
	}
	if netError, ok := err.(net.Error); ok && netError.Timeout() {
		return "timeout"
	}
	text := err.Error()
	if len(text) > 160 {
		text = text[:160]
	}
	return text
}
