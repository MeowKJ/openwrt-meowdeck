package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIsZeroConfigurationLANOnly(t *testing.T) {
	cfg := Default()
	if cfg.Hostname != "meow.lan" || len(cfg.LANInterfaces) != 1 || cfg.LANInterfaces[0] != "lan" {
		t.Fatalf("unexpected default config: %#v", cfg)
	}
}

func TestLegacyConfigurationLoadsWithoutManagementFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := []byte(`{"listen":"127.0.0.1:9080","hostname":"meow.lan","adminToken":"old","services":[{"id":"old"}]}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LANInterfaces) != 1 || cfg.LANInterfaces[0] != "lan" {
		t.Fatalf("legacy config did not receive safe defaults: %#v", cfg)
	}
}

func TestRejectsDuplicateOrUnsafeInterfaces(t *testing.T) {
	cfg := Default()
	cfg.LANInterfaces = []string{"lan", "lan"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate interface rejection")
	}
	cfg.LANInterfaces = []string{"wan;reboot"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsafe interface rejection")
	}
}
