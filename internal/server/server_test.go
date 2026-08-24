package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/catalog"
)

type fakeCatalog struct {
	snapshot catalog.Snapshot
}

func (f fakeCatalog) Snapshot() catalog.Snapshot { return f.snapshot }
func (f fakeCatalog) Device(id string) (catalog.Device, []catalog.Service, bool) {
	for _, device := range f.snapshot.Devices {
		if device.ID == id {
			return device, nil, true
		}
	}
	return catalog.Device{}, nil, false
}

func testAssets(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>MeowDeck</title>")}}
}

func TestReadOnlyAPI(t *testing.T) {
	server := New("127.0.0.1:0", fakeCatalog{snapshot: catalog.Snapshot{
		Version: "test", GeneratedAt: time.Now(), Devices: []catalog.Device{{ID: "device"}},
	}}, testAssets(t), "test")
	for _, requestPath := range []string{"/healthz", "/api/v1/version", "/api/v1/catalog", "/api/v1/incidents", "/api/v1/devices/device"} {
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		recorder := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s returned %d", requestPath, recorder.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/catalog", nil)
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed && recorder.Code != http.StatusNotFound {
		t.Fatalf("write request unexpectedly accepted: %d", recorder.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	server := New("127.0.0.1:0", fakeCatalog{}, testAssets(t), "test")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing content security policy")
	}
}
