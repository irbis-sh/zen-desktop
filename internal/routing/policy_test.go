package routing

import (
	"runtime"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/config"
)

func TestPolicyBlocklist(t *testing.T) {
	policy := NewPolicy(config.RoutingConfig{
		Mode:     config.RoutingModeBlocklist,
		AppPaths: []string{"/usr/bin/browser"},
	})

	if policy.ShouldProxy("/usr/bin/browser") {
		t.Fatal("selected app should be transparent in blocklist mode")
	}
	if !policy.ShouldProxy("/usr/bin/other") {
		t.Fatal("unselected app should be proxied in blocklist mode")
	}
}

func TestPolicyAllowlist(t *testing.T) {
	policy := NewPolicy(config.RoutingConfig{
		Mode:     config.RoutingModeAllowlist,
		AppPaths: []string{"/usr/bin/browser"},
	})

	if !policy.ShouldProxy("/usr/bin/browser") {
		t.Fatal("selected app should be proxied in allowlist mode")
	}
	if policy.ShouldProxy("/usr/bin/other") {
		t.Fatal("unselected app should be transparent in allowlist mode")
	}
}

func TestDarwinAppBundleMatchesNestedExecutables(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	policy := NewPolicy(config.RoutingConfig{
		Mode:     config.RoutingModeAllowlist,
		AppPaths: []string{"/Applications/Foo.app"},
	})

	if !policy.ShouldProxy("/Applications/Foo.app/Contents/MacOS/Foo") {
		t.Fatal(".app bundle should match nested executables")
	}
	if policy.ShouldProxy("/Applications/Foobar.app/Contents/MacOS/Foobar") {
		t.Fatal(".app bundle should not match path prefixes outside the bundle")
	}
}

func TestDarwinSafariAssociatedWebKitProcess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	policy := NewPolicy(config.RoutingConfig{
		Mode:     config.RoutingModeBlocklist,
		AppPaths: []string{"/System/Volumes/Preboot/Cryptexes/App/System/Applications/Safari.app"},
	})

	webkitPath := "/System/Volumes/Preboot/Cryptexes/Incoming/OS/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.Networking.xpc/Contents/MacOS/com.apple.WebKit.Networking"
	if policy.ShouldProxy(webkitPath) {
		t.Fatal("Safari selection should match the WebKit networking XPC process")
	}
}
