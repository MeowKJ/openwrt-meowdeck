package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/catalog"
)

type Catalog interface {
	Snapshot() catalog.Snapshot
	Device(string) (catalog.Device, []catalog.Service, bool)
}

type Server struct {
	http    *http.Server
	catalog Catalog
	assets  fs.FS
	version string
}

func New(listen string, source Catalog, assets fs.FS, version string) *Server {
	server := &Server{catalog: source, assets: assets, version: version}
	server.http = &http.Server{Addr: listen, Handler: server.routes(), ReadHeaderTimeout: 5 * time.Second}
	return server
}

func (s *Server) HTTPServer() *http.Server { return s.http }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": s.version})
	})
	mux.HandleFunc("GET /api/v1/catalog", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.catalog.Snapshot())
	})
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.catalog.Snapshot())
	})
	mux.HandleFunc("GET /api/v1/incidents", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.catalog.Snapshot().Incidents)
	})
	mux.HandleFunc("GET /api/v1/devices/{id}", func(w http.ResponseWriter, request *http.Request) {
		device, services, ok := s.catalog.Device(request.PathValue("id"))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Device   catalog.Device    `json:"device"`
			Services []catalog.Service `json:"services"`
		}{Device: device, Services: services})
	})
	mux.Handle("/", s.staticHandler())
	return securityHeaders(mux)
}

func (s *Server) staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		requested := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if requested == "." || requested == "" {
			requested = "index.html"
		}
		if _, err := fs.Stat(s.assets, requested); err != nil {
			requested = "index.html"
		}
		payload, err := fs.ReadFile(s.assets, requested)
		if err != nil {
			slog.Error("read embedded asset", "path", requested, "error", err)
			http.Error(w, "frontend not built", http.StatusServiceUnavailable)
			return
		}
		if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if strings.Contains(path.Base(requested), "-") && requested != "index.html" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if request.Method == http.MethodGet {
			_, _ = w.Write(payload)
		}
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, request)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode response", "error", err)
	}
}
