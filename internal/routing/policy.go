// Package routing provides [Policy] which decides whether traffic from an app should be routed
// through Zen.
package routing

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/config"
)

const (
	safariBundlePath = "/System/Volumes/Preboot/Cryptexes/App/System/Applications/Safari.app"

	// safariWebKitNetworkingPath is the path of a binary that makes requests on Safari.app's behalf on macOS.
	safariWebKitNetworkingPath = "/System/Volumes/Preboot/Cryptexes/Incoming/OS/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.Networking.xpc/Contents/MacOS/com.apple.WebKit.Networking"
)

// Policy decides whether requests from a process should be proxied by Zen.
//
// Policy is immutable after construction and safe to use concurrently as a
// callback.
type Policy struct {
	mode       config.RoutingMode
	exactPaths map[string]struct{}
	pathRoots  []string
}

// NewPolicy creates a Policy from app routing config.
func NewPolicy(routing config.RoutingConfig) *Policy {
	p := &Policy{
		mode:       routing.Mode,
		exactPaths: map[string]struct{}{},
		pathRoots:  []string{},
	}

	for _, appPath := range routing.AppPaths {
		p.addPath(appPath)
	}

	return p
}

// ShouldProxy reports whether traffic from processPath should be routed through
// Zen's filtering proxy.
//
// An empty process path is treated as unmatched. That means blocklist mode
// proxies it and allowlist mode bypasses it.
func (p *Policy) ShouldProxy(processPath string) bool {
	path := normalisePath(processPath)
	if path == "" {
		return p.shouldProxyMatched(false)
	}

	return p.shouldProxyMatched(p.matches(path))
}

func (p *Policy) shouldProxyMatched(matched bool) bool {
	if p.mode == config.RoutingModeAllowlist {
		return matched
	}
	return !matched
}

func (p *Policy) addPath(path string) {
	path = normalisePath(path)
	if path == "" {
		return
	}

	if runtime.GOOS == "darwin" && strings.HasSuffix(path, ".app") {
		p.pathRoots = append(p.pathRoots, path)
		if path == normalisePath(safariBundlePath) {
			p.exactPaths[normalisePath(safariWebKitNetworkingPath)] = struct{}{}
		}
	} else {
		p.exactPaths[path] = struct{}{}
	}
}

func (p *Policy) matches(path string) bool {
	if _, ok := p.exactPaths[path]; ok {
		return true
	}

	for _, root := range p.pathRoots {
		if strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

func normalisePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else {
		// EvalSymlinks calls Clean on the result, so we only run it manually if it failed.
		path = filepath.Clean(path)
	}

	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		// Filepaths are case-insensitive on Windows and macOS.
		path = strings.ToLower(path)
	}
	return path
}
