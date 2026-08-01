package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	DefaultListen          = "127.0.0.1:9080"
	DefaultHostname        = "meow.lan"
	DefaultIntervalSeconds = 30
	DefaultTimeoutSeconds  = 4
	DefaultHistorySize     = 120
)

type Probe struct {
	Type           string `json:"type"`
	Target         string `json:"target"`
	ExpectedStatus []int  `json:"expectedStatus,omitempty"`
}

type Service struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Subdomain   string `json:"subdomain,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	URL         string `json:"url,omitempty"`
	Proxy       bool   `json:"proxy,omitempty"`
	Protected   bool   `json:"protected,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
	Probe       Probe  `json:"probe"`
}

type Config struct {
	Listen          string            `json:"listen"`
	Hostname        string            `json:"hostname"`
	IntervalSeconds int               `json:"intervalSeconds"`
	TimeoutSeconds  int               `json:"timeoutSeconds"`
	HistorySize     int               `json:"historySize"`
	Redirects       map[string]string `json:"redirects"`
	Services        []Service         `json:"services"`
}

func Default() Config {
	return Config{
		Listen:          DefaultListen,
		Hostname:        DefaultHostname,
		IntervalSeconds: DefaultIntervalSeconds,
		TimeoutSeconds:  DefaultTimeoutSeconds,
		HistorySize:     DefaultHistorySize,
		Redirects:       map[string]string{},
		Services: []Service{
			{
				ID: "router", Slug: "router", Name: "GL.iNet 管理后台", Description: "GL-MT3600BE · 端口 80",
				Category: "network", Icon: "router", URL: "http://192.168.8.1/", Protected: true,
				Probe: Probe{Type: "http", Target: "http://127.0.0.1/", ExpectedStatus: []int{200, 302, 401, 403}},
			},
			{
				ID: "luci", Slug: "luci", Name: "LuCI 高级管理", Description: "OpenWrt · 端口 8080",
				Category: "network", Icon: "terminal", URL: "http://192.168.8.1:8080/cgi-bin/luci/", Protected: true,
				Probe: Probe{Type: "tcp", Target: "127.0.0.1:8080"},
			},
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	}
	if cfg.Hostname == "" {
		cfg.Hostname = DefaultHostname
	}
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = DefaultIntervalSeconds
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if cfg.HistorySize <= 0 {
		cfg.HistorySize = DefaultHistorySize
	}
	if cfg.Redirects == nil {
		cfg.Redirects = map[string]string{}
	}
	for i := range cfg.Services {
		if cfg.Services[i].Slug == "" {
			cfg.Services[i].Slug = cfg.Services[i].ID
		}
	}
}

var localNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var reservedSlugs = map[string]struct{}{
	"api": {}, "healthz": {}, "assets": {},
}
var allowedCategories = map[string]struct{}{
	"network": {}, "automation": {}, "smart-home": {}, "device": {},
}

func (cfg Config) Validate() error {
	seen := make(map[string]struct{}, len(cfg.Services))
	seenSlugs := make(map[string]struct{}, len(cfg.Services))
	seenSubdomains := make(map[string]struct{}, len(cfg.Services))
	for i, service := range cfg.Services {
		if strings.TrimSpace(service.ID) == "" {
			return fmt.Errorf("service %d: id is required", i)
		}
		if _, exists := seen[service.ID]; exists {
			return fmt.Errorf("service %q: duplicate id", service.ID)
		}
		seen[service.ID] = struct{}{}
		if !localNamePattern.MatchString(service.Slug) {
			return fmt.Errorf("service %q: invalid slug %q", service.ID, service.Slug)
		}
		if _, reserved := reservedSlugs[service.Slug]; reserved {
			return fmt.Errorf("service %q: slug %q is reserved", service.ID, service.Slug)
		}
		if _, exists := seenSlugs[service.Slug]; exists {
			return fmt.Errorf("service %q: duplicate slug %q", service.ID, service.Slug)
		}
		seenSlugs[service.Slug] = struct{}{}
		if service.Subdomain != "" {
			if !localNamePattern.MatchString(service.Subdomain) {
				return fmt.Errorf("service %q: invalid subdomain %q", service.ID, service.Subdomain)
			}
			if _, exists := seenSubdomains[service.Subdomain]; exists {
				return fmt.Errorf("service %q: duplicate subdomain %q", service.ID, service.Subdomain)
			}
			seenSubdomains[service.Subdomain] = struct{}{}
		}
		if service.Name == "" || service.Category == "" {
			return fmt.Errorf("service %q: name and category are required", service.ID)
		}
		if _, allowed := allowedCategories[service.Category]; !allowed {
			return fmt.Errorf("service %q: unsupported category %q", service.ID, service.Category)
		}
		if service.URL != "" {
			parsed, err := url.Parse(service.URL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return fmt.Errorf("service %q: url must be an absolute HTTP or HTTPS URL", service.ID)
			}
		}
		if !service.Disabled && (service.Probe.Type == "" || service.Probe.Target == "") {
			return fmt.Errorf("service %q: enabled service requires a probe type and target", service.ID)
		}
		switch service.Probe.Type {
		case "http", "tcp", "ping", "process", "":
		default:
			return fmt.Errorf("service %q: unsupported probe type %q", service.ID, service.Probe.Type)
		}
	}
	return nil
}

func Save(path string, cfg Config) error {
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("chmod temporary config: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
