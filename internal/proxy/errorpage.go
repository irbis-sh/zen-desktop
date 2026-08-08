package proxy

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"html/template"
	"log"
	"net"
	"net/http"

	"github.com/irbis-sh/zen-desktop/internal/fetchmeta"
	"github.com/irbis-sh/zen-desktop/internal/pagestyle"
)

//go:embed errorpage.html
var errorPageHTML string

var errorPageTmpl = template.Must(template.New("errorpage").Parse(errorPageHTML))

type errorPageData struct {
	SharedStyle template.CSS
	Title       string
	Message     string
	// Detail is the raw error string, shown in a collapsed section for debugging.
	// It may contain the request URL, which is fine: the page is only served to
	// the client that requested that URL.
	Detail string
}

// writeUpstreamError responds to a failed upstream request: a friendly HTML error
// page for browser navigations, a plain-text 502 otherwise.
func writeUpstreamError(w http.ResponseWriter, req *http.Request, err error) {
	if isTLSError(err) {
		// connectHandler adds hosts that fail TLS to the transparent list, which
		// only takes effect on a fresh CONNECT. Close the client connection so a
		// retry cannot reuse the intercepted session and reconnects unfiltered.
		// Go's HTTP/2 server translates this header into a GOAWAY frame.
		w.Header().Set("Connection", "close")
	}

	if !fetchmeta.IsLikelyUserNavigation(req) {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	title, message := classifyUpstreamError(err)
	writeErrorPage(w, title, message, err.Error())
}

// writeFilterError responds to an internal filtering failure: a friendly HTML error
// page for browser navigations, a plain-text 502 otherwise.
func writeFilterError(w http.ResponseWriter, req *http.Request, err error) {
	if !fetchmeta.IsLikelyUserNavigation(req) {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	writeErrorPage(w, "Something went wrong",
		"Zen hit an internal error while processing this page. Retrying might help.",
		err.Error())
}

// classifyUpstreamError maps an upstream request error to a headline and a
// plain-language explanation for the error page.
func classifyUpstreamError(err error) (title, message string) {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	// Timeouts are checked before DNS errors so that a slow resolver reports
	// "took too long" rather than "server not found": url.Error and net.DNSError
	// both surface timeouts through the net.Error interface.
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "This site took too long to respond",
			"The server didn't respond in time. It might be overloaded, or there may be a problem with your connection."
	case errors.As(err, &dnsErr):
		return "Server not found",
			"Zen couldn't find a server at this address. Check the address for typos, or try again in a moment."
	case errorsIsAny(err, connRefusedErrs):
		return "Connection refused",
			"The server refused the connection. The site might be down, or it may not accept connections on this port."
	case errorsIsAny(err, connResetErrs):
		return "Connection was reset",
			"The connection to the server was interrupted. This is usually temporary."
	case isTLSError(err):
		// writeUpstreamError closes the client connection on TLS errors, and
		// connectHandler puts the host on the transparent list, so a retry over a
		// fresh CONNECT reconnects without interception. "May" rather than "will":
		// the plain-HTTP path serves this message without a transparent-list entry.
		return "Secure connection failed",
			"Zen couldn't establish a secure connection to this site. Retrying may reconnect without Zen's filtering for this site."
	default:
		return "This site can't be reached",
			"Zen couldn't reach the server. Check your connection, or try again in a moment."
	}
}

// errorsIsAny reports whether errors.Is matches err against any of targets.
func errorsIsAny(err error, targets []error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func writeErrorPage(w http.ResponseWriter, title, message, detail string) {
	var buf bytes.Buffer
	err := errorPageTmpl.Execute(&buf, errorPageData{
		SharedStyle: pagestyle.Shared,
		Title:       title,
		Message:     message,
		Detail:      detail,
	})
	if err != nil {
		log.Printf("error executing error page template: %v", err)
		http.Error(w, detail, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadGateway)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("error writing error page: %v", err)
	}
}
