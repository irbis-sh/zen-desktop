package resolver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// The tests here run inside synctest bubbles, like those in serve_test.go, so the loop's
// waits and the probe's time limit come out exact.

var (
	active         = Status{Mode: StatusActive}
	noIPv4         = Status{Mode: StatusNoIPv4}
	vpn            = Status{Mode: StatusVPN}
	errRefused     = errors.New("connection refused")
	failingRefused = Status{Mode: StatusFailing, Error: "connection refused"}
	usable         = always(rootServer())
)

func TestFailureKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		lookup answerFunc
	}{
		{name: "client error", lookup: failWith(errRefused)},
		{name: "SERVFAIL", lookup: always(rcode(dnsmessage.RCodeServerFailure))},
		{name: "REFUSED", lookup: always(rcode(dnsmessage.RCodeRefused))},
		{name: "DNSSEC failure", lookup: always(withEDE(rcode(dnsmessage.RCodeServerFailure), 7))},
		{name: "DoH's share runs out", lookup: hang},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := newFallbackRig(t, tt.lookup, usable, true)

				g.lookupNames(t, 4)
				synctest.Wait()
				if n := len(g.server.probeTimes()); n != 0 {
					t.Fatalf("%d probes after 4 failures, want none", n)
				}
				g.lookupNames(t, 1)
				synctest.Wait()
				if n := len(g.server.probeTimes()); n != 1 {
					t.Errorf("%d probes after 5 failures, want 1", n)
				}
			})
		})
	}
}

func TestUsableAnswersResetCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *dnsmessage.Message
	}{
		{name: "addresses", msg: answer(a("good.example.", 60, "192.0.2.1"))},
		{name: "NXDOMAIN", msg: nxdomain()},
		{name: "block", msg: withEDE(nxdomain(), 15)},
		{name: "local address", msg: answer(a("good.example.", 60, "10.0.0.1"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				lookup := func(_ context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
					if q.Name.String() == "good.example." {
						return tt.msg, nil
					}
					return nil, errRefused
				}
				g := newFallbackRig(t, lookup, usable, true)

				g.lookupNames(t, 4)
				if _, err := ask(t.Context(), g.r, question("good.example.", dnsmessage.TypeA)); err != nil {
					t.Fatal(err)
				}
				g.lookupNames(t, 4)
				synctest.Wait()

				if n := len(g.server.probeTimes()); n != 0 {
					t.Errorf("%d probes, want none: the usable answer should have reset the count", n)
				}
			})
		})
	}
}

func TestCancelledLookupNeitherCountsNorResets(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		lookup := func(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
			if q.Name.String() == "slow.example." {
				return hang(ctx, q)
			}
			return nil, errRefused
		}
		g := newFallbackRig(t, lookup, usable, true)

		g.lookupNames(t, 4)
		// The caller gives up after 1 s, before DoH's share of 3 s runs out.
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)
		_, _ = ask(ctx, g.r, question("slow.example.", dnsmessage.TypeA))
		synctest.Wait()
		if n := len(g.server.probeTimes()); n != 0 {
			t.Fatalf("%d probes, want none: the cancelled lookup counted", n)
		}

		g.lookupNames(t, 1)
		synctest.Wait()
		if n := len(g.server.probeTimes()); n != 1 {
			t.Errorf("%d probes, want 1: the cancelled lookup reset the count", n)
		}
	})
}

func TestRetriesOfServfailCountOncePerType(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, always(rcode(dnsmessage.RCodeServerFailure)), usable, true)

		// Go resends a SERVFAIL, and the cache answers the resends.
		for _, qtype := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
			for range 8 {
				if _, err := ask(t.Context(), g.r, question(testHost, qtype)); err != nil {
					t.Fatal(err)
				}
			}
		}
		g.lookupNames(t, 2)
		synctest.Wait()
		if n := len(g.server.probeTimes()); n != 0 {
			t.Fatalf("%d probes after 4 counted failures, want none", n)
		}
		g.lookupNames(t, 1)
		synctest.Wait()
		if n := len(g.server.probeTimes()); n != 1 {
			t.Errorf("%d probes after 5 counted failures, want 1", n)
		}
	})
}

