package resolver

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// The tests here use testing/synctest, so that the time limits come out exact and an
// entry's TTL passes at once. They call dial directly, with a fake DoH client and a fake OS
// lookup.
//
// Never use a real net.Resolver inside a bubble, including through Resolver.NetResolver or
// New's OS lookup. Package net keeps its resolv.conf state in a global, with a channel
// created on first use. If a bubble creates it, the next bubble or test to look up a name
// dies with "fatal error: send on synctest channel from outside bubble", which no test can
// recover from. The tests that need a real net.Resolver are in resolver_test.go, outside
// any bubble.

func TestServeAnswersGoQuery(t *testing.T) {
	t.Parallel()

	r := newResolver(&fakeDoH{answer: always(answer(cname(testHost, 300, "cdn.example."), a("cdn.example.", 300, "192.0.2.1")))}, nil, nil)
	query := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 1234, RecursionDesired: true},
		Questions: []dnsmessage.Question{question(testHost, dnsmessage.TypeA)},
	}

	got, err := send(t.Context(), r, &query)
	if err != nil {
		t.Fatal(err)
	}

	if got.ID != 1234 || !got.Response || !got.RecursionAvailable || !got.RecursionDesired {
		t.Errorf("header = %+v", got.Header)
	}
	if len(got.Questions) != 1 || got.Questions[0] != query.Questions[0] {
		t.Errorf("questions = %v", got.Questions)
	}
	// Only address records, owned by the question's name, with a TTL of 0.
	if len(got.Answers) != 1 {
		t.Fatalf("answers = %+v, want one record", got.Answers)
	}
	h := got.Answers[0].Header
	if h.Type != dnsmessage.TypeA || h.Name != query.Questions[0].Name || h.TTL != 0 {
		t.Errorf("record header = %+v", h)
	}
	if len(got.Authorities) != 0 || len(got.Additionals) != 0 {
		t.Errorf("extra sections: %+v, %+v", got.Authorities, got.Additionals)
	}
}

func TestServeCopiesClearRD(t *testing.T) {
	t.Parallel()

	r := newResolver(&fakeDoH{answer: always(nxdomain())}, nil, nil)
	query := dnsmessage.Message{Questions: []dnsmessage.Question{question(testHost, dnsmessage.TypeA)}}

	got, err := send(t.Context(), r, &query)
	if err != nil {
		t.Fatal(err)
	}

	if got.RecursionDesired {
		t.Error("RD set in the answer, though the query had it clear")
	}
}

func TestServeAnswersNOTIMP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		q    dnsmessage.Question
	}{
		{name: "other type", q: question(testHost, dnsmessage.TypeMX)},
		{name: "other class", q: dnsmessage.Question{
			Name:  dnsmessage.MustNewName(testHost),
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassCHAOS,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeDoH{}
			r := newResolver(client, nil, nil)

			got, err := ask(t.Context(), r, tt.q)
			if err != nil {
				t.Fatal(err)
			}

			if got.RCode != dnsmessage.RCodeNotImplemented {
				t.Errorf("rcode = %v, want NOTIMP", got.RCode)
			}
			if client.count() != 0 {
				t.Errorf("client got %d queries, want none", client.count())
			}
		})
	}
}

func TestServeClosesPipeOnBadQuery(t *testing.T) {
	t.Parallel()

	two := dnsmessage.Message{Questions: []dnsmessage.Question{
		question(testHost, dnsmessage.TypeA),
		question(testHost, dnsmessage.TypeAAAA),
	}}
	packedTwo, err := two.Pack()
	if err != nil {
		t.Fatal(err)
	}
	packedNone, err := (&dnsmessage.Message{}).Pack()
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string][]byte{
		"garbage":       {1, 2, 3},
		"two questions": packedTwo,
		"no question":   packedNone,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newResolver(&fakeDoH{}, nil, nil)
			conn, err := r.dial(t.Context(), "udp", "")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			if err := writeMessage(conn, raw); err != nil {
				t.Fatal(err)
			}
			if _, err := readMessage(conn); err == nil {
				t.Error("got an answer, want the pipe closed")
			}
		})
	}
}

func TestDoHOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		msg       *dnsmessage.Message
		wantRCode dnsmessage.RCode
		wantAddrs int
		wantOS    int
	}{
		{name: "addresses", msg: answer(a(testHost, 60, "192.0.2.1")), wantAddrs: 1},
		{name: "no records", msg: withSOA(answer(), 60, 60)},
		{name: "NXDOMAIN", msg: nxdomain(), wantRCode: dnsmessage.RCodeNameError},
		{name: "blocked by EDE", msg: withEDE(answer(a(testHost, 60, "192.0.2.1")), 15), wantRCode: dnsmessage.RCodeNameError},
		{name: "blocked by 0.0.0.0", msg: answer(a(testHost, 60, "0.0.0.0")), wantRCode: dnsmessage.RCodeNameError},
		{name: "DNSSEC failure", msg: withEDE(rcode(dnsmessage.RCodeServerFailure), 7), wantRCode: dnsmessage.RCodeServerFailure},
		{name: "private address", msg: answer(a(testHost, 60, "10.0.0.1")), wantAddrs: 1, wantOS: 1},
		{name: "REFUSED", msg: rcode(dnsmessage.RCodeRefused), wantAddrs: 1, wantOS: 1},
		{name: "SERVFAIL", msg: rcode(dnsmessage.RCodeServerFailure), wantAddrs: 1, wantOS: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.2")}}
			r := newResolver(&fakeDoH{answer: always(tt.msg)}, nil, os.lookup)

			got, err := ask(t.Context(), r, question(testHost, dnsmessage.TypeA))
			if err != nil {
				t.Fatal(err)
			}

			if got.RCode != tt.wantRCode || len(got.Answers) != tt.wantAddrs {
				t.Errorf("got %v with %d records, want %v with %d", got.RCode, len(got.Answers), tt.wantRCode, tt.wantAddrs)
			}
			if os.count() != tt.wantOS {
				t.Errorf("OS got %d lookups, want %d", os.count(), tt.wantOS)
			}
		})
	}
}

func TestClientErrorGoesToOSAndIsNotCached(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) {
		return nil, errors.New("x509: certificate signed by unknown authority")
	}}
	os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("192.0.2.1")}}
	r := newResolver(client, nil, os.lookup)
	q := question(testHost, dnsmessage.TypeA)

	for range 2 {
		got, err := ask(t.Context(), r, q)
		if err != nil {
			t.Fatal(err)
		}
		if got.RCode != dnsmessage.RCodeSuccess || len(got.Answers) != 1 {
			t.Errorf("got %v with %d records, want the OS's address", got.RCode, len(got.Answers))
		}
	}

	if client.count() != 2 || os.count() != 2 {
		t.Errorf("client got %d queries and OS %d lookups, want 2 each", client.count(), os.count())
	}
}

func TestCachedAnswerSkipsClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *dnsmessage.Message
	}{
		{name: "addresses", msg: answer(a(testHost, 60, "192.0.2.1"))},
		{name: "no records", msg: withSOA(answer(), 60, 60)},
		{name: "NXDOMAIN", msg: nxdomain()},
		{name: "block without an SOA", msg: withEDE(nxdomain(), 17)},
		{name: "DNSSEC failure", msg: withEDE(rcode(dnsmessage.RCodeServerFailure), 7)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeDoH{answer: always(tt.msg)}
			r := newResolver(client, nil, nil)
			q := question(testHost, dnsmessage.TypeA)

			first, err := ask(t.Context(), r, q)
			if err != nil {
				t.Fatal(err)
			}
			second, err := ask(t.Context(), r, q)
			if err != nil {
				t.Fatal(err)
			}

			if client.count() != 1 {
				t.Errorf("client got %d queries, want 1", client.count())
			}
			if first.RCode != second.RCode || len(first.Answers) != len(second.Answers) {
				t.Errorf("cached answer differs: %+v, %+v", first, second)
			}
		})
	}
}

