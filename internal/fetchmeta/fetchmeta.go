// Package fetchmeta classifies proxied requests using Fetch Metadata request headers.
//
// Reference: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Sec-Fetch-Dest
package fetchmeta

import (
	"net/http"
	"strings"
)

// IsUserNavigation reports whether req is a top-level navigation, started by the user
// or by a script, based on Fetch Metadata request headers. Zen uses it to show the
// block page instead of a plain 403.
//
// Note: browsers only send Fetch Metadata headers to HTTPS endpoints,
// so this always returns false for plain-HTTP requests.
func IsUserNavigation(req *http.Request) bool {
	dest := req.Header.Get("Sec-Fetch-Dest")
	mode := req.Header.Get("Sec-Fetch-Mode")
	user := req.Header.Get("Sec-Fetch-User")

	return dest == "document" && (mode == "navigate" || user == "?1")
}

// IsUserActivatedNavigation reports whether req is a top-level navigation that the
// user started directly, such as by clicking a link, based on Fetch Metadata request
// headers. Block rules without $document or $all let these navigations through.
//
// It is the strictest of the three navigation checks. [IsUserNavigation] also
// accepts navigations a script started without user activation, and
// [IsLikelyUserNavigation] also accepts frames and guesses from the Accept header.
// Keep it the strictest: with a looser check, rules without $document or $all would
// stop blocking script-driven redirects to trackers and malware.
func IsUserActivatedNavigation(req *http.Request) bool {
	return req.Header.Get("Sec-Fetch-Dest") == "document" && req.Header.Get("Sec-Fetch-User") == "?1"
}

// IsLikelyUserNavigation reports whether req looks like a browser navigation whose
// response the user will see rendered as a page: a top-level navigation of any
// method, or a frame. For requests that carry no Fetch Metadata headers
// (plain-HTTP requests, browsers predating Fetch Metadata), it falls back to an
// Accept-header heuristic.
//
// This is deliberately broader than [IsUserNavigation]: it exists to pick the body
// format of an error response, where a false positive only means an HTML body on
// an already-failed request. Don't use it for filtering decisions.
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