func TestUsableProbeKeepsDoHOn(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), usable, true)

		g.lookupNames(t, 5)
		synctest.Wait()
		// The usable probe reset the count, so these count as the first four.
		g.lookupNames(t, 4)
		synctest.Wait()
		if n := len(g.server.probeTimes()); n != 1 {
			t.Fatalf("%d probes after 9 failures, want 1", n)
		}
		g.lookupNames(t, 1)
		synctest.Wait()

		if n := len(g.server.probeTimes()); n != 2 {
			t.Errorf("%d probes after 10 failures, want 2", n)
		}
		if got := g.statuses.get(); !slices.Equal(got, []Status{active}) {
			t.Errorf("statuses = %v, want only active", got)
		}
		if g.lookups() != 10 {
			t.Errorf("client got %d lookups, want 10", g.lookups())
		}
	})
}

func TestOneLoopWhileFailuresArrive(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), hang, true)

		// The probe hangs, so DoH stays on and failures 6 to 12 arrive while the loop runs.
		g.lookupNames(t, 12)
		synctest.Wait()

		if n := len(g.server.probeTimes()); n != 1 {
			t.Errorf("%d probes, want 1", n)
		}
	})
}

func TestProbeOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		probe      answerFunc
		want       []Status
		wantResets int
	}{
		{name: "root server's address", probe: usable, want: []Status{active}},
		{name: "NXDOMAIN", probe: always(nxdomain()), want: []Status{active}},
		{
			name:  "SERVFAIL",
			probe: always(rcode(dnsmessage.RCodeServerFailure)),
			want:  []Status{active, {Mode: StatusFailing, Error: "server answered SERVFAIL"}},
		},
		{name: "client error", probe: failWith(errRefused), want: []Status{active, failingRefused}, wantResets: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := newFallbackRig(t, failWith(errRefused), tt.probe, true)

				g.lookupNames(t, 5)
				synctest.Wait()

				if got := g.statuses.get(); !slices.Equal(got, tt.want) {
					t.Errorf("statuses = %v, want %v", got, tt.want)
				}
				if got := g.client.resetCount(); got != tt.wantResets {
					t.Errorf("client reset %d times, want %d", got, tt.wantResets)
				}
			})
		})
	}
}

func TestFailingSendsEveryNameToOS(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		lookup := func(_ context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
			if q.Name.String() == "cached.example." {
				return answer(a("cached.example.", 300, "192.0.2.1")), nil
			}
			return nil, errRefused
		}
		g := newFallbackRig(t, lookup, failWith(errRefused), true)
		cached := question("cached.example.", dnsmessage.TypeA)
		if _, err := ask(t.Context(), g.r, cached); err != nil {
			t.Fatal(err)
		}

		g.lookupNames(t, 5)
		synctest.Wait()
		before := g.lookups()
		got, err := ask(t.Context(), g.r, cached)
		if err != nil {
			t.Fatal(err)
		}
		g.lookupNames(t, 3)

		if n := g.lookups() - before; n != 0 {
			t.Errorf("client got %d lookups while failing, want none", n)
		}
		if addrs := addrsOf(t, got); !slices.Equal(addrs, g.os.addrs) {
			t.Errorf("cached name answered with %v, want the OS's %v", addrs, g.os.addrs)
		}
	})
}

func TestWaitingLookupsFinishWhenStateTurnsFailing(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		lookup := func(_ context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
			if q.Name.String() == "slow.example." {
				time.Sleep(time.Second)
				return answer(a("slow.example.", 60, "192.0.2.1")), nil
			}
			return nil, errRefused
		}
		g := newFallbackRig(t, lookup, failWith(errRefused), true)

		var got *dnsmessage.Message
		var err error
		done := make(chan struct{})
		go func() {
			defer close(done)
			got, err = ask(t.Context(), g.r, question("slow.example.", dnsmessage.TypeA))
		}()
		synctest.Wait()
		g.lookupNames(t, 5)
		synctest.Wait()
		if s := g.statuses.get(); !slices.Equal(s, []Status{active, failingRefused}) {
			t.Fatalf("statuses = %v, want failing before the slow answer", s)
		}
		<-done

		if err != nil {
			t.Fatal(err)
		}
		if addrs := addrsOf(t, got); !slices.Equal(addrs, []netip.Addr{netip.MustParseAddr("192.0.2.1")}) {
			t.Errorf("slow lookup got %v, want the DoH answer", addrs)
		}
	})
}

