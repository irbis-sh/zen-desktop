package resolver

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// CNAMEs looks names up through a real net.Resolver, so these tests stay outside synctest
// bubbles (see the comment in serve_test.go). They move cached answers into the past with
// expire instead.

func TestCNAMEsSkipsNamesTheOSAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		host  string
		setup func(r *Resolver)
	}{
		{name: "IPv4 address", host: "192.0.2.1"},
		{name: "IPv6 address", host: "2001:db8::1"},
		{name: "name without a dot", host: "nas"},
		{name: "name under a network domain", host: "wiki.corp.example", setup: func(r *Resolver) { r.SetRouting([]string{"corp.example"}, false) }},
		{name: "fallback mode", host: testHost, setup: func(r *Resolver) { r.dohOff.Store(true) }},
		{name: "VPN handles all DNS", host: testHost, setup: func(r *Resolver) { r.SetRouting(nil, true) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeDoH{}
			r := newTestResolver(client, nil, nil)
			if tt.setup != nil {
				tt.setup(r)
			}

			if got := r.CNAMEs(t.Context(), tt.host); got != nil {
				t.Errorf("CNAMEs(%q) = %v, want nil", tt.host, got)
			}
			if client.count() != 0 {
				t.Errorf("client got %d queries, want 0", client.count())
			}
		})
	}
}

func TestCNAMEsLooksUpNewNameOnce(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: chainTo("a.cdn.example.", "b.cdn.example.")}
	r := newTestResolver(client, nil, nil)
	want := []string{"a.cdn.example", "b.cdn.example"}

	if got := r.CNAMEs(t.Context(), testHost); !slices.Equal(got, want) {
		t.Errorf("CNAMEs = %v, want %v", got, want)
	}
	if got := r.CNAMEs(t.Context(), "NEWS.Example"); !slices.Equal(got, want) {
		t.Errorf("CNAMEs with upper case and no trailing dot = %v, want %v", got, want)
	}
	// The dial after a check finds the answers in the cache.
	if _, err := r.NetResolver().LookupNetIP(t.Context(), "ip", testHost); err != nil {
		t.Fatal(err)
	}

	if client.count() != 2 {
		t.Errorf("client got %d queries, want 2: one per type", client.count())
	}
}

func TestCNAMEsUsesExpiredChainUntilMaxChainAge(t *testing.T) {
	t.Parallel()

	var moved atomic.Bool
	client := &fakeDoH{answer: func(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
		if moved.Load() {
			return chainTo("new.example.")(ctx, q)
		}
		return chainTo("old.example.")(ctx, q)
	}}
	r := newTestResolver(client, nil, nil)
	r.CNAMEs(t.Context(), testHost)
	moved.Store(true)

	expire(r, maxChainAge-time.Minute)
	if got, want := r.CNAMEs(t.Context(), testHost), []string{"old.example"}; !slices.Equal(got, want) {
		t.Errorf("CNAMEs before maxChainAge = %v, want %v", got, want)
	}
	if client.count() != 2 {
		t.Errorf("client got %d queries before maxChainAge, want 2", client.count())
	}

	expire(r, maxChainAge)
	if got, want := r.CNAMEs(t.Context(), testHost), []string{"new.example"}; !slices.Equal(got, want) {
		t.Errorf("CNAMEs after maxChainAge = %v, want %v", got, want)
	}
	if client.count() != 4 {
		t.Errorf("client got %d queries after maxChainAge, want 4", client.count())
	}
}

func TestCNAMEsUsesAAAAChainWithoutA(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{}
	r := newTestResolver(client, nil, nil)
	r.cache.put(cacheKey{name: "news.example", qtype: dnsmessage.TypeAAAA}, verdict{ttl: time.Minute, chain: []string{"v6.cdn.example"}})

	if got, want := r.CNAMEs(t.Context(), testHost), []string{"v6.cdn.example"}; !slices.Equal(got, want) {
		t.Errorf("CNAMEs = %v, want %v", got, want)
	}
	if client.count() != 0 {
		t.Errorf("client got %d queries, want 0", client.count())
	}
}

