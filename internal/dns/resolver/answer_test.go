package resolver

import (
	"fmt"
	"net/netip"
	"slices"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

const testHost = "news.example."

func TestInterpret(t *testing.T) {
	t.Parallel()

	public := []netip.Addr{netip.MustParseAddr("192.0.2.1")}
	tests := []struct {
		name string
		msg  *dnsmessage.Message
		want verdict
	}{
		{
			name: "EDE 15 with a NOERROR answer",
			msg:  withEDE(answer(a(testHost, 300, "192.0.2.1")), 15),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: 5 * time.Minute},
		},
		{
			name: "EDE 4, no records, SOA",
			msg:  withEDE(withSOA(nxdomain(), 60, 60), 4),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: time.Minute},
		},
		{
			name: "EDE 16 on a SERVFAIL",
			msg:  withEDE(rcode(dnsmessage.RCodeServerFailure), 16),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: minTTL},
		},
		{
			name: "EDE 17 in an option that isn't the first, no records, no SOA",
			msg:  withEDE(nxdomain(), 29, 17),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: minTTL},
		},
		{
			name: "EDE without an info code",
			msg:  withRawEDE(answer(a(testHost, 60, "192.0.2.1")), []byte{15}),
			want: verdict{addrs: public, ttl: time.Minute},
		},
		{
			name: "EDE outside the lists, as Quad9 sends on ordinary answers",
			msg:  withEDE(answer(a(testHost, 60, "192.0.2.1")), 29),
			want: verdict{addrs: public, ttl: time.Minute},
		},
		{
			name: "0.0.0.0",
			msg:  answer(a(testHost, 30, "0.0.0.0")),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: 30 * time.Second},
		},
		{
			name: "0.0.0.0 with a low TTL",
			msg:  answer(a(testHost, 0, "0.0.0.0")),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: minTTL},
		},
		{
			name: "0.0.0.0 with a high TTL",
			msg:  answer(a(testHost, 86400, "0.0.0.0")),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: 5 * time.Minute},
		},
		{
			name: "0.0.0.0 in a SERVFAIL",
			msg: &dnsmessage.Message{
				Header:  dnsmessage.Header{Response: true, RCode: dnsmessage.RCodeServerFailure},
				Answers: []dnsmessage.Resource{a(testHost, 30, "0.0.0.0")},
			},
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: 30 * time.Second},
		},
		{
			name: "SERVFAIL with a DNSSEC EDE",
			msg:  withEDE(rcode(dnsmessage.RCodeServerFailure), 6),
			want: verdict{rcode: dnsmessage.RCodeServerFailure, ttl: minTTL},
		},
		{
			name: "SERVFAIL with several EDEs, the DNSSEC one last",
			msg:  withEDE(rcode(dnsmessage.RCodeServerFailure), 23, 9),
			want: verdict{rcode: dnsmessage.RCodeServerFailure, ttl: minTTL},
		},
		{
			name: "NXDOMAIN with an SOA",
			msg:  withSOA(nxdomain(), 900, 600),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: 5 * time.Minute},
		},
		{
			name: "NXDOMAIN without an SOA",
			msg:  nxdomain(),
			want: verdict{rcode: dnsmessage.RCodeNameError, ttl: minTTL},
		},
		{
			name: "private address",
			msg:  answer(a(testHost, 120, "10.0.0.1"), a(testHost, 120, "192.0.2.1")),
			want: verdict{useOS: true, ttl: 2 * time.Minute},
		},
		{
			name: "loopback address",
			msg:  answer(a(testHost, 0, "127.0.0.1")),
			want: verdict{useOS: true, ttl: minTTL},
		},
		{
			name: "link-local address",
			msg:  answer(a(testHost, 60, "169.254.0.1")),
			want: verdict{useOS: true, ttl: time.Minute},
		},
		{
			name: "addresses",
			msg:  answer(a(testHost, 120, "192.0.2.1")),
			want: verdict{addrs: public, ttl: 2 * time.Minute},
		},
		{
			name: "addresses with a TTL of 0",
			msg:  answer(a(testHost, 0, "192.0.2.1")),
			want: verdict{addrs: public, ttl: minTTL},
		},
		{
			name: "addresses keep a TTL above the 5-minute cap",
			msg:  answer(a(testHost, 3600, "192.0.2.1")),
			want: verdict{addrs: public, ttl: time.Hour},
		},
		{
			name: "NOERROR without records, with an SOA",
			msg:  withSOA(answer(), 120, 60),
			want: verdict{ttl: time.Minute},
		},
		{
			name: "NOERROR without records, without an SOA",
			msg:  answer(),
			want: verdict{ttl: minTTL},
		},
		{
			name: "SERVFAIL without an EDE",
			msg:  rcode(dnsmessage.RCodeServerFailure),
			want: verdict{useOS: true, ttl: minTTL},
		},
		{
			name: "REFUSED",
			msg:  rcode(dnsmessage.RCodeRefused),
			want: verdict{useOS: true, ttl: minTTL},
		},
		{
			name: "REFUSED with a DNSSEC EDE",
			msg:  withEDE(rcode(dnsmessage.RCodeRefused), 6),
			want: verdict{useOS: true, ttl: minTTL},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := interpret(tt.msg, question(testHost, dnsmessage.TypeA))
			if got.rcode != tt.want.rcode || got.useOS != tt.want.useOS || got.ttl != tt.want.ttl || !slices.Equal(got.addrs, tt.want.addrs) {
				t.Errorf("interpret = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInterpretNegativeTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		soaTTL, soaMin uint32
		want           time.Duration
	}{
		{name: "the SOA's TTL is lower", soaTTL: 60, soaMin: 3600, want: time.Minute},
		{name: "MINIMUM is lower", soaTTL: 3600, soaMin: 60, want: time.Minute},
		{name: "floor", soaTTL: 1, soaMin: 1, want: 10 * time.Second},
		{name: "cap", soaTTL: 86400, soaMin: 86400, want: 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := interpret(withSOA(nxdomain(), tt.soaTTL, tt.soaMin), question(testHost, dnsmessage.TypeA))
			if got.ttl != tt.want {
				t.Errorf("ttl = %v, want %v", got.ttl, tt.want)
			}
		})
	}
}

