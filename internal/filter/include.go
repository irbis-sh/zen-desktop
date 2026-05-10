package filter

import (
	"fmt"
	"net/url"
	"strings"
)

func resolveInclude(base *url.URL, after string) (includeURL string, err error) {
	target := strings.TrimSpace(after)
	if target == "" {
		return "", fmt.Errorf("empty !#include")
	}
	absURL, isAbs, err := resolveURL(base, target)
	if err != nil {
		return "", fmt.Errorf("resolve %q with base %q: %w", target, base, err)
	}
	if isAbs && base != nil && !sameHost(base, absURL) {
		return "", fmt.Errorf("forbidden cross-origin include %q with base %q", absURL.String(), base.String())
	}
	return absURL.String(), nil
}

func resolveURL(base *url.URL, raw string) (parsedURL *url.URL, isAbs bool, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false, fmt.Errorf("invalid url/path %q: %w", raw, err)
	}

	if u.IsAbs() {
		return u, true, nil
	}

	if base == nil {
		return nil, false, fmt.Errorf("%q is relative but base url is empty", raw)
	}

	resolved := base.ResolveReference(u)
	return resolved, false, nil
}

func sameHost(a, b *url.URL) bool {
	return strings.EqualFold(a.Host, b.Host)
}
