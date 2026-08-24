package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
)

const (
	DefaultListen   = "127.0.0.1:9080"
	DefaultHostname = "meow.lan"
)

type Config struct {
	Listen        string   `json:"listen"`
	Hostname      string   `json:"hostname"`
	LANInterfaces []string `json:"lanInterfaces"`
}

func Default() Config {
	return Config{Listen: DefaultListen, Hostname: DefaultHostname, LANInterfaces: []string{"lan"}}
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
	if len(cfg.LANInterfaces) == 0 {
		cfg.LANInterfaces = []string{"lan"}
	}
}

var localNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var interfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)

func (cfg Config) Validate() error {
	if err := validateHostname(cfg.Hostname); err != nil {
		return err
	}
	if len(cfg.LANInterfaces) == 0 || len(cfg.LANInterfaces) > 8 {
		return fmt.Errorf("lanInterfaces must contain between 1 and 8 logical interfaces")
	}
	seen := map[string]bool{}
	for _, name := range cfg.LANInterfaces {
		if !interfacePattern.MatchString(name) {
			return fmt.Errorf("invalid LAN interface %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate LAN interface %q", name)
		}
		seen[name] = true
	}
	return nil
}

func validateHostname(hostname string) error {
	if len(hostname) > 253 || net.ParseIP(hostname) != nil {
		return fmt.Errorf("hostname %q is not a valid DNS name", hostname)
	}
	if hostname != strings.ToLower(hostname) {
		return fmt.Errorf("hostname %q must use lowercase letters", hostname)
	}
	for _, label := range strings.Split(hostname, ".") {
		if !localNamePattern.MatchString(label) {
			return fmt.Errorf("hostname %q is not a valid DNS name", hostname)
		}
	}
	return nil
}