func TestCacheNamesIgnoreCaseAndType(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: always(answer(a(testHost, 60, "192.0.2.1")))}
	r := newResolver(client, nil, nil)

	for _, q := range []dnsmessage.Question{
		question("news.example.", dnsmessage.TypeA),
		question("NEWS.Example.", dnsmessage.TypeA),
		question("news.example.", dnsmessage.TypeAAAA),
	} {
		if _, err := ask(t.Context(), r, q); err != nil {
			t.Fatal(err)
		}
	}

	if client.count() != 2 {
		t.Errorf("client got %d queries, want 2: one per type", client.count())
	}
}

func TestUseOSVerdictExpires(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     *dnsmessage.Message
		lasting time.Duration
	}{
		{name: "private address", msg: answer(a(testHost, 120, "10.0.0.1")), lasting: 2 * time.Minute},
		{name: "another rcode", msg: rcode(dnsmessage.RCodeRefused), lasting: 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				client := &fakeDoH{answer: always(tt.msg)}
				os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.2")}}
				r := newResolver(client, nil, os.lookup)
				q := question(testHost, dnsmessage.TypeA)

				ask(t.Context(), r, q)
				time.Sleep(tt.lasting - time.Millisecond)
				ask(t.Context(), r, q)
				if client.count() != 1 || os.count() != 2 {
					t.Errorf("before expiry: client got %d queries and OS %d lookups, want 1 and 2", client.count(), os.count())
				}

				// The verdict covers its own type only.
				ask(t.Context(), r, question(testHost, dnsmessage.TypeAAAA))
				if client.count() != 2 {
					t.Errorf("AAAA used the A verdict: client got %d queries, want 2", client.count())
				}

				time.Sleep(time.Millisecond)
				ask(t.Context(), r, q)
				if client.count() != 3 {
					t.Errorf("after expiry: client got %d queries, want 3", client.count())
				}
			})
		})
	}
}

func TestRoutedNameSkipsClientAndCache(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: always(answer(a("Wiki.Corp.Example.", 60, "192.0.2.1")))}
	os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.2")}}
	r := newResolver(client, []string{"corp.example"}, os.lookup)

	got, err := ask(t.Context(), r, question("Wiki.Corp.Example.", dnsmessage.TypeA))
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Answers) != 1 || client.count() != 0 {
		t.Errorf("got %d records and %d DoH queries, want 1 and none", len(got.Answers), client.count())
	}
	if want := []string{"wiki.corp.example"}; !slices.Equal(os.hosts(), want) {
		t.Errorf("OS looked up %q, want %q", os.hosts(), want)
	}
}

func TestVPNBypassesCachedAnswer(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: always(answer(a(testHost, 60, "192.0.2.1")))}
	os := &fakeOS{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.2")}}
	r := newResolver(client, nil, os.lookup)
	q := question(testHost, dnsmessage.TypeA)

	if _, err := ask(t.Context(), r, q); err != nil {
		t.Fatal(err)
	}
	r.SetRouting(nil, true)
	got, err := ask(t.Context(), r, q)
	if err != nil {
		t.Fatal(err)
	}

	want := []netip.Addr{netip.MustParseAddr("10.0.0.2")}
	if addrs := addrsOf(t, got); !slices.Equal(addrs, want) {
		t.Errorf("addrs = %v, want the OS's %v", addrs, want)
	}
}

