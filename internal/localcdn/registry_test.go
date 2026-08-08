package localcdn

import (
	"net/url"
	"strings"
	"testing"
)

const registryTestManifest = `{
  "version": 1,
  "libraries": [
    {
      "key": "jquery",
      "name": "jQuery",
      "license": "MIT",
      "enabledByDefault": true,
      "resources": [
        {
          "id": "jquery-3.7.1",
          "library": "jquery",
          "version": "3.7.1",
          "versionRange": ">=3.0.0 <4.0.0",
          "patterns": [
            "https://ajax.googleapis.com/ajax/libs/jquery/{version}/jquery.min.js",
            "https://ajax.googleapis.com/ajax/libs/jquery/{version}/jquery.js",
            "https://code.jquery.com/jquery-{version}.min.js",
            "https://cdn.jsdelivr.net/npm/jquery@*/dist/jquery.min.js"
          ],
          "file": "resources/jquery/3.7.1/jquery.min.js",
          "contentType": "application/javascript; charset=utf-8"
        },
        {
          "id": "jquery-2.2.4",
          "library": "jquery",
          "version": "2.2.4",
          "versionRange": ">=2.0.0 <3.0.0",
          "patterns": [
            "https://ajax.googleapis.com/ajax/libs/jquery/{version}/jquery.min.js"
          ],
          "file": "resources/jquery/2.2.4/jquery.min.js",
          "contentType": "application/javascript; charset=utf-8"
        }
      ]
    },
    {
      "key": "icons",
      "name": "Icons",
      "license": "Apache-2.0",
      "enabledByDefault": true,
      "resources": [
        {
          "id": "icons-css",
          "library": "icons",
          "version": "1.0.0",
          "patterns": [
            "https://fonts.googleapis.com/css?family=Material+Icons"
          ],
          "file": "resources/google-material-icons/v145/material-icons.css",
          "contentType": "text/css; charset=utf-8"
        },
        {
          "id": "wildcard-host",
          "library": "icons",
          "version": "1.0.0",
          "patterns": [
            "https://*.example.com/assets/*/app.js"
          ],
          "file": "resources/google-material-icons/v145/material-icons.css",
          "contentType": "text/css; charset=utf-8"
        }
      ]
    }
  ],
  "cdnHosts": [
    "cdn.example.net"
  ]
}`

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry([]byte(registryTestManifest), nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func TestRegistryMatch(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t)

	tests := []struct {
		name    string
		rawURL  string
		wantID  string
		wantHit bool
	}{
		{
			name:    "exact googleapis jquery",
			rawURL:  "https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js",
			wantID:  "jquery-3.7.1",
			wantHit: true,
		},
		{
			name:    "non-min variant",
			rawURL:  "https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.js",
			wantID:  "jquery-3.7.1",
			wantHit: true,
		},
		{
			name:    "dashed version pattern",
			rawURL:  "https://code.jquery.com/jquery-3.7.1.min.js",
			wantID:  "jquery-3.7.1",
			wantHit: true,
		},
		{
			name:    "wildcard version segment",
			rawURL:  "https://cdn.jsdelivr.net/npm/jquery@3.7.1/dist/jquery.min.js",
			wantID:  "jquery-3.7.1",
			wantHit: true,
		},
		{
			name:    "version range picks major 2",
			rawURL:  "https://ajax.googleapis.com/ajax/libs/jquery/2.2.4/jquery.min.js",
			wantID:  "jquery-2.2.4",
			wantHit: true,
		},
		{
			name:    "out of range version",
			rawURL:  "https://ajax.googleapis.com/ajax/libs/jquery/4.0.0/jquery.min.js",
			wantHit: false,
		},
		{
			name:    "unknown host",
			rawURL:  "https://example.com/ajax/libs/jquery/3.7.1/jquery.min.js",
			wantHit: false,
		},
		{
			name:    "unknown path",
			rawURL:  "https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery-ui.min.js",
			wantHit: false,
		},
		{
			name:    "query matches",
			rawURL:  "https://fonts.googleapis.com/css?family=Material+Icons",
			wantID:  "icons-css",
			wantHit: true,
		},
		{
			name:    "query mismatch",
			rawURL:  "https://fonts.googleapis.com/css?family=Roboto",
			wantHit: false,
		},
		{
			name:    "missing query",
			rawURL:  "https://fonts.googleapis.com/css",
			wantHit: false,
		},
		{
			name:    "wildcard host",
			rawURL:  "https://cdn.example.com/assets/v2/app.js",
			wantID:  "wildcard-host",
			wantHit: true,
		},
		{
			name:    "wildcard host without subdomain",
			rawURL:  "https://example.com/assets/v2/app.js",
			wantHit: false,
		},
		{
			name:    "host case insensitive",
			rawURL:  "https://AJAX.GOOGLEAPIS.COM/ajax/libs/jquery/3.7.1/jquery.min.js",
			wantID:  "jquery-3.7.1",
			wantHit: true,
		},
		{
			name:    "query ignored for plain resource",
			rawURL:  "https://ajax.googleapis.com/ajax/libs/jquery/3.7.1/jquery.min.js?v=1",
			wantID:  "jquery-3.7.1",
			wantHit: true,
		},
		{
			name:    "greedy wildcard does not match shorter path",
			rawURL:  "https://cdn.jsdelivr.net/npm/jquery@3.7.1",
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			got, _ := registry.match(u.Hostname(), u.Path, u.Query())
			if tt.wantHit != (got != nil) {
				t.Fatalf("match(%s) hit=%v, want %v", tt.rawURL, got != nil, tt.wantHit)
			}
			if tt.wantHit && got.mapping.ID != tt.wantID {
				t.Fatalf("matched %q, want %q", got.mapping.ID, tt.wantID)
			}
		})
	}
}

