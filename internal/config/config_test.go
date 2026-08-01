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

func TestHostnameValidation(t *testing.T) {
	for _, hostname := range []string{"meow.lan", "deck.home.arpa", "a-b.example"} {
		cfg := Default()
		cfg.Hostname = hostname
		if err := cfg.Validate(); err != nil {
			t.Fatalf("hostname %q should be valid: %v", hostname, err)
		}
	}
	for _, hostname := range []string{"Meow.lan", "-meow.lan", "meow..lan", "192.168.8.1", "meow.lan;reboot"} {
		cfg := Default()
		cfg.Hostname = hostname
		if err := cfg.Validate(); err == nil {
			t.Fatalf("hostname %q should be rejected", hostname)
		}
	}
}