func TestProbeSchedule(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), failWith(errRefused), true)
		// This kick comes while no loop runs. The first round has to take it, or a second
		// round would follow at once.
		g.r.NetworkChanged()

		g.lookupNames(t, 5)
		time.Sleep(200 * time.Second)
		synctest.Wait()

		want := []time.Duration{0, 1, 3, 7, 15, 31, 63, 123, 183}
		for i := range want {
			want[i] *= time.Second
		}
		if got := g.server.probeTimes(); !slices.Equal(got, want) {
			t.Errorf("probes at %v, want %v", got, want)
		}
	})
}

func TestSlowProbeWhileDoHIsOff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Each probe takes 5 s, as on a link where connecting is slow.
		slow := func(ctx context.Context, _ dnsmessage.Question) (*dnsmessage.Message, error) {
			select {
			case <-time.After(5 * time.Second):
				return rootServer(), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		g := newFallbackRig(t, failWith(errRefused), slow, true)
		g.lookupNames(t, 5)
		time.Sleep(time.Minute)
		synctest.Wait()

		// The first probe runs while DoH is on and times out after 3 s. The second, at 4 s,
		// runs while DoH is off and has the time it needs.
		want := []Status{active, {Mode: StatusFailing, Error: "timed out"}, active}
		if got := g.statuses.get(); !slices.Equal(got, want) {
			t.Errorf("statuses = %v, want %v", got, want)
		}
	})
}

func TestRecoveryFromFailing(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), failWith(errRefused), true)
		g.lookupNames(t, 5)
		synctest.Wait()

		g.server.set(usable, usable)
		time.Sleep(2 * time.Second) // the round at 1 s
		synctest.Wait()
		if got := g.statuses.get(); !slices.Equal(got, []Status{active, failingRefused, active}) {
			t.Errorf("statuses = %v, want active again", got)
		}

		probes := len(g.server.probeTimes())
		time.Sleep(10 * time.Minute)
		if n := len(g.server.probeTimes()); n != probes {
			t.Errorf("%d probes after recovery, want none", n-probes)
		}
		before := g.lookups()
		g.lookupNames(t, 1)
		if g.lookups() != before+1 {
			t.Error("lookup after recovery didn't reach the client")
		}
	})
}

func TestNoRouteAtStart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, usable, usable, false)
		if got := g.statuses.get(); !slices.Equal(got, []Status{noIPv4}) {
			t.Fatalf("statuses when New returns = %v, want noIPv4", got)
		}

		g.lookupNames(t, 1)
		synctest.Wait()
		if len(g.server.probeTimes()) != 0 || g.lookups() != 0 || g.os.count() != 1 {
			t.Errorf("without a route: %d probes, %d DoH lookups and %d OS lookups, want 0, 0 and 1",
				len(g.server.probeTimes()), g.lookups(), g.os.count())
		}
	})
}

func TestRouteLostBeforeFiveFailures(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), usable, true)

		g.route.Store(false)
		g.lookupNames(t, 5)
		synctest.Wait()

		if got := g.statuses.get(); !slices.Equal(got, []Status{active, noIPv4}) {
			t.Errorf("statuses = %v, want noIPv4, not failing", got)
		}
		if n := len(g.server.probeTimes()); n != 0 {
			t.Errorf("%d probes without a route, want none", n)
		}
	})
}

