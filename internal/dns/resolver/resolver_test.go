package resolver

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

// These tests run through a real net.Resolver, so they must stay outside synctest bubbles.
// See the comment in serve_test.go. How often Go retries depends on the machine's DNS
// configuration, so they count the fake's queries rather than Go's dials.

func TestNetResolverReturnsFakeAddresses(t *testing.T) {
	t.Parallel()

	client := &fakeDoH{answer: func(_ context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
		if q.Type == dnsmessage.TypeAAAA {
			return answer(cname(testHost, 60, "cdn.example."), aaaa("cdn.example.", 60, "2001:db8::1")), nil
		}
		return answer(cname(testHost, 60, "cdn.example."), a("cdn.example.", 60, "192.0.2.1"), a("cdn.example.", 60, "192.0.2.2")), nil
	}}
	r := newResolver(client, nil, nil)

	got, err := r.NetResolver().LookupNetIP(t.Context(), "ip", testHost)
	if err != nil {
		t.Fatal(err)
	}

	// Go sorts addresses by the machine's routes (RFC 6724), which differ between runners.
	unmapped := make([]netip.Addr, 0, len(got))
	for _, a := range got {
		unmapped = append(unmapped, a.Unmap())
	}
	slices.SortFunc(unmapped, netip.Addr.Compare)
	want := []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
		netip.MustParseAddr("2001:db8::1"),
	}
	if !slices.Equal(unmapped, want) {
		t.Errorf("LookupNetIP = %v, want %v", unmapped, want)
	}
}

func TestNetResolverDoesNotRepeatFinalAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *dnsmessage.Message
	}{
		{name: "NXDOMAIN", msg: nxdomain()},
		{name: "NOERROR without records or an SOA", msg: answer()},
		{name: "DNSSEC SERVFAIL, which Go retries", msg: withEDE(rcode(dnsmessage.RCodeServerFailure), 7)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeDoH{answer: always(tt.msg)}
			r := newResolver(client, nil, nil)

			addrs, err := r.NetResolver().LookupNetIP(t.Context(), "ip", testHost)
			if err == nil {
				t.Fatalf("LookupNetIP = %v, want an error", addrs)
			}

			if client.count() != 2 {
				t.Errorf("client got %d queries, want 2: one per type", client.count())
			}
		})
	}
}

// fakeDoH is a scriptable dohClient.
type fakeDoH struct {
	// answer replies to each query.
	answer func(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error)

	mu      sync.Mutex
	queries int
	resets  int
}

func (f *fakeDoH) Query(ctx context.Context, q dnsmessage.Question) (*dnsmessage.Message, error) {
	f.mu.Lock()
	f.queries++
	f.mu.Unlock()
	if f.answer == nil {
		return nil, errors.New("fakeDoH: unexpected query")
	}
	return f.answer(ctx, q)
}

func (f *fakeDoH) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resets++
}

func (f *fakeDoH) Close() {}

func (f *fakeDoH) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries
}

func (f *fakeDoH) resetCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resets
}

// always returns a fakeDoH answer function that always replies with msg.
func always(msg *dnsmessage.Message) func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) {
	return func(context.Context, dnsmessage.Question) (*dnsmessage.Message, error) { return msg, nil }
}

// hang is a fakeDoH answer function that never replies.
func hang(ctx context.Context, _ dnsmessage.Question) (*dnsmessage.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// fakeOS is a scriptable OS lookup.
type fakeOS struct {
	addrs []netip.Addr
	err   error
	// hang makes lookups wait for their context.
	hang bool

	mu    sync.Mutex
	asked []string
}

func (f *fakeOS) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	f.asked = append(f.asked, host)
	f.mu.Unlock()
	if f.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.addrs, f.err
}

func (f *fakeOS) count() int { return len(f.hosts()) }

func (f *fakeOS) hosts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

func question(name string, qtype dnsmessage.Type) dnsmessage.Question {
	return dnsmessage.Question{Name: dnsmessage.MustNewName(name), Type: qtype, Class: dnsmessage.ClassINET}
}

// answer returns a NOERROR response holding records.
func answer(records ...dnsmessage.Resource) *dnsmessage.Message {
	return &dnsmessage.Message{Header: dnsmessage.Header{Response: true}, Answers: records}
}

func nxdomain() *dnsmessage.Message { return rcode(dnsmessage.RCodeNameError) }

func rcode(c dnsmessage.RCode) *dnsmessage.Message {
	return &dnsmessage.Message{Header: dnsmessage.Header{Response: true, RCode: c}}
}

// withSOA adds an SOA record to msg's authority section.
func withSOA(msg *dnsmessage.Message, ttl, minimum uint32) *dnsmessage.Message {
	msg.Authorities = append(msg.Authorities, dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{
			Name:  dnsmessage.MustNewName("example."),
			Type:  dnsmessage.TypeSOA,
			Class: dnsmessage.ClassINET,
			TTL:   ttl,
		},
		Body: &dnsmessage.SOAResource{
			NS:     dnsmessage.MustNewName("ns.example."),
			MBox:   dnsmessage.MustNewName("admin.example."),
			MinTTL: minimum,
		},
	})
	return msg
}

// withEDE adds an OPT record with one Extended DNS Error per code to msg.
func withEDE(msg *dnsmessage.Message, codes ...uint16) *dnsmessage.Message {
	data := make([][]byte, 0, len(codes))
	for _, c := range codes {
		data = append(data, append(binary.BigEndian.AppendUint16(nil, c), 'x'))
	}
	return withRawEDE(msg, data...)
}

func withRawEDE(msg *dnsmessage.Message, data ...[]byte) *dnsmessage.Message {
	options := make([]dnsmessage.Option, 0, 1+len(data))
	options = append(options, dnsmessage.Option{Code: 12}) // padding comes first
	for _, d := range data {
		options = append(options, dnsmessage.Option{Code: optionEDE, Data: d})
	}
	msg.Additionals = append(msg.Additionals, dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName("."), Type: dnsmessage.TypeOPT},
		Body:   &dnsmessage.OPTResource{Options: options},
	})
	return msg
}

func a(name string, ttl uint32, addr string) dnsmessage.Resource {
	return dnsmessage.Resource{
		Header: header(name, dnsmessage.TypeA, ttl),
		Body:   &dnsmessage.AResource{A: netip.MustParseAddr(addr).As4()},
	}
}

func aaaa(name string, ttl uint32, addr string) dnsmessage.Resource {
	return dnsmessage.Resource{
		Header: header(name, dnsmessage.TypeAAAA, ttl),
		Body:   &dnsmessage.AAAAResource{AAAA: netip.MustParseAddr(addr).As16()},
	}
}

func cname(name string, ttl uint32, target string) dnsmessage.Resource {
	return dnsmessage.Resource{
		Header: header(name, dnsmessage.TypeCNAME, ttl),
		Body:   &dnsmessage.CNAMEResource{CNAME: dnsmessage.MustNewName(target)},
	}
}

func header(name string, qtype dnsmessage.Type, ttl uint32) dnsmessage.ResourceHeader {
	return dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName(name), Type: qtype, Class: dnsmessage.ClassINET, TTL: ttl}
}
