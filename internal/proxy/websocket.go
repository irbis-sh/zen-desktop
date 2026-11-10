package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

func (p *Proxy) proxyWebsocketTLS(w http.ResponseWriter, req *http.Request) {
	config := &tls.Config{
		// tls.Client fails without it: unlike tls.Dialer, it can't take the name from the
		// address.
		ServerName: req.URL.Hostname(),
		MinVersion: tls.VersionTLS12,
		// NextProtos stays empty: the upgrade needs HTTP/1.1.
	}
	// As in mirroringTransport: nil outside of tests, which means the system roots.
	if c := p.requestTransport.TLSClientConfig; c != nil {
		config.RootCAs = c.RootCAs
	}

	// Not tls.Dialer: its NetDialer field takes a *net.Dialer, which would skip p.dial.
	hijackAndTunnelWebsocket(w, req, func(ctx context.Context, network, addr string) (net.Conn, error) {
		rawConn, err := p.dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		conn := tls.Client(rawConn, config)

		ctx, cancel := context.WithTimeout(ctx, tlsHandshakeTimeout)
		defer cancel()
		if err := conn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return nil, err
		}
		return conn, nil
	})
}

func (p *Proxy) proxyWebsocket(w http.ResponseWriter, req *http.Request) {
	hijackAndTunnelWebsocket(w, req, p.dial)
}

// hijackAndTunnelWebsocket takes over the client's connection, opens one to the upstream
// with dial, and passes the handshake and then the frames between the two. It dials with
// req's context. After the hijack, that context ends when the handler returns, which is
// after the dial.
func hijackAndTunnelWebsocket(w http.ResponseWriter, req *http.Request, dial func(ctx context.Context, network, addr string) (net.Conn, error)) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		log.Printf("hijacking websocket(%s): %v", redacted.Redacted(req.URL.Host), err)
		return
	}
	defer clientConn.Close()

	targetConn, err := dial(req.Context(), "tcp", req.URL.Host)
	if err != nil {
		log.Printf("dialing websocket backend(%s): %v", redacted.Redacted(req.URL.Host), err)
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer targetConn.Close()

	if err := websocketHandshake(req, targetConn, clientConn); err != nil {
		return
	}
	linkBidirectionalTunnel(targetConn, clientConn)
}

func websocketHandshake(req *http.Request, targetConn io.ReadWriter, clientConn io.ReadWriter) error {
	err := req.Write(targetConn)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		log.Printf("writing websocket request to backend(%s): %v", redacted.Redacted(req.URL.Host), err)
		return err
	}

	// Reading the 101 can buffer bytes past it: a server that speaks first may send its
	// first frame in the same packet. Those bytes go to the client before the tunnel
	// starts, which reads targetConn itself rather than this buffer.
	targetReader := bufio.NewReader(targetConn)

	resp, err := http.ReadResponse(targetReader, req)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		log.Printf("reading websocket response from backend(%s): %v", redacted.Redacted(req.URL.Host), err)
		return err
	}
	defer resp.Body.Close()

	err = resp.Write(clientConn)
	if err != nil {
		log.Printf("writing websocket response to client(%s): %v", redacted.Redacted(req.URL.Host), err)
		return err
	}

	if n := targetReader.Buffered(); n > 0 {
		buffered, _ := targetReader.Peek(n)
		if _, err := clientConn.Write(buffered); err != nil {
			log.Printf("writing buffered websocket data to client(%s): %v", redacted.Redacted(req.URL.Host), err)
			return err
		}
	}

	return nil
}

func isWS(r *http.Request) bool {
	// RFC 6455, the WebSocket Protocol specification, does not explicitly specify if the Upgrade header
	// should only contain the value "websocket" or not, so we employ some defensive programming here.
	return headerContains(r.Header, "Connection", "upgrade") &&
		headerContains(r.Header, "Upgrade", "websocket")
}
