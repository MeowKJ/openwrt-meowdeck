package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/MeowKJ/openwrt-meowdeck/internal/config"
	"github.com/MeowKJ/openwrt-meowdeck/internal/monitor"
)

func TestStatusAndRedirect(t *testing.T) {
	cfg := config.Default()
	manager := monitor.New(cfg, "test-version")
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<h1>MeowDeck</h1>")}}
	server := New("127.0.0.1:0", manager, fs.FS(assets), cfg, t.TempDir()+"/config.json")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	recorder := httptest.NewRecorder()
	server.HTTPServer().Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status response %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/router", nil).WithContext(context.Background())
	recorder = httptest.NewRecorder()
	server.HTTPServer().Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTemporaryRedirect {
		t.Fatalf("unexpected redirect response %d", recorder.Code)
	}
}

func TestAddAndDeleteCustomService(t *testing.T) {
	cfg := config.Default()
	manager := monitor.New(cfg, "test-version")
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<h1>MeowDeck</h1>")}}
	server := New("127.0.0.1:0", manager, fs.FS(assets), cfg, t.TempDir()+"/config.json")

	service := config.Service{
		ID: "farm", Slug: "farm", Subdomain: "grow", Name: "Farm", Category: "automation", Icon: "sprout",
		URL: "http://192.168.8.178:8081", Probe: config.Probe{Type: "http", Target: "http://192.168.8.178:8081"},
	}
	payload, err := json.Marshal(service)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewReader(payload))
	request.Header.Set(editHeader, "1")
	recorder := httptest.NewRecorder()
	server.HTTPServer().Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("unexpected create response %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/farm", nil)
	recorder = httptest.NewRecorder()
	server.HTTPServer().Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTemporaryRedirect || recorder.Header().Get("Location") != "http://grow.meow.lan" {
		t.Fatalf("unexpected alias redirect: %d %q", recorder.Code, recorder.Header().Get("Location"))
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/services/farm", nil)
	request.Header.Set(editHeader, "1")
	recorder = httptest.NewRecorder()
	server.HTTPServer().Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected delete response %d", recorder.Code)
	}
}

func TestBuiltInServiceCannotBeDeleted(t *testing.T) {
	cfg := config.Default()
	manager := monitor.New(cfg, "test-version")
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<h1>MeowDeck</h1>")}}
	server := New("127.0.0.1:0", manager, fs.FS(assets), cfg, t.TempDir()+"/config.json")

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/services/router", nil)
	request.Header.Set(editHeader, "1")
	recorder := httptest.NewRecorder()
	server.HTTPServer().Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected delete response %d", recorder.Code)
	}
}

func TestAddRequiresEditHeader(t *testing.T) {
	cfg := config.Default()
	manager := monitor.New(cfg, "test-version")
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<h1>MeowDeck</h1>")}}
	server := New("127.0.0.1:0", manager, fs.FS(assets), cfg, t.TempDir()+"/config.json")

	request := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(`{}`))
	recorder := httptest.NewRecorder()
	server.HTTPServer().Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected create response %d", recorder.Code)
	}
}
