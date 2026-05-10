package asset

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type certGenerator interface {
	GetCertificate(host string) (*tls.Certificate, error)
}

// Server hosts asset resources over HTTPS.
type Server struct {
	addr          string
	engine        *Engine
	certGenerator certGenerator
	httpServer    *http.Server
}

// NewServer creates a new HTTPS asset server bound to [host].
func NewServer(port int, engine *Engine, certGenerator certGenerator) (*Server, error) {
	if port == 0 {
		return nil, errors.New("port cannot be 0")
	}
	if engine == nil {
		return nil, errors.New("engine is nil")
	}
	if certGenerator == nil {
		return nil, errors.New("certGenerator is nil")
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	s := &Server{
		addr:          addr,
		engine:        engine,
		certGenerator: certGenerator,
	}

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s, nil
}

// ListenAndServe starts the HTTPS server and begins listening for requests.
func (s *Server) ListenAndServe() error {
	tlsConfig := &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.certGenerator.GetCertificate(host)
		},
		MinVersion: tls.VersionTLS12,
	}

	ln, err := tls.Listen("tcp", s.addr, tlsConfig)
	if err != nil {
		return err
	}

	log.Printf("assetserver: listening on address %s", s.addr)

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("assetserver: error serving: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the HTTPS server.
func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}

	return s.httpServer.Shutdown(ctx)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var raw string
	if ref := r.Header.Get("Referer"); ref != "" {
		raw = ref
	} else if origin := r.Header.Get("Origin"); origin != "" {
		raw = origin
	} else {
		http.Error(w, "missing Referer and Origin", http.StatusBadRequest)
		return
	}

	refererURL, err := url.Parse(raw)
	if err != nil {
		log.Printf("assetserver: invalid referer URL %q: %v", raw, err)
		http.Error(w, "invalid referer", http.StatusBadRequest)
		return
	}

	var (
		kind        kind
		contentType string
	)
	switch r.URL.Path {
	case cosmeticCSSPath:
		kind = cosmeticCSS
		contentType = "text/css; charset=utf-8"
	case cssRulePath:
		kind = cssRule
		contentType = "text/css; charset=utf-8"
	case scriptletsPath:
		kind = scriptlets
		contentType = "application/javascript; charset=utf-8"
	case extendedCSSPath:
		kind = extendedCSS
		contentType = "application/javascript; charset=utf-8"
	case jsRulePath:
		kind = jsRule
		contentType = "application/javascript; charset=utf-8"
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}

	body, err := s.engine.assetBytes(refererURL.Hostname(), kind)
	if err != nil {
		log.Printf("assetserver: failed to resolve asset %q: %v", r.URL.Path, err)
		http.Error(w, "asset resolution error", http.StatusInternalServerError)
		return
	}
	if len(body) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))

	w.WriteHeader(http.StatusOK)
	w.Write(body) // #nosec G705 -- body is from internal asset storage, not user input
}
