package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"slices"
	"sync/atomic"

	"github.com/irbis-sh/zen-desktop/internal/redacted"
	utls "github.com/refraction-networking/utls"
)

// Zen terminates the client's TLS and opens its own connection upstream. Made with
// crypto/tls, that connection carries Go's ClientHello under the browser's User-Agent.
// Bot-detection services read the mismatch as a bot signal: DataDome, for one, answers
// it with a hard "You have been blocked" page, where a direct visit from the same
// browser gets a device check it passes. Replaying the client's own ClientHello
// upstream through uTLS keeps the TLS fingerprint the server sees the same as without
// Zen. The HTTP/2 fingerprint (SETTINGS, window sizes, header order) is still Go's.

const (
	recordHeaderLen     = 5
	recordTypeHandshake = 22
	// maxRecordLen is the largest plaintext record TLS allows (RFC 8446, section 5.1).
	maxRecordLen = 1 << 14
)

// readClientHello reads the first TLS record from r, which for a TLS client is the
// one carrying its ClientHello. The returned bytes are everything read from r, even
// on error, so the caller can replay them to the TLS server and let it report the
// failure.
func readClientHello(r io.Reader) ([]byte, error) {
	hdr := make([]byte, recordHeaderLen)
	if n, err := io.ReadFull(r, hdr); err != nil {
		return hdr[:n], err
	}
	if hdr[0] != recordTypeHandshake {
		return hdr, fmt.Errorf("record type %d is not handshake", hdr[0])
	}
	length := int(binary.BigEndian.Uint16(hdr[3:]))
	if length > maxRecordLen {
		return hdr, fmt.Errorf("record length %d exceeds %d", length, maxRecordLen)
	}

	rec := make([]byte, recordHeaderLen+length)
	copy(rec, hdr)
	n, err := io.ReadFull(r, rec[recordHeaderLen:])
	return rec[:recordHeaderLen+n], err
}

// clientHelloSpec parses a ClientHello record into a uTLS spec.
//
// A spec cannot be shared between connections: uTLS fills its extensions in place
// during the handshake. Each dial parses its own.
func clientHelloSpec(record []byte) (*utls.ClientHelloSpec, error) {
	// Browsers send extensions before uTLS knows them: Chrome 154 sends 0xca34, an
	// experimental code point for Trust Anchor IDs. Refusing unknown extensions would
	// mean Go's hello for every connection from that browser, so they are passed
	// through as raw bytes. uTLS does not act on them, and still verifies the
	// certificate the server picks. An extension that only informs the server is
	// harmless, but one that changes how records are protected, such as
	// encrypt_then_mac from OpenSSL clients, fails the handshake if the server accepts
	// it. The dialer then falls back to Go's hello. One that limits record size, such as
	// max_fragment_length from embedded TLS libraries or record_size_limit from Firefox,
	// is not honoured either. A server that answers with a limit below the TLS maximum
	// fails only on the first larger record, too late to fall back. Few servers do.
	spec, err := (&utls.Fingerprinter{AllowBluntMimicry: true}).FingerprintClientHello(record)
	if err != nil {
		return nil, err
	}

	// A client resuming its session with Zen offers Zen's session ticket, which means
	// nothing upstream. Without it the hello is the client's first-connection hello.
	spec.Extensions = slices.DeleteFunc(spec.Extensions, func(ext utls.TLSExtension) bool {
		_, ok := ext.(utls.PreSharedKeyExtension)
		return ok
	})

	for _, ext := range spec.Extensions {
		if keyShare, ok := ext.(*utls.KeyShareExtension); ok {
			keyShare.KeyShares = dropLaterClassicalKeyShares(keyShare.KeyShares)
		}
	}

	return spec, nil
}

// dropLaterClassicalKeyShares keeps only the first classical key share, leaving GREASE
// and post-quantum hybrid shares alone.
//
// uTLS keeps the private key of the first classical share only, so a server picking a
// later one fails the handshake with "invalid server key share"
// (https://github.com/refraction-networking/utls/issues/402). Firefox offers X25519 and
// P-256, and some servers pick P-256. Without its share, such a server asks for one in
// a HelloRetryRequest, which uTLS answers. Fingerprints such as JA3 and JA4 read the
// supported groups, which stay as the client sent them, not the key shares.
func dropLaterClassicalKeyShares(shares []utls.KeyShare) []utls.KeyShare {
	seenClassical := false
	return slices.DeleteFunc(shares, func(share utls.KeyShare) bool {
		switch share.Group {
		case utls.X25519, utls.CurveP256, utls.CurveP384, utls.CurveP521:
			if seenClassical {
				return true
			}
			seenClassical = true
		}
		return false
	})
}

// mirroringTransport returns a transport whose connections present clientHello
// upstream. It serves a single client connection: there is no key to share
// connections under, as hellos differ between clients and Chrome shuffles its
// extension order on every connection.
func (p *Proxy) mirroringTransport(clientHello []byte) *http.Transport {
	t := p.requestTransport.Clone()
	dialer := &mirroringDialer{
		dialContext: t.DialContext,
		clientHello: clientHello,
	}
	if t.TLSClientConfig != nil {
		dialer.rootCAs = t.TLSClientConfig.RootCAs
	}
	t.DialTLSContext = dialer.DialTLSContext
	return t
}

