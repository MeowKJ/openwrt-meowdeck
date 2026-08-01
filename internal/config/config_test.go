package config

import "testing"

func TestDefaultIsValid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if cfg.Hostname != "meow.lan" {
		t.Fatalf("unexpected hostname %q", cfg.Hostname)
	}
	if len(cfg.Services) != 2 || !cfg.Services[0].Protected || !cfg.Services[1].Protected {
		t.Fatalf("default config should contain two protected services: %#v", cfg.Services)
	}
}

func TestDuplicateServiceFails(t *testing.T) {
	cfg := Default()
	cfg.Services = append(cfg.Services, cfg.Services[0])
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate service validation error")
	}
}

func TestInvalidCategoryFails(t *testing.T) {
	cfg := Default()
	cfg.Services[0].Category = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected category validation error")
	}
}
