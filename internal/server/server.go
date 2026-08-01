package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/config"
	"github.com/MeowKJ/openwrt-meowdeck/internal/monitor"
)

const editHeader = "X-MeowDeck-Edit"

type Server struct {
	http       *http.Server
	monitor    *monitor.Manager
	assets     fs.FS
	configMu   sync.RWMutex
	cfg        config.Config
	configPath string
}

func New(listen string, manager *monitor.Manager, assets fs.FS, cfg config.Config, configPath string) *Server {
	server := &Server{monitor: manager, assets: assets, cfg: cfg, configPath: configPath}
	server.http = &http.Server{
		Addr:              listen,
		Handler:           server.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server
}

func (s *Server) HTTPServer() *http.Server { return s.http }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.monitor.Snapshot())
	})
	mux.HandleFunc("POST /api/v1/services", s.handleAddService)
	mux.HandleFunc("DELETE /api/v1/services/{id}", s.handleDeleteService)
	mux.Handle("/", s.aliasOrStaticHandler())
	return securityHeaders(mux)
}

func (s *Server) handleAddService(w http.ResponseWriter, r *http.Request) {
	if !allowEdit(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing edit header"})
		return
	}
	var service config.Service
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&service); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid service payload"})
		return
	}
	if service.Slug == "" {
		service.Slug = service.ID
	}
	if service.ID == "" {
		service.ID = service.Slug
	}
	service.Protected = false
	if service.Category == "" {
		service.Category = "device"
	}
	if service.Icon == "" {
		service.Icon = "cpu"
	}
	if service.Description == "" {
		service.Description = "自定义本地服务"
	}
	if service.Probe.Type == "" && service.URL != "" {
		service.Probe = config.Probe{Type: "http", Target: service.URL}
	}

	s.configMu.Lock()
	next := s.cfg
	next.Services = append(append([]config.Service(nil), s.cfg.Services...), service)
	if err := config.Save(s.configPath, next); err != nil {
		s.configMu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.cfg = next
	s.configMu.Unlock()
	s.monitor.ReplaceServices(next.Services)
	go s.monitor.CheckNow(context.Background())
	writeJSON(w, http.StatusCreated, service)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	if !allowEdit(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing edit header"})
		return
	}
	id := r.PathValue("id")
	s.configMu.Lock()
	next := s.cfg
	next.Services = make([]config.Service, 0, len(s.cfg.Services))
	found := false
	for _, service := range s.cfg.Services {
		if service.ID != id {
			next.Services = append(next.Services, service)
			continue
		}
		found = true
		if service.Protected {
			s.configMu.Unlock()
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "built-in services cannot be removed"})
			return
		}
	}
	if !found {
		s.configMu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}
	if err := config.Save(s.configPath, next); err != nil {
		s.configMu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.cfg = next
	s.configMu.Unlock()
	s.monitor.ReplaceServices(next.Services)
	w.WriteHeader(http.StatusNoContent)
}

func allowEdit(r *http.Request) bool {
	return r.Header.Get(editHeader) == "1"
}

func (s *Server) aliasOrStaticHandler() http.Handler {
	static := s.staticHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service, suffix, bySubdomain, found := s.resolveAlias(r)
		if !found {
			static.ServeHTTP(w, r)
			return
		}
		if service.URL == "" {
			http.Error(w, "service URL is not configured", http.StatusNotFound)
			return
		}
		if bySubdomain && service.Proxy {
			s.proxyService(w, r, service)
			return
		}
		if !bySubdomain && service.Subdomain != "" {
			target := "http://" + service.Subdomain + "." + s.hostname() + suffix
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			return
		}
		target := strings.TrimRight(service.URL, "/") + suffix
		if suffix == "" {
			target = service.URL
		}
		if r.URL.RawQuery != "" {
			separator := "?"
			if strings.Contains(target, "?") {
				separator = "&"
			}
			target += separator + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	})
}

func (s *Server) resolveAlias(r *http.Request) (config.Service, string, bool, bool) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	host := strings.ToLower(r.Host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	base := strings.ToLower(s.cfg.Hostname)
	if strings.HasSuffix(host, "."+base) {
		alias := strings.TrimSuffix(host, "."+base)
		for _, service := range s.cfg.Services {
			if service.Subdomain == alias {
				return service, r.URL.Path, true, true
			}
		}
	}
	cleaned := strings.Trim(r.URL.Path, "/")
	if cleaned == "" {
		return config.Service{}, "", false, false
	}
	parts := strings.SplitN(cleaned, "/", 2)
	for _, service := range s.cfg.Services {
		if service.Slug == parts[0] {
			suffix := ""
			if len(parts) == 2 {
				suffix = "/" + parts[1]
			}
			return service, suffix, false, true
		}
	}
	return config.Service{}, "", false, false
}

func (s *Server) hostname() string {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.cfg.Hostname
}

func (s *Server) proxyService(w http.ResponseWriter, r *http.Request, service config.Service) {
	target, err := url.Parse(service.URL)
	if err != nil {
		http.Error(w, "invalid proxy target", http.StatusBadGateway)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		originalDirector(request)
		request.Host = target.Host
	}
	proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, proxyErr error) {
		slog.Warn("service proxy failed", "service", service.ID, "error", proxyErr)
		http.Error(writer, "service unavailable", http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

func (s *Server) staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
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
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
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