// errMirroring marks a dial that failed on the mirrored hello rather than on reaching
// the server.
var errMirroring = errors.New("mirroring client hello")

// mirroringDialer dials TLS connections that present a client's ClientHello.
//
// Once mirroring fails, it dials with Go's hello instead, for that dial and every later
// one. uTLS cannot mirror every hello against every server, and Go's hello is what Zen
// sent before mirroring existed. Without the fallback, the failure would reach
// connectHandler as a TLS error, which stops Zen filtering the host.
type mirroringDialer struct {
	dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	clientHello []byte
	// rootCAs is nil outside of tests, which means the system roots. Tests set it to
	// trust their upstream's certificate.
	rootCAs    *x509.CertPool
	useGoHello atomic.Bool
}

// DialTLSContext implements http.Transport's DialTLSContext.
func (d *mirroringDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	if !d.useGoHello.Load() {
		conn, err := d.dialMirrored(ctx, network, addr, host)
		if !errors.Is(err, errMirroring) {
			return conn, err
		}
		log.Printf("dialing(%s), falling back to Go's hello: %v", redacted.Redacted(addr), redacted.Redacted(err))
		d.useGoHello.Store(true)
	}

	return d.dialGo(ctx, network, addr, host)
}

func (d *mirroringDialer) dialMirrored(ctx context.Context, network, addr, host string) (net.Conn, error) {
	spec, err := clientHelloSpec(d.clientHello)
	if err != nil {
		return nil, fmt.Errorf("%w: parse: %w", errMirroring, err)
	}

	rawConn, err := d.dialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	conn := utls.UClient(rawConn, &utls.Config{ServerName: host, RootCAs: d.rootCAs}, utls.HelloCustom)
	if err := conn.ApplyPreset(spec); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("%w: apply: %w", errMirroring, err)
	}

	// http.Transport applies TLSHandshakeTimeout only to handshakes it makes itself.
	ctx, cancel := context.WithTimeout(ctx, tlsHandshakeTimeout)
	defer cancel()
	if err := conn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		if ctx.Err() != nil {
			// A handshake cut short by the timeout or the request ending says nothing
			// about the hello, and Go's would only wait as long again.
			return nil, err
		}
		return nil, fmt.Errorf("%w: handshake: %w", errMirroring, err)
	}

	// The version range and cipher suites come from the client's hello, and some clients
	// still offer TLS 1.0 or 3DES. A connection Go's hello would refuse for either falls
	// back to it and fails there, as it did before mirroring. The rest of Go's floor does
	// not carry over: uTLS predates Go 1.25 and accepts SHA-1 signatures in TLS 1.2, as
	// browsers do.
	if state := conn.ConnectionState(); state.Version < tls.VersionTLS12 || !isSecureCipherSuite(state.CipherSuite) {
		rawConn.Close()
		return nil, fmt.Errorf("%w: negotiated %s with %s", errMirroring, tls.VersionName(state.Version), tls.CipherSuiteName(state.CipherSuite))
	}

	return uconn{conn}, nil
}

// isSecureCipherSuite reports whether crypto/tls lists id among its suites without
// known security issues.
func isSecureCipherSuite(id uint16) bool {
	return slices.ContainsFunc(tls.CipherSuites(), func(suite *tls.CipherSuite) bool {
		return suite.ID == id
	})
}

// dialGo dials with Go's hello, configured as http.Transport configures its own.
func (d *mirroringDialer) dialGo(ctx context.Context, network, addr, host string) (net.Conn, error) {
	rawConn, err := d.dialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	conn := tls.Client(rawConn, &tls.Config{
		ServerName: host,
		NextProtos: []string{"h2", "http/1.1"},
		RootCAs:    d.rootCAs,
	})

	ctx, cancel := context.WithTimeout(ctx, tlsHandshakeTimeout)
	defer cancel()
	if err := conn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}

	return conn, nil
}

// uconn adapts a uTLS connection to http.Transport. The transport only speaks HTTP/2
// over a connection whose ConnectionState method returns crypto/tls's type.
type uconn struct {
	*utls.UConn
}

func (c uconn) ConnectionState() tls.ConnectionState {
	s := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:                     s.Version,
		HandshakeComplete:           s.HandshakeComplete,
		DidResume:                   s.DidResume,
		CipherSuite:                 s.CipherSuite,
		NegotiatedProtocol:          s.NegotiatedProtocol,
		ServerName:                  s.ServerName,
		PeerCertificates:            s.PeerCertificates,
		VerifiedChains:              s.VerifiedChains,
		SignedCertificateTimestamps: s.SignedCertificateTimestamps,
		OCSPResponse:                s.OCSPResponse,
	}
}

// replayConn is a net.Conn that serves reads from r, which replays bytes already
// consumed from the connection before reading on.
type replayConn struct {
	net.Conn
	r io.Reader
}

func newReplayConn(conn net.Conn, consumed []byte) *replayConn {
	return &replayConn{Conn: conn, r: io.MultiReader(bytes.NewReader(consumed), conn)}
}

func (c *replayConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}
