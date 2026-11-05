package resolver

import (
	"sync"
	"testing"
)

func TestUseOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		host   string
		domain []string // from SetRouting
		vpn    bool
		want   bool
	}{
		{name: "public name", host: "news.example", want: false},
		{name: "public name under a VPN", host: "news.example", vpn: true, want: true},
		{name: "single label", host: "nas", want: true},
		{name: "localhost", host: "localhost", want: true},
		{name: "under localhost", host: "app.localhost", want: true},
		{name: "local", host: "printer.local", want: true},
		{name: "home.arpa", host: "nas.home.arpa", want: true},
		{name: "internal", host: "ip-10-0-0-1.ec2.internal", want: true},
		{name: "lan", host: "nas.lan", want: true},
		{name: "reserved word in the middle", host: "local.news.example", want: false},
		{name: "system domain itself", host: "corp.example", want: true},
		{name: "under a system domain", host: "wiki.corp.example", want: true},
		{name: "ends like a system domain", host: "notcorp.example", want: false},
		{name: "announced domain itself", host: "branch.example", domain: []string{"branch.example"}, want: true},
		{name: "under an announced domain", host: "git.branch.example", domain: []string{"branch.example"}, want: true},
		{name: "ends like an announced domain", host: "notbranch.example", domain: []string{"branch.example"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newResolver(&fakeDoH{}, []string{"corp.example"}, nil)
			r.SetRouting(tt.domain, tt.vpn)

			if got := r.useOS(tt.host); got != tt.want {
				t.Errorf("useOS(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestSetRoutingFiltersDomains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		announced string
		probe     string
		want      bool
	}{
		{announced: "com", probe: "news.com", want: false},
		{announced: "co.uk", probe: "news.co.uk", want: false},
		{announced: "dev", probe: "news.dev", want: false},
		{announced: "github.io", probe: "news.github.io", want: false},
		{announced: "corp", probe: "wiki.corp", want: true},
		{announced: "home", probe: "nas.home", want: true},
		{announced: "lan", probe: "nas.lan", want: true},
		{announced: "example-tailnet.ts.net", probe: "nas.example-tailnet.ts.net", want: true},
		// An empty domain, such as the root once its dot is trimmed, must match nothing.
		{announced: "", probe: "news.example", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.announced, func(t *testing.T) {
			t.Parallel()

			r := newResolver(&fakeDoH{}, nil, nil)
			r.SetRouting([]string{tt.announced}, false)

			if got := r.useOS(tt.probe); got != tt.want {
				t.Errorf("after announcing %q, useOS(%q) = %v, want %v", tt.announced, tt.probe, got, tt.want)
			}
		})
	}
	t.Run("empty is not stored", func(t *testing.T) {
		t.Parallel()

		r := newResolver(&fakeDoH{}, nil, nil)
		r.SetRouting([]string{""}, false)

		if n := len(r.routing.Load().domains); n != 0 {
			t.Errorf("stored %d domains, want 0", n)
		}
	})
}

func TestSystemDomainsAreNotFiltered(t *testing.T) {
	t.Parallel()

	r := newResolver(&fakeDoH{}, []string{"dev"}, nil)

	if !r.useOS("internal-tool.dev") {
		t.Error("a user's own dev domain doesn't go to the OS")
	}
}

func TestSetRoutingReplacesEarlierState(t *testing.T) {
	t.Parallel()

	r := newResolver(&fakeDoH{}, nil, nil)
	r.SetRouting([]string{"branch.example"}, true)
	r.SetRouting(nil, false)

	if r.useOS("git.branch.example") {
		t.Error("earlier routing state still applies")
	}
}

func TestSetRoutingDuringLookups(t *testing.T) {
	t.Parallel()

	r := newResolver(&fakeDoH{}, nil, nil)
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			r.SetRouting([]string{"branch.example"}, false)
			r.SetRouting(nil, true)
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 1000 {
				r.useOS("git.branch.example")
			}
		})
	}
	wg.Wait()
}
