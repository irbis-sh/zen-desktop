package sysproxy

import (
	"strings"
	"testing"
)

func TestRenderPac(t *testing.T) {
	t.Parallel()

	pac := string(renderPac(12345, []string{"example.com", "gov.uk"}, []string{"cdnjs.cloudflare.com"}))

	if !strings.Contains(pac, `return "PROXY 127.0.0.1:12345; DIRECT";`) {
		t.Fatalf("PAC does not contain the PROXY fallback directive:\n%s", pac)
	}
	for _, host := range []string{"example.com", "gov.uk"} {
		if !strings.Contains(pac, `"`+host+`"`) {
			t.Fatalf("PAC does not contain excluded host %q:\n%s", host, pac)
		}
	}
	if pac == string(transparentPAC) {
		t.Fatal("PAC collapsed to an unconditional DIRECT")
	}
	if !strings.Contains(pac, "PROXY") {
		t.Fatalf("PAC does not route through the proxy at all:\n%s", pac)
	}
	if !strings.Contains(pac, `"cdnjs.cloudflare.com"`) {
		t.Fatalf("PAC does not include the always-proxy CDN host:\n%s", pac)
	}
	// User exclusions must be checked first, then CDN hosts, then the built-in
	// exclusions (so an explicit user whitelist wins over the CDN override,
	// while a built-in parent exclusion cannot bypass CDN interception).
	userIndex := strings.Index(pac, "userExcludedHosts")
	cdnIndex := strings.Index(pac, "cdnHosts")
	excludedIndex := strings.Index(pac, "excludedHosts")
	if userIndex == -1 || cdnIndex == -1 || excludedIndex == -1 || userIndex >= cdnIndex || cdnIndex >= excludedIndex {
		t.Fatalf("PAC must check user exclusions, then CDN hosts, then built-in exclusions:\n%s", pac)
	}
}

func TestBuildExcludedHosts(t *testing.T) {
	t.Parallel()

	hosts := buildExcludedHosts([]string{"example.com", "sub.example.org"})
	joined := strings.Join(hosts, "\n")
	for _, host := range []string{"example.com", "sub.example.org"} {
		if !strings.Contains(joined, host) {
			t.Fatalf("excluded hosts do not contain %q: %q", host, joined)
		}
	}
	if len(hosts) < 3 {
		t.Fatalf("expected built-in exclusion lists to be merged, got %d hosts", len(hosts))
	}
}

func TestIsExcludedHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		user []string
		want bool
	}{
		// cdnjs.cloudflare.com matches the broad cloudflare.com exclusion; the
		// localcdn engine and the PAC both override this for bundled CDN hosts
		// (see TestEngineIgnoresExclusionForKnownCDNHosts and TestRenderPac).
		{name: "cdnjs matches parent exclusion", host: "cdnjs.cloudflare.com", want: true},
		{name: "jsdelivr is not excluded", host: "cdn.jsdelivr.net", want: false},
		{name: "user host excluded", host: "example.com", user: []string{"example.com"}, want: true},
		{name: "subdomain of user host excluded", host: "sub.example.com", user: []string{"example.com"}, want: true},
		{name: "suffix collision not excluded", host: "notexample.com", user: []string{"example.com"}, want: false},
		{name: "built-in exclusion applies", host: "proton.me", want: true},
		{name: "case insensitive", host: "EXAMPLE.COM", user: []string{"example.com"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsExcludedHost(tt.host, tt.user); got != tt.want {
				t.Fatalf("IsExcludedHost(%q, %v) = %v, want %v", tt.host, tt.user, got, tt.want)
			}
		})
	}
}

func TestIsUserExcludedHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		user []string
		want bool
	}{
		{name: "no user hosts", host: "cdnjs.cloudflare.com", user: nil, want: false},
		{name: "exact user host", host: "cdnjs.cloudflare.com", user: []string{"cdnjs.cloudflare.com"}, want: true},
		{name: "user subdomain", host: "cdnjs.cloudflare.com", user: []string{"cloudflare.com"}, want: true},
		{name: "built-in exclusion is not user exclusion", host: "proton.me", user: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUserExcludedHost(tt.host, tt.user); got != tt.want {
				t.Fatalf("IsUserExcludedHost(%q, %v) = %v, want %v", tt.host, tt.user, got, tt.want)
			}
		})
	}
}
