package whitelistserver

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Server struct {
	networkRules networkRules
	httpSrv      *http.Server
	port         int
}

type networkRules interface {
	ParseRule(rule string, filterName *string) (isException bool, err error)
}

func New(nr networkRules) *Server {
	return &Server{networkRules: nr}
}

func (s *Server) handleAllow(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	switch r.Method {
	case http.MethodGet:
		rule := r.URL.Query().Get("rule")
		returnTo := r.URL.Query().Get("returnTo")

		if rule == "" {
			http.Error(w, "missing rule", http.StatusBadRequest)
			return
		}
		if returnTo == "" {
			http.Error(w, "missing returnTo", http.StatusBadRequest)
			return
		}

		if len(rule) > 2048 {
			http.Error(w, "rule too long", http.StatusBadRequest)
			return
		}
		if len(returnTo) > 4096 {
			http.Error(w, "return url too long", http.StatusBadRequest)
			return
		}

		u, err := url.Parse(returnTo)
		if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
			http.Error(w, "invalid returnTo url", http.StatusBadRequest)
			return
		}

		filterList := "Allowlist"
		if _, err := s.networkRules.ParseRule(fmt.Sprintf("@@%s", rule), &filterList); err != nil {
			http.Error(w, "networkrules: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, returnTo, http.StatusSeeOther) // #nosec G710 -- local allowlist endpoint intentionally returns to the originally blocked URL.
		return

	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return

	default:
		w.Header().Set("Allow", "GET, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/allow-rule", s.handleAllow)

	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}

	addr := fmt.Sprintf("127.0.0.1:%d", 0) // random port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	s.port = actualPort

	log.Printf("whitelist server listening on port %d", actualPort)

	go func() {
		if err := s.httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("error serving whitelist server: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	if s.httpSrv != nil {
		if err := s.httpSrv.Close(); err != nil {
			return fmt.Errorf("close: %v", err)
		}
	}
	return nil
}

func (s *Server) GetPort() int {
	return s.port
}
