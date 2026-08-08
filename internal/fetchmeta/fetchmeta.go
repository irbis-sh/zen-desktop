// Package fetchmeta classifies proxied requests using Fetch Metadata request headers.
//
// Reference: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Sec-Fetch-Dest
package fetchmeta

import (
	"net/http"
	"strings"
)

// IsUserNavigation reports whether req is a user-initiated top-level navigation,
// based on Fetch Metadata request headers.
//
// Note: browsers only send Fetch Metadata headers to HTTPS endpoints,
// so this always returns false for plain-HTTP requests.
func IsUserNavigation(req *http.Request) bool {
	dest := req.Header.Get("Sec-Fetch-Dest")
	mode := req.Header.Get("Sec-Fetch-Mode")
	user := req.Header.Get("Sec-Fetch-User")

	return dest == "document" && (mode == "navigate" || user == "?1")
}

// IsLikelyUserNavigation reports whether req looks like a browser navigation whose
// response the user will see rendered as a page: a top-level navigation of any
// method, or a frame. For requests that carry no Fetch Metadata headers
// (plain-HTTP requests, browsers predating Fetch Metadata), it falls back to an
// Accept-header heuristic.
//
// This is deliberately broader than [IsUserNavigation]: it exists to pick the body
// format of an error response, where a false positive only means an HTML body on
// an already-failed request. Keep filtering decisions on [IsUserNavigation].
func IsLikelyUserNavigation(req *http.Request) bool {
	dest := req.Header.Get("Sec-Fetch-Dest")
	mode := req.Header.Get("Sec-Fetch-Mode")
	user := req.Header.Get("Sec-Fetch-User")

	switch dest {
	case "document", "iframe", "frame":
		return mode == "navigate" || user == "?1"
	case "":
		return strings.Contains(req.Header.Get("Accept"), "text/html")
	default:
		return false
	}
}