func TestRouteAppears(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		probe answerFunc
		want  []Status
	}{
		{name: "appears with a usable probe", probe: usable, want: []Status{noIPv4, active}},
		{name: "appears with a failed probe", probe: failWith(errRefused), want: []Status{noIPv4, failingRefused}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := newFallbackRig(t, usable, tt.probe, false)

				synctest.Wait()
				g.route.Store(true)
				time.Sleep(1500 * time.Millisecond) // the round at 1 s
				synctest.Wait()

				if got := g.statuses.get(); !slices.Equal(got, tt.want) {
					t.Errorf("statuses = %v, want %v", got, tt.want)
				}
			})
		})
	}
}

func TestVPNHidesStateAndStopsProbes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, usable, usable, true)

		g.r.SetRouting(nil, true)
		g.route.Store(false)
		g.r.NetworkChanged()
		g.route.Store(true)
		time.Sleep(10 * time.Minute)

		if got := g.statuses.get(); !slices.Equal(got, []Status{active, vpn}) {
			t.Errorf("statuses = %v, want vpn whatever the state", got)
		}
		if n := len(g.server.probeTimes()); n != 0 {
			t.Errorf("%d probes while a VPN handles all DNS, want none", n)
		}

		// The rounds under the VPN found a route but sent no probe, so they left the state
		// as it was. Only a probe turns DoH back on.
		g.r.SetRouting(nil, false)
		synctest.Wait()
		if got := g.statuses.get(); !slices.Equal(got, []Status{active, vpn, noIPv4, active}) {
			t.Errorf("statuses = %v, want noIPv4 until the probe", got)
		}
	})
}

func TestClearingVPNProbesAtOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), failWith(errRefused), true)
		g.lookupNames(t, 5)
		time.Sleep(100 * time.Second) // the next round is at 123 s
		g.r.SetRouting(nil, true)
		g.server.set(usable, usable)

		time.Sleep(10 * time.Second)
		g.r.SetRouting(nil, false)
		synctest.Wait()

		want := []Status{active, failingRefused, vpn, failingRefused, active}
		if got := g.statuses.get(); !slices.Equal(got, want) {
			t.Errorf("statuses = %v, want %v", got, want)
		}
		if probes := g.server.probeTimes(); probes[len(probes)-1] != 110*time.Second {
			t.Errorf("last probe at %v, want 110s, when the flag cleared", probes[len(probes)-1])
		}
	})
}

func TestNetworkChangedProbesAtOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// A SERVFAIL probe doesn't reset the client, so every reset is NetworkChanged's.
		var resets []int // the client's resets as each probe arrives
		g := newFallbackRig(t, failWith(errRefused), nil, true)
		g.server.set(failWith(errRefused), func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) {
			resets = append(resets, g.client.resetCount())
			return rcode(dnsmessage.RCodeServerFailure), nil
		})
		g.lookupNames(t, 5)
		time.Sleep(100 * time.Second)

		g.r.NetworkChanged()
		time.Sleep(1500 * time.Millisecond)
		synctest.Wait()

		// Rounds at 0, 1, 3, 7, 15, 31 and 63 s, then 100 s, and 101 s after the
		// shortest wait.
		want := []time.Duration{0, 1, 3, 7, 15, 31, 63, 100, 101}
		for i := range want {
			want[i] *= time.Second
		}
		if got := g.server.probeTimes(); !slices.Equal(got, want) {
			t.Errorf("probes at %v, want %v", got, want)
		}
		if wantResets := []int{0, 0, 0, 0, 0, 0, 0, 1, 1}; !slices.Equal(resets, wantResets) {
			t.Errorf("client resets seen by the probes = %v, want %v", resets, wantResets)
		}
	})
}