func TestCNAMEsIgnoresAnswerWithLocalAddress(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: always(answer(cname(testHost, 60, "cdn.example."), a("cdn.example.", 60, "10.0.0.1")))}
	os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.1")}}
	r := newTestResolver(client, nil, os.lookup)

	if got := r.CNAMEs(t.Context(), testHost); got != nil {
		t.Errorf("CNAMEs = %v, want nil", got)
	}
}

func TestCNAMEsAfterFailedQuery(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: failWith(errors.New("timeout"))}
	os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("192.0.2.1")}}
	r := newTestResolver(client, nil, os.lookup)

	for range 2 {
		if got := r.CNAMEs(t.Context(), testHost); got != nil {
			t.Errorf("CNAMEs = %v, want nil", got)
		}
	}
	if client.count() != 2 {
		t.Errorf("client got %d queries from two checks, want 2", client.count())
	}

	// A dial still asks the DoH server. Four failures in all stay below failureLimit.
	if _, err := r.NetResolver().LookupNetIP(t.Context(), "ip", testHost); err != nil {
		t.Fatal(err)
	}
	if client.count() != 4 {
		t.Errorf("client got %d queries after a dial, want 4", client.count())
	}
}

func TestCNAMEsAfterFailedRefresh(t *testing.T) {
	t.Parallel()

	var failing atomic.Bool
	client := &fakeDoH{answer: func(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
		if failing.Load() {
			return nil, errors.New("timeout")
		}
		return chainTo("cdn.example.")(ctx, q)
	}}
	os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("192.0.2.1")}}
	r := newTestResolver(client, nil, os.lookup)
	r.CNAMEs(t.Context(), testHost)

	// The answer expires, and the dial that refreshes it is answered by the OS.
	expire(r, time.Second)
	failing.Store(true)
	if _, err := r.NetResolver().LookupNetIP(t.Context(), "ip", testHost); err != nil {
		t.Fatal(err)
	}

	if got := r.CNAMEs(t.Context(), testHost); got != nil {
		t.Errorf("CNAMEs = %v, want nil: the connection was dialled from the OS's answer", got)
	}
}

func TestConcurrentCNAMEsShareLookup(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: func(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
		// Long enough for the other calls to start while the first lookup waits.
		time.Sleep(50 * time.Millisecond)
		return chainTo("cdn.example.")(ctx, q)
	}}
	r := newTestResolver(client, nil, nil)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if got, want := r.CNAMEs(t.Context(), testHost), []string{"cdn.example"}; !slices.Equal(got, want) {
				t.Errorf("CNAMEs = %v, want %v", got, want)
			}
		})
	}
	wg.Wait()

	if client.count() != 2 {
		t.Errorf("client got %d queries, want 2: one per type", client.count())
	}
}

// chainTo returns a fakeDoH answer function that gives testHost a CNAME chain through
// names, in order, with an address of the question's type on the last one.
func chainTo(names ...string) answerFunc {
	return func(_ context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
		var records []dnsmessage.Resource
		from := testHost
		for _, n := range names {
			records = append(records, cname(from, 60, n))
			from = n
		}
		if q.Type == dnsmessage.TypeAAAA {
			records = append(records, aaaa(from, 300, "2001:db8::1"))
		} else {
			records = append(records, a(from, 300, "192.0.2.1"))
		}
		return answer(records...), nil
	}
}

// expire makes testHost's cached answers d past their expiry.
func expire(r *Resolver, d time.Duration) {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	for _, qtype := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
		if el, ok := r.cache.entries[cacheKey{name: "news.example", qtype: qtype}]; ok {
			e := el.Value.(*cacheEntry)
			e.expires = wallClock().Add(-d)
		}
	}
}