func TestRegistryKnownCDNHost(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t)

	tests := []struct {
		host string
		want bool
	}{
		{host: "cdn.example.net", want: true},
		{host: "ajax.googleapis.com", want: true},
		{host: "cdn.example.com", want: true},
		{host: "foo.cdn.example.net", want: false},
		{host: "unknown.example.org", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Parallel()
			if got := registry.isKnownCDNHost(tt.host); got != tt.want {
				t.Fatalf("isKnownCDNHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestRegistryCDNHostsIncludesBundledHosts(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t)
	hosts := registry.CDNHosts()
	joined := strings.Join(hosts, "\n")
	for _, want := range []string{"cdn.example.net", "ajax.googleapis.com", "fonts.googleapis.com"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("CDNHosts() missing %q: %v", want, hosts)
		}
	}
	if strings.Contains(joined, "example.com") {
		t.Fatalf("CDNHosts() must only contain exact hosts, got %v", hosts)
	}
}

func TestIsKnownCDNHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		want bool
	}{
		{host: "cdnjs.cloudflare.com", want: true},
		{host: "cdn.jsdelivr.net", want: true},
		{host: "challenges.cloudflare.com", want: false},
		{host: "example.com", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Parallel()
			if got := IsKnownCDNHost(tt.host); got != tt.want {
				t.Fatalf("IsKnownCDNHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestRegistryRejectsInvalidManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "invalid json", data: "not json"},
		{name: "empty patterns", data: `{"version":1,"libraries":[{"key":"a","name":"A","resources":[{"id":"1","library":"a","version":"1.0.0","patterns":[],"file":"x.js","contentType":"text/javascript"}]}]}`},
		{name: "empty file", data: `{"version":1,"libraries":[{"key":"a","name":"A","resources":[{"id":"1","library":"a","version":"1.0.0","patterns":["https://cdn.example.com/a.js"],"file":"","contentType":"text/javascript"}]}]}`},
		{name: "invalid version range", data: `{"version":1,"libraries":[{"key":"a","name":"A","resources":[{"id":"1","library":"a","version":"1.0.0","versionRange":"not-a-range","patterns":["https://cdn.example.com/a.js"],"file":"x.js","contentType":"text/javascript"}]}]}`},
		{name: "pattern without host", data: `{"version":1,"libraries":[{"key":"a","name":"A","resources":[{"id":"1","library":"a","version":"1.0.0","patterns":["/a.js"],"file":"x.js","contentType":"text/javascript"}]}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewRegistry([]byte(tt.data), nil); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestCleanRelativePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "resources/jquery/3.7.1/jquery.min.js", want: "resources/jquery/3.7.1/jquery.min.js"},
		{in: "custom.js", want: "custom.js"},
		{in: "sub/dir/file.css", want: "sub/dir/file.css"},
		{in: "../evil.js", wantErr: true},
		{in: "a/../../evil.js", wantErr: true},
		{in: "/absolute/path.js", wantErr: true},
		{in: "C:\\windows\\path.js", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := cleanRelativePath(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("cleanRelativePath(%q) expected error, got %q", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("cleanRelativePath(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("cleanRelativePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