func TestNetworkChangedAbandonsProbe(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// The first probe times out after 3 s and later ones after 10 s, since DoH is off
		// by then: rounds at 0, 4 and 16 s, then 30 s.
		g := newFallbackRig(t, failWith(errRefused), hang, true)
		g.lookupNames(t, 5)
		time.Sleep(17 * time.Second)

		g.r.NetworkChanged()
		time.Sleep(12 * time.Second)
		synctest.Wait()

		// The probe at 17 s replaces the one from 16 s. Only one round follows the
		// change, and the wait after it starts again from 1 s.
		want := []time.Duration{0, 4, 16, 17, 28}
		for i := range want {
			want[i] *= time.Second
		}
		if got := g.server.probeTimes(); !slices.Equal(got, want) {
			t.Errorf("probes at %v, want %v", got, want)
		}
		// The probes that timed out, from 0, 4 and 17 s, each reset the client, and so did
		// NetworkChanged. The abandoned probe from 16 s didn't.
		if got := g.client.resetCount(); got != 4 {
			t.Errorf("client reset %d times, want 4", got)
		}
	})
}

func TestNetworkChangedWithoutRoute(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, usable, usable, true)

		g.route.Store(false)
		g.r.NetworkChanged()
		if got := g.statuses.get(); !slices.Equal(got, []Status{active, noIPv4}) {
			t.Fatalf("statuses when NetworkChanged returns = %v, want noIPv4", got)
		}
		g.r.NetworkChanged()
		g.route.Store(true)
		time.Sleep(1500 * time.Millisecond)
		synctest.Wait()

		if got := g.statuses.get(); !slices.Equal(got, []Status{active, noIPv4, active}) {
			t.Errorf("statuses = %v, want active once the route is back", got)
		}
	})
}

func TestNetworkChangedIgnoresOldProbe(t *testing.T) {
	t.Parallel()

	// In both cases, a probe is waiting when the route goes away.
	tests := []struct {
		name  string
		probe answerFunc
	}{
		{name: "old probe times out", probe: hang},
		{
			name: "old probe comes back usable",
			probe: func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) {
				// Unlike the real client, this ignores the cancelled context, so that the
				// round's own check is what keeps the answer out.
				time.Sleep(2 * time.Second)
				return rootServer(), nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := newFallbackRig(t, failWith(errRefused), tt.probe, true)
				g.lookupNames(t, 5)

				time.Sleep(time.Second)
				g.route.Store(false)
				g.r.NetworkChanged()
				time.Sleep(10 * time.Second)
				synctest.Wait()

				if got := g.statuses.get(); !slices.Equal(got, []Status{active, noIPv4}) {
					t.Errorf("statuses = %v, want the old probe to change nothing", got)
				}
				before := g.lookups()
				g.lookupNames(t, 1)
				if g.lookups() != before {
					t.Error("lookup without a route reached the client")
				}
			})
		})
	}
}

