package proxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestWebsocketForwardsFrameSentWithHandshake covers a server that speaks first, as
// chat and live-update servers often do: its first frame goes out in the same write as
// the 101, so the proxy reads both in one go. The frame must still reach the client.
func TestWebsocketForwardsFrameSentWithHandshake(t *testing.T) {
	t.Parallel()

	// An unmasked text frame carrying "hello" (RFC 6455, section 5.2).
	const greeting = "\x81\x05hello"
	upstream := startCannedUpstream(t, "HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"\r\n"+
		greeting)

	// A raw connection rather than proxyClient: the client's timeout stops bounding reads
	// once a 101 hands over the connection, so without the deadline a regression hangs.
	conn, err := net.DialTimeout("tcp", startTestProxy(t, nil), backstopTimeout)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(backstopTimeout)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "http://"+upstream+"/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	if err := req.WriteProxy(conn); err != nil {
		t.Fatalf("write request: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}

	got := make([]byte, len(greeting))
	if _, err := io.ReadFull(br, got); err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if string(got) != greeting {
		t.Fatalf("greeting = %q, want %q", got, greeting)
	}
}

// startCannedUpstream starts a server that accepts one connection, reads a request
// from it, and answers with reply in a single write. It returns the server's address.
func startCannedUpstream(t *testing.T, reply string) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Closing with the request unread would send a RST, which can discard the reply
		// before the proxy reads it.
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return
		}
		io.WriteString(conn, reply)
	}()

	return ln.Addr().String()
}
