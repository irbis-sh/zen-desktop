package rulemodifiers

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Request is what rules are matched against. Every rule checks the same request,
// so values that cost something to derive are worked out once, not per rule.
type Request struct {
	*http.Request
	// sourceHost is the host of the page that made the request: the Referer's
	// host, or the request's own host when there is no Referer.
	sourceHost string
	// sourceHostOK is false when the Referer can't be parsed. No $domain rule
	// matches then, inverted ones included. An empty sourceHost can't signal this:
	// "example.com/page" parses to an empty host, and domain=~x does match it.
	sourceHostOK bool
	// sourceLabel is the label before sourceHost's public suffix, such as
	// "example" for www.example.co.uk. It is empty if there is no such label.
	sourceLabel string
	// Query is the request's parsed query, or nil if it has none. Rules are
	// matched against it before any of them runs; ModifyReq then removes
	// parameters from it.
	Query url.Values
}

// NewRequest derives the values rules are matched against from req. Call it
// before any rule changes req, and don't change req while the result is in use.
func NewRequest(req *http.Request) Request {
	host, ok := parseSourceHost(req)
	var query url.Values
	if req.URL.RawQuery != "" {
		query = req.URL.Query()
	}
	return Request{Request: req, sourceHost: host, sourceHostOK: ok, sourceLabel: labelBeforeSuffix(host), Query: query}
}

func parseSourceHost(req *http.Request) (host string, ok bool) {
	referer := req.Header.Get("Referer")
	// Without this fallback, domain=~example.com would apply to navigations to
	// example.com itself.
	if referer == "" {
		return req.URL.Hostname(), true
	}
	u, err := url.Parse(referer)
	if err != nil {
		return "", false
	}
	return u.Hostname(), true
}

func labelBeforeSuffix(host string) string {
	eTLD1, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return ""
	}
	label, _, _ := strings.Cut(eTLD1, ".")
	return label
}