func TestStatusError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "context deadline", err: fmt.Errorf("probe: %w", context.DeadlineExceeded), want: "timed out"},
		{
			name: "dial timeout",
			err: &net.OpError{
				Op: "dial", Net: "tcp",
				Addr: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 443},
				Err:  os.ErrDeadlineExceeded,
			},
			want: "timed out",
		},
		{name: "connection reset", err: connReset(62167), want: "read tcp: read: connection reset by peer"},
		{name: "connection reset, another port", err: connReset(62170), want: "read tcp: read: connection reset by peer"},
		{
			name: "other error",
			err:  errors.New("tls: failed to verify certificate: x509: certificate is valid for a.example, not dns.quad9.net"),
			want: "tls: failed to verify certificate: x509: certificate is valid for a.example, not dns.quad9.net",
		},
		{
			name: "expired certificate",
			err: &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{
				Reason: x509.Expired,
				Detail: "current time 2026-11-12T20:47:41Z is after 2025-01-01T00:00:00Z",
			}},
			want: "x509: certificate has expired or is not yet valid",
		},
		{name: "SERVFAIL", err: rcodeError(dnsmessage.RCodeServerFailure), want: "server answered SERVFAIL"},
		{name: "FORMERR", err: rcodeError(dnsmessage.RCodeFormatError), want: "server answered FORMERR"},
		{name: "NOTIMP", err: rcodeError(dnsmessage.RCodeNotImplemented), want: "server answered NOTIMP"},
		{name: "REFUSED", err: rcodeError(dnsmessage.RCodeRefused), want: "server answered REFUSED"},
		{name: "code without a common name", err: rcodeError(9), want: "server answered rcode 9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := statusError(tt.err); got != tt.want {
				t.Errorf("statusError = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewPortsGiveOneStatus(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var port atomic.Int32
		port.Store(60000)
		probe := func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) {
			return nil, connReset(int(port.Add(1)))
		}
		g := newFallbackRig(t, failWith(errRefused), probe, true)

		g.lookupNames(t, 5)
		time.Sleep(10 * time.Second)

		want := []Status{active, {Mode: StatusFailing, Error: "read tcp: read: connection reset by peer"}}
		if got := g.statuses.get(); !slices.Equal(got, want) {
			t.Errorf("statuses = %v, want %v", got, want)
		}
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		probe answerFunc
		at    time.Duration
	}{
		{name: "during a wait", probe: failWith(errRefused), at: 500 * time.Millisecond},
		{name: "during a probe", probe: hang, at: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := newFallbackRig(t, failWith(errRefused), tt.probe, true)
				g.lookupNames(t, 5)
				time.Sleep(tt.at)
				g.r.Close()
				probes, statuses := len(g.server.probeTimes()), g.statuses.get()

				// Without the recovery, no status would follow even if the loop kept running.
				g.server.set(usable, usable)
				time.Sleep(2 * time.Minute)
				g.r.Close()
				g.server.set(failWith(errRefused), failWith(errRefused))
				g.lookupNames(t, 5)
				g.route.Store(false)
				g.r.NetworkChanged()
				g.r.SetRouting(nil, true)
				time.Sleep(2 * time.Minute)

				if n := len(g.server.probeTimes()); n != probes {
					t.Errorf("%d probes after Close, want none", n-probes)
				}
				if got := g.statuses.get(); !slices.Equal(got, statuses) {
					t.Errorf("statuses = %v, want none after %v", got, statuses)
				}
			})
		})
	}
}

func TestNetworkChangedAfterCloseKeepsDoHOn(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, usable, usable, true)
		g.r.Close()
		g.route.Store(false)
		g.r.NetworkChanged()

		// No loop could turn DoH back on, and lookups still work after Close.
		if g.r.dohOff.Load() {
			t.Error("DoH off after NetworkChanged without a route after Close")
		}
	})
}

func TestCloseRightAfterNetworkChanged(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), hang, true)
		g.lookupNames(t, 5)
		time.Sleep(time.Second)

		// NetworkChanged abandons the waiting probe and kicks the loop. The loop must
		// still stop, whichever of the kick and the close it sees first.
		g.r.NetworkChanged()
		g.r.Close()
		time.Sleep(5 * time.Minute)

		if n := len(g.server.probeTimes()); n != 1 {
			t.Errorf("%d probes, want only the one from before Close", n)
		}
	})
}

func TestNilOnStatus(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("192.0.2.99")}}
		r := newResolver(&fakeDoH{answer: failWith(errRefused)}, nil, os.lookup, func() bool { return true }, nil)
		t.Cleanup(r.Close)

		for i := range 5 {
			if _, err := ask(t.Context(), r, question(fmt.Sprintf("broken-%d.example.", i), dnsmessage.TypeA)); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		if !r.dohOff.Load() {
			t.Error("DoH still on after a failed probe")
		}
	})
}

func TestSameStatusIsReportedOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, usable, usable, true)

		g.r.SetRouting(nil, false)
		g.r.SetRouting([]string{"corp.example"}, false)
		g.route.Store(false)
		g.r.NetworkChanged()
		g.r.NetworkChanged()
		g.r.SetRouting(nil, true)
		g.r.SetRouting(nil, true)
		synctest.Wait()

		if got := g.statuses.get(); !slices.Equal(got, []Status{active, noIPv4, vpn}) {
			t.Errorf("statuses = %v, want each once", got)
		}
	})
}