func TestInterpretMappedAddresses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want verdict
	}{
		{addr: "::", want: verdict{rcode: dnsmessage.RCodeNameError, ttl: time.Minute}},
		{addr: "::ffff:0.0.0.0", want: verdict{rcode: dnsmessage.RCodeNameError, ttl: time.Minute}},
		{addr: "::ffff:10.0.0.1", want: verdict{useOS: true, ttl: time.Minute}},
		{addr: "fd00::1", want: verdict{useOS: true, ttl: time.Minute}},
		{addr: "::1", want: verdict{useOS: true, ttl: time.Minute}},
		{addr: "fe80::1", want: verdict{useOS: true, ttl: time.Minute}},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()

			got := interpret(answer(aaaa(testHost, 60, tt.addr)), question(testHost, dnsmessage.TypeAAAA))
			if got.rcode != tt.want.rcode || got.useOS != tt.want.useOS || got.ttl != tt.want.ttl {
				t.Errorf("interpret = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInterpretChain(t *testing.T) {
	t.Parallel()

	q := question(testHost, dnsmessage.TypeA)
	want := []netip.Addr{netip.MustParseAddr("192.0.2.1")}

	tests := []struct {
		name    string
		msg     *dnsmessage.Message
		wantTTL time.Duration
	}{
		{
			name: "in order",
			msg: answer(
				cname(testHost, 300, "a.cdn.example."),
				cname("a.cdn.example.", 200, "b.cdn.example."),
				a("b.cdn.example.", 400, "192.0.2.1"),
			),
			wantTTL: 200 * time.Second,
		},
		{
			name: "reversed",
			msg: answer(
				a("b.cdn.example.", 400, "192.0.2.1"),
				cname("a.cdn.example.", 200, "b.cdn.example."),
				cname(testHost, 300, "a.cdn.example."),
			),
			wantTTL: 200 * time.Second,
		},
		{
			name: "names differ in case",
			msg: answer(
				cname("NEWS.example.", 300, "A.cdn.example."),
				a("a.CDN.example.", 100, "192.0.2.1"),
			),
			wantTTL: 100 * time.Second,
		},
		{
			name: "records of other owners are ignored",
			msg: answer(
				a("other.example.", 1, "192.0.2.9"),
				cname("other.example.", 1, "another.example."),
				cname(testHost, 300, "a.cdn.example."),
				a(testHost, 1, "192.0.2.8"),
				a("a.cdn.example.", 300, "192.0.2.1"),
			),
			wantTTL: 300 * time.Second,
		},
		{
			name:    "16 CNAME records",
			msg:     answer(chain(16)...),
			wantTTL: 300 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := interpret(tt.msg, q)
			if !slices.Equal(got.addrs, want) || got.ttl != tt.wantTTL {
				t.Errorf("got addrs %v, ttl %v; want %v, %v", got.addrs, got.ttl, want, tt.wantTTL)
			}
		})
	}
}

func TestInterpretChainTooLong(t *testing.T) {
	t.Parallel()

	got := interpret(answer(chain(17)...), question(testHost, dnsmessage.TypeA))

	if got.useOS || got.rcode != dnsmessage.RCodeSuccess || len(got.addrs) != 0 {
		t.Errorf("got %+v, want NOERROR without records", got)
	}
}

func TestInterpretChainLoop(t *testing.T) {
	t.Parallel()

	msg := answer(cname(testHost, 300, "a.example."), cname("a.example.", 300, testHost))
	got := interpret(msg, question(testHost, dnsmessage.TypeA))

	if got.rcode != dnsmessage.RCodeSuccess || len(got.addrs) != 0 {
		t.Errorf("got %+v, want NOERROR without records", got)
	}
}

func TestInterpretKeepsOnlyTheQuestionsType(t *testing.T) {
	t.Parallel()

	msg := answer(a(testHost, 60, "192.0.2.1"), aaaa(testHost, 60, "2001:db8::1"))
	got := interpret(msg, question(testHost, dnsmessage.TypeAAAA))

	if want := []netip.Addr{netip.MustParseAddr("2001:db8::1")}; !slices.Equal(got.addrs, want) {
		t.Errorf("addrs = %v, want %v", got.addrs, want)
	}
}

func TestInterpretCapsAddresses(t *testing.T) {
	t.Parallel()

	records := make([]dnsmessage.Resource, 0, maxAddrs+1)
	for i := range maxAddrs + 1 {
		ttl := uint32(60)
		if i == maxAddrs {
			ttl = 0 // the TTL of a record that isn't kept doesn't count
		}
		records = append(records, a(testHost, ttl, fmt.Sprintf("192.0.2.%d", i)))
	}

	got := interpret(answer(records...), question(testHost, dnsmessage.TypeA))

	if len(got.addrs) != maxAddrs || got.ttl != time.Minute {
		t.Errorf("kept %d addresses with ttl %v, want %d with 1m0s", len(got.addrs), got.ttl, maxAddrs)
	}
}

// chain returns n CNAME records from testHost to an address.
func chain(n int) []dnsmessage.Resource {
	records := make([]dnsmessage.Resource, 0, n+1)
	name := testHost
	for i := range n {
		next := fmt.Sprintf("hop%d.example.", i)
		records = append(records, cname(name, 300, next))
		name = next
	}
	return append(records, a(name, 300, "192.0.2.1"))
}