func TestOSLookups(t *testing.T) {
	t.Parallel()

	v4 := netip.MustParseAddr("192.0.2.1")
	mapped := netip.MustParseAddr("::ffff:192.0.2.2")
	v6 := netip.MustParseAddr("2001:db8::1")

	tests := []struct {
		name      string
		addrs     []netip.Addr
		err       error
		qtype     dnsmessage.Type
		wantRCode dnsmessage.RCode
		want      []netip.Addr
	}{
		{name: "IPv4 for A", addrs: []netip.Addr{v4, mapped, v6}, qtype: dnsmessage.TypeA, want: []netip.Addr{v4, mapped.Unmap()}},
		{name: "IPv6 for AAAA", addrs: []netip.Addr{v4, mapped, v6}, qtype: dnsmessage.TypeAAAA, want: []netip.Addr{v6}},
		{name: "only the other type", addrs: []netip.Addr{v6}, qtype: dnsmessage.TypeA},
		{name: "not found", err: &net.DNSError{IsNotFound: true}, qtype: dnsmessage.TypeA, wantRCode: dnsmessage.RCodeNameError},
		{name: "another DNS error", err: &net.DNSError{IsTemporary: true}, qtype: dnsmessage.TypeA, wantRCode: dnsmessage.RCodeServerFailure},
		{name: "another error", err: errors.New("no network"), qtype: dnsmessage.TypeA, wantRCode: dnsmessage.RCodeServerFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			os := &fakeOS{addrs: tt.addrs, err: tt.err}
			r := newResolver(&fakeDoH{}, nil, os.lookup)

			got, err := ask(t.Context(), r, question("nas.lan.", tt.qtype))
			if err != nil {
				t.Fatal(err)
			}

			if got.RCode != tt.wantRCode {
				t.Errorf("rcode = %v, want %v", got.RCode, tt.wantRCode)
			}
			if addrs := addrsOf(t, got); !slices.Equal(addrs, tt.want) {
				t.Errorf("addrs = %v, want %v", addrs, tt.want)
			}
			if !slices.Equal(os.hosts(), []string{"nas.lan"}) {
				t.Errorf("OS looked up %q, want the name without its dot", os.hosts())
			}
		})
	}
}

func TestNetworkChanged(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: always(answer(a(testHost, 60, "192.0.2.1")))}
	r := newResolver(client, nil, nil)
	q := question(testHost, dnsmessage.TypeA)

	if _, err := ask(t.Context(), r, q); err != nil {
		t.Fatal(err)
	}
	r.NetworkChanged()
	if _, err := ask(t.Context(), r, q); err != nil {
		t.Fatal(err)
	}

	if client.count() != 2 {
		t.Errorf("client got %d queries, want 2: the cache should be empty", client.count())
	}
	if got := client.resetCount(); got != 1 {
		t.Errorf("Reset called %d times, want 1", got)
	}
}

func TestWhenDoHGivesWayToOS(t *testing.T) {
	t.Parallel()

	refused := func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) {
		return nil, errors.New("connection refused")
	}
	tests := []struct {
		name    string
		client  func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error)
		timeout time.Duration // 0 means no deadline
		want    time.Duration
	}{
		{name: "a hung lookup gets 60% of the budget", client: hang, timeout: time.Second, want: 600 * time.Millisecond},
		{name: "the budget is 5 s without a deadline", client: hang, want: 3 * time.Second},
		{name: "a fast failure goes at once", client: refused, timeout: time.Second, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				var askedAfter time.Duration
				r := newResolver(&fakeDoH{answer: tt.client}, nil, func(context.Context, string) ([]netip.Addr, error) {
					askedAfter = time.Since(start)
					return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil
				})
				ctx := t.Context()
				if tt.timeout != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tt.timeout)
					defer cancel()
				}

				got, err := ask(ctx, r, question(testHost, dnsmessage.TypeA))
				if err != nil {
					t.Fatal(err)
				}

				if askedAfter != tt.want {
					t.Errorf("OS asked after %v, want %v", askedAfter, tt.want)
				}
				if len(got.Answers) != 1 {
					t.Errorf("got %d records, want the OS's address", len(got.Answers))
				}
			})
		})
	}
}

func TestHungOSEndsInSERVFAILBeforeDeadline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
	}{
		{name: "after a hung DoH lookup", host: testHost},
		{name: "routed to the OS", host: "nas.lan."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				r := newResolver(&fakeDoH{answer: hang}, nil, (&fakeOS{hang: true}).lookup)
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				start := time.Now()

				got, err := ask(ctx, r, question(tt.host, dnsmessage.TypeA))
				if err != nil {
					t.Fatal(err)
				}

				if elapsed := time.Since(start); elapsed != 900*time.Millisecond {
					t.Errorf("answered after %v, want 900ms", elapsed)
				}
				if got.RCode != dnsmessage.RCodeServerFailure {
					t.Errorf("rcode = %v, want SERVFAIL", got.RCode)
				}
			})
		})
	}
}

func TestCancelClosesPipeAtOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
	}{
		{name: "during a DoH lookup", host: testHost},
		{name: "during an OS lookup", host: "nas.lan."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				os := &fakeOS{hang: true}
				r := newResolver(&fakeDoH{answer: hang}, nil, os.lookup)
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()

				done := make(chan error, 1)
				go func() {
					_, err := ask(ctx, r, question(tt.host, dnsmessage.TypeA))
					done <- err
				}()
				synctest.Wait()
				start := time.Now()
				cancel()

				if err := <-done; err == nil {
					t.Error("got an answer, want the pipe closed")
				}
				if elapsed := time.Since(start); elapsed != 0 {
					t.Errorf("pipe closed after %v, want at once", elapsed)
				}
			})
		})
	}
}

func TestCancelDuringDoHDoesNotAskOS(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		os := &fakeOS{}
		r := newResolver(&fakeDoH{answer: hang}, nil, os.lookup)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		go ask(ctx, r, question(testHost, dnsmessage.TypeA))
		synctest.Wait()
		cancel()
		synctest.Wait()

		if os.count() != 0 {
			t.Errorf("OS got %d lookups, want none: nobody is waiting for the answer", os.count())
		}
	})
}

func TestDialRefusesDoneContext(t *testing.T) {
	t.Parallel()

	r := newResolver(&fakeDoH{}, nil, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if conn, err := r.dial(ctx, "udp", ""); !errors.Is(err, context.Canceled) {
		t.Errorf("dial = %v, %v; want context.Canceled", conn, err)
	}
}

func TestPipeClosesWhileWaitingForQuery(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := newResolver(&fakeDoH{}, nil, nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		start := time.Now()

		conn, err := r.dial(ctx, "udp", "")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		if _, err := readMessage(conn); err == nil {
			t.Error("got a message, want the pipe closed")
		}
		if elapsed := time.Since(start); elapsed != time.Second {
			t.Errorf("pipe closed after %v, want 1s", elapsed)
		}
	})
}

func TestLookupsDropRequestContextValues(t *testing.T) {
	t.Parallel()

	type key struct{}
	var clientSaw, osSaw any
	client := &fakeDoH{answer: func(ctx context.Context, _ dnsmessage.Question) (*dnsmessage.Message, error) {
		clientSaw = ctx.Value(key{})
		return rcode(dnsmessage.RCodeRefused), nil
	}}
	r := newResolver(client, nil, func(ctx context.Context, _ string) ([]netip.Addr, error) {
		osSaw = ctx.Value(key{})
		return nil, nil
	})

	ctx := context.WithValue(t.Context(), key{}, "from the request")
	if _, err := ask(ctx, r, question(testHost, dnsmessage.TypeA)); err != nil {
		t.Fatal(err)
	}

	if clientSaw != nil || osSaw != nil {
		t.Errorf("lookups saw the request's values: client %v, OS %v", clientSaw, osSaw)
	}
}

// ask sends a query for q to r and returns the answer.
func ask(ctx context.Context, r *Resolver, q dnsmessage.Question) (*dnsmessage.Message, error) {
	return send(ctx, r, &dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 1, RecursionDesired: true},
		Questions: []dnsmessage.Question{q},
	})
}

// send plays Go's part in an exchange, over dial.
func send(ctx context.Context, r *Resolver, query *dnsmessage.Message) (*dnsmessage.Message, error) {
	conn, err := r.dial(ctx, "udp", "")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	packed, err := query.Pack()
	if err != nil {
		return nil, err
	}
	if err := writeMessage(conn, packed); err != nil {
		return nil, err
	}
	raw, err := readMessage(conn)
	if err != nil {
		return nil, err
	}
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return nil, err
	}
	return &msg, nil
}

func addrsOf(t *testing.T, msg *dnsmessage.Message) []netip.Addr {
	t.Helper()

	var addrs []netip.Addr
	for _, rr := range msg.Answers {
		switch body := rr.Body.(type) {
		case *dnsmessage.AResource:
			addrs = append(addrs, netip.AddrFrom4(body.A))
		case *dnsmessage.AAAAResource:
			addrs = append(addrs, netip.AddrFrom16(body.AAAA))
		default:
			t.Errorf("unexpected record: %+v", rr)
		}
	}
	return addrs
}