func TestConcurrentCallsAreRaceFree(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := newFallbackRig(t, failWith(errRefused), failWith(errRefused), true)

		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				for j := range 20 {
					_, _ = ask(t.Context(), g.r, question(fmt.Sprintf("n%d-%d.example.", i, j), dnsmessage.TypeA))
					time.Sleep(100 * time.Millisecond)
				}
			})
		}
		wg.Go(func() {
			for i := range 20 {
				g.route.Store(i%3 != 0)
				g.r.NetworkChanged()
				time.Sleep(150 * time.Millisecond)
			}
		})
		wg.Go(func() {
			for i := range 20 {
				g.r.SetRouting(nil, i%2 == 0)
				time.Sleep(70 * time.Millisecond)
			}
		})
		wg.Go(func() {
			time.Sleep(time.Second)
			g.r.Close()
		})
		wg.Wait()
	})
}

type answerFunc = func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error)

func failWith(err error) answerFunc {
	return func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) { return nil, err }
}

// rootServer returns a usual answer to a probe.
func rootServer() *dnsmessage.Message {
	return answer(a("a.root-servers.net.", 518400, "198.41.0.4"))
}

// connReset returns the error a read gets when the server resets the connection, with
// the local port set to port.
func connReset(port int) error {
	return &net.OpError{
		Op:     "read",
		Net:    "tcp",
		Source: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port},
		Addr:   &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 62165},
		Err:    os.NewSyscallError("read", syscall.ECONNRESET),
	}
}

// fallbackRig is a resolver with a fake server, OS lookup and route, closed when the
// test ends.
type fallbackRig struct {
	r        *Resolver
	client   *fakeDoH
	server   *fakeServer
	os       *fakeOS
	route    *atomic.Bool
	statuses *statusLog
	// names is how many names lookupNames has used.
	names int
}

// newFallbackRig returns a rig whose server answers lookups with lookup and probes with
// probe. routeUp is whether there is an IPv4 route when the resolver starts.
func newFallbackRig(t *testing.T, lookup, probe answerFunc, routeUp bool) *fallbackRig {
	t.Helper()

	g := &fallbackRig{
		server:   &fakeServer{start: time.Now()},
		os:       &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("192.0.2.99")}},
		route:    &atomic.Bool{},
		statuses: &statusLog{},
	}
	g.client = &fakeDoH{answer: g.server.answer}
	g.server.set(lookup, probe)
	g.route.Store(routeUp)
	g.r = newResolver(g.client, nil, g.os.lookup, g.route.Load, g.statuses.add)
	t.Cleanup(g.r.Close)
	return g
}

// lookupNames looks up n names that haven't been looked up before, one after another.
func (g *fallbackRig) lookupNames(t *testing.T, n int) {
	t.Helper()

	for range n {
		g.names++
		name := fmt.Sprintf("name-%d.example.", g.names)
		if _, err := ask(t.Context(), g.r, question(name, dnsmessage.TypeA)); err != nil {
			t.Fatal(err)
		}
	}
}

// lookups returns how many queries other than probes reached the client.
func (g *fallbackRig) lookups() int {
	return g.client.count() - len(g.server.probeTimes())
}

// fakeServer answers like a DoH server whose answers a test can change while the
// resolver runs. It answers the probe's name with probe, and every other name with
// lookup.
type fakeServer struct {
	start time.Time

	mu     sync.Mutex
	lookup answerFunc
	probe  answerFunc
	// probes holds when each probe arrived, since start.
	probes []time.Duration
}

func (s *fakeServer) set(lookup, probe answerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookup, s.probe = lookup, probe
}

func (s *fakeServer) answer(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
	s.mu.Lock()
	f := s.lookup
	if strings.EqualFold(q.Name.String(), probeQuestion.Name.String()) {
		f = s.probe
		s.probes = append(s.probes, time.Since(s.start))
	}
	s.mu.Unlock()
	return f(ctx, q)
}

func (s *fakeServer) probeTimes() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.probes)
}

// statusLog records the statuses a resolver reports.
type statusLog struct {
	mu   sync.Mutex
	list []Status
}

func (l *statusLog) add(s Status) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, s)
}

func (l *statusLog) get() []Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.list)
}
