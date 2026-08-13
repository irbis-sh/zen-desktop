package asset

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
)

// Handler serves injected page assets. It is mounted on the proxy under
// constants.LocalEndpointHost rather than a listener of its own: proxied
// requests are exempt from browsers' local-network-access permission checks,
// while a socket on 127.0.0.1 is not.
type Handler struct {
	engine *Engine
}

// NewHandler creates a Handler serving assets from engine.
func NewHandler(engine *Engine) *Handler {
	return &Handler{engine: engine}
}

// assetContentTypes maps the asset paths the handler answers for to their
// content types; any other path is a 404.
var assetContentTypes = map[string]string{
	cosmeticCSSPath: "text/css; charset=utf-8",
	cssRulePath:     "text/css; charset=utf-8",
	scriptletsPath:  "application/javascript; charset=utf-8",
	extendedCSSPath: "application/javascript; charset=utf-8",
	jsRulePath:      "application/javascript; charset=utf-8",
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	raw := r.Header.Get("Referer")
	if raw == "" {
		raw = r.Header.Get("Origin")
	}
	if raw == "" {
		http.Error(w, "missing Referer and Origin", http.StatusBadRequest)
		return
	}

	refererURL, err := url.Parse(raw)
	if err != nil {
		log.Printf("asset: invalid referer URL %q: %v", raw, err)
		http.Error(w, "invalid referer", http.StatusBadRequest)
		return
	}

	contentType, ok := assetContentTypes[r.URL.Path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	body, err := h.engine.assetBytes(refererURL.Hostname(), r.URL.Path)
	if err != nil {
		log.Printf("asset: failed to resolve asset %q: %v", r.URL.Path, err)
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
