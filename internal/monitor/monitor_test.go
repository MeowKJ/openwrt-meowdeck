package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MeowKJ/openwrt-meowdeck/internal/config"
)

func TestHTTPProbeAndHistory(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	cfg := config.Default()
	cfg.Services = []config.Service{{
		ID: "test", Name: "Test", Category: "network", Icon: "cpu",
		Probe: config.Probe{Type: "http", Target: target.URL, ExpectedStatus: []int{http.StatusNoContent}},
	}}
	cfg.HistorySize = 2

	manager := New(cfg, "test")
	manager.CheckNow(context.Background())
	manager.CheckNow(context.Background())
	manager.CheckNow(context.Background())
	snapshot := manager.Snapshot()
	if snapshot.Services[0].State != StateOnline {
		t.Fatalf("expected online, got %s", snapshot.Services[0].State)
	}
	if len(snapshot.Services[0].History) != 2 {
		t.Fatalf("expected capped history of 2, got %d", len(snapshot.Services[0].History))
	}
}
