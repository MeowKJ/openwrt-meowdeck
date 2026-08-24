package discovery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/MeowKJ/openwrt-meowdeck/internal/catalog"
)

func TestClassifyServiceUsesKnownPortAndEscapedTitle(t *testing.T) {
	name, kind := classifyService(8123, []byte("<title>anything</title>"), http.Header{})
	if name != "Home Assistant" || kind != "home-assistant" {
		t.Fatalf("unexpected classification: %q %q", name, kind)
	}
	name, _ = classifyService(8080, []byte("<title>A &amp; B</title>"), http.Header{})
	if name != "A & B" {
		t.Fatalf("unexpected title: %q", name)
	}
}

func TestHTTPDiscoveryNeverFollowsRedirects(t *testing.T) {
	var externalRequests atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		externalRequests.Add(1)
		_, _ = writer.Write([]byte("<title>External</title>"))
	}))
	defer external.Close()
	local := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", external.URL)
		writer.WriteHeader(http.StatusFound)
	}))
	defer local.Close()
	parsed, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewHTTPScanner()
	_, _ = scanner.inspect(context.Background(), catalog.Device{ID: "device", IPv4: host, Name: "Local"}, "http", port, "/")
	if externalRequests.Load() != 0 {
		t.Fatal("LAN service redirected the discovery client to another host")
	}
}
