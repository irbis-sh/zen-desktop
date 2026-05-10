package proxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

func (p *Proxy) proxyWebsocketTLS(w http.ResponseWriter, req *http.Request) {
	dialer := &tls.Dialer{NetDialer: p.netDialer, Config: &tls.Config{MinVersion: tls.VersionTLS12}}
	hijackAndTunnelWebsocket(w, req, dialer.Dial)
}

func (p *Proxy) proxyWebsocket(w http.ResponseWriter, req *http.Request) {
	hijackAndTunnelWebsocket(w, req, p.netDialer.Dial)
}

func hijackAndTunnelWebsocket(w http.ResponseWriter, req *http.Request, dial func(network, addr string) (net.Conn, error)) {
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

	targetConn, err := dial("tcp", req.URL.Host)
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

	return nil
}

func isWS(r *http.Request) bool {
	// RFC 6455, the WebSocket Protocol specification, does not explicitly specify if the Upgrade header
	// should only contain the value "websocket" or not, so we employ some defensive programming here.
	return headerContains(r.Header, "Connection", "upgrade") &&
		headerContains(r.Header, "Upgrade", "websocket")
}
