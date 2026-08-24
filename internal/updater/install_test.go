package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallKeepsLastKnownGoodAfterSuccess(t *testing.T) {
	directory := t.TempDir()
	installPath := filepath.Join(directory, "meowdeck")
	candidate := filepath.Join(t.TempDir(), "candidate")
	serviceScript := filepath.Join(directory, "service")
	writeExecutable(t, installPath, "old")
	writeExecutable(t, candidate, "new")
	writeExecutable(t, serviceScript, "#!/bin/sh\nexit 0\n")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" {
			_, _ = writer.Write([]byte("ok\n"))
			return
		}
		_, _ = writer.Write([]byte(`{"version":"v2"}`))
	}))
	defer server.Close()
	options := Options{
		InstallPath: installPath, ServiceScript: serviceScript, HTTPClient: server.Client(),
		HealthURL: server.URL + "/healthz", VersionURL: server.URL + "/api/v1/version",
	}
	if err := installAndVerify(context.Background(), options, candidate, "v2"); err != nil {
		t.Fatal(err)
	}
	assertFile(t, installPath, "new")
	assertFile(t, filepath.Join(directory, "meowdeck.previous"), "old")
}

func TestInstallRollbackPreservesExistingPrevious(t *testing.T) {
	directory := t.TempDir()
	installPath := filepath.Join(directory, "meowdeck")
	candidate := filepath.Join(t.TempDir(), "candidate")
	serviceScript := filepath.Join(directory, "service")
	writeExecutable(t, installPath, "current")
	writeExecutable(t, filepath.Join(directory, "meowdeck.previous"), "older")
	writeExecutable(t, candidate, "broken")
	writeExecutable(t, serviceScript, "#!/bin/sh\nexit 0\n")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" {
			_, _ = writer.Write([]byte("ok\n"))
			return
		}
		_, _ = writer.Write([]byte(`{"version":"wrong"}`))
	}))
	defer server.Close()
	options := Options{
		InstallPath: installPath, ServiceScript: serviceScript, HTTPClient: server.Client(),
		HealthURL: server.URL + "/healthz", VersionURL: server.URL + "/api/v1/version",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := installAndVerify(ctx, options, candidate, "v2"); err == nil {
		t.Fatal("expected exact-version health check to fail")
	}
	assertFile(t, installPath, "current")
	assertFile(t, filepath.Join(directory, "meowdeck.previous"), "older")
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, expected string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != expected {
		t.Fatalf("%s contains %q, want %q", path, raw, expected)
	}
}
