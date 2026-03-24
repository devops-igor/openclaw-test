package serve

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config holds the share server configuration.
type Config struct {
	Host           string // Public URL base (e.g. "http://your-ip:port")
	Port           int    // Listener port (default 8080, 0 = OS assigns)
	TimeoutMinutes int    // Auto-shutdown timeout in minutes
}

// Server is a temporary HTTP server that serves a single file.
type Server struct {
	Port int // actual bound port, set after net.Listen succeeds

	cfg    Config
	file   string // absolute path to the file
	logger *slog.Logger

	mu       sync.Mutex
	done     chan struct{} // closed when download completes or timeout
	listener net.Listener
}

// New creates a new file-serving Server.
func New(cfg Config, filePath string, logger *slog.Logger) *Server {
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.TimeoutMinutes == 0 {
		cfg.TimeoutMinutes = 30
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:    cfg,
		file:   filePath,
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Filename returns the base name of the served file.
func (s *Server) Filename() string {
	return filepath.Base(s.file)
}

// GetPort returns the actual bound port of the server.
func (s *Server) GetPort() int {
	return s.Port
}

// Serve starts the HTTP server and blocks until the file is downloaded or timeout.
// Returns the public URL used to download the file, or an error.
func (s *Server) Serve(ctx context.Context) (string, error) {
	filename := s.Filename()
	// Sanitize: only base name, no path traversal
	safeName := filepath.Base(filename)
	// URL-encode the filename so the route pattern doesn't contain spaces or special chars
	encodedName := url.PathEscape(safeName)

	mux := http.NewServeMux()
	mux.HandleFunc("/file/"+encodedName, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.logger.Info("serving file download", "filename", safeName, "remote", r.RemoteAddr)
		// Escape quotes in filename to prevent header injection
		escapedName := strings.ReplaceAll(safeName, `"`, `\"`)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, escapedName))
		http.ServeFile(w, r, s.file)
		// Signal download complete
		s.mu.Lock()
		defer s.mu.Unlock()
		select {
		case <-s.done:
			// already closed
		default:
			close(s.done)
			s.logger.Info("file download complete", "filename", safeName)
		}
	})

	// Try configured port, fall back to OS-assigned port
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.logger.Warn("configured port unavailable, using OS-assigned port", "port", s.cfg.Port, "error", err)
		ln, err = net.Listen("tcp", ":0")
		if err != nil {
			return "", fmt.Errorf("listen: %w", err)
		}
	}
	s.listener = ln

	actualPort := ln.Addr().(*net.TCPAddr).Port
	s.Port = actualPort
	publicURL := s.BuildURL(actualPort, encodedName)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	// Start serving in background
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("share server started", "url", publicURL, "port", actualPort)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for: download complete, timeout, or context cancellation
	timeout := time.Duration(s.cfg.TimeoutMinutes) * time.Minute
	select {
	case <-s.done:
		s.logger.Info("download completed, shutting down share server", "filename", safeName)
	case <-time.After(timeout):
		s.logger.Info("share server timeout, shutting down", "filename", safeName, "timeout_min", s.cfg.TimeoutMinutes)
	case <-ctx.Done():
		s.logger.Info("context cancelled, shutting down share server", "filename", safeName)
	case err := <-errCh:
		s.logger.Error("share server error", "error", err)
		_ = srv.Shutdown(context.Background())
		s.cleanup()
		return "", fmt.Errorf("server error: %w", err)
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	s.cleanup()
	return publicURL, nil
}

// BuildURL constructs the public download URL.
func (s *Server) BuildURL(port int, filename string) string {
	host := s.cfg.Host
	if host == "" {
		// Default to local IP
		host = fmt.Sprintf("http://%s", getLocalIP())
	}
	// Remove trailing slash
	host = strings.TrimRight(host, "/")
	// If host doesn't include a port, append the actual port
	if !strings.Contains(host, fmt.Sprintf(":%d", port)) {
		// Strip any existing port from host
		if idx := strings.LastIndex(host, ":"); idx > strings.LastIndex(host, "://")+2 {
			host = host[:idx]
		}
		host = fmt.Sprintf("%s:%d", host, port)
	}
	return fmt.Sprintf("%s/file/%s", host, filename)
}

// cleanup deletes the served file.
func (s *Server) cleanup() {
	if s.file != "" {
		if err := os.Remove(s.file); err != nil {
			s.logger.Warn("failed to remove temp file", "file", s.file, "error", err)
		} else {
			s.logger.Info("temp file removed", "file", s.file)
		}
	}
}

// getLocalIP returns the first non-loopback IPv4 address.
func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "localhost:0"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip != nil {
				return ip.String()
			}
		}
	}
	return "localhost"
}
