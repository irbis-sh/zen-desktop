package process

import (
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

func BenchmarkFindPIDBySourcePort(b *testing.B) {
	conn := newBenchmarkTCPConnection(b)
	port := uint16(conn.sourcePort) // #nosec G115 -- TCP ports fit in uint16.

	b.ResetTimer()

	for b.Loop() {
		_, err := findPIDBySourcePort(port)
		if err != nil {
			b.Fatalf("findPIDBySourcePort(%d): %v", conn.sourcePort, err)
		}
	}
}

func BenchmarkFindByRequest(b *testing.B) {
	conn := newBenchmarkTCPConnection(b)
	req := &http.Request{RemoteAddr: conn.remoteAddr}

	b.ResetTimer()

	for b.Loop() {
		_, err := FindByRequest(req)
		if err != nil {
			b.Fatalf("FindByRequest: %v", err)
		}
	}
}

func BenchmarkFindByRequestParallel(b *testing.B) {
	conn := newBenchmarkTCPConnection(b)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		req := &http.Request{RemoteAddr: conn.remoteAddr}

		for pb.Next() {
			_, err := FindByRequest(req)
			if err != nil {
				b.Errorf("FindByRequest: %v", err)
				return
			}
		}
	})
}

func BenchmarkPIDExecutablePath(b *testing.B) {
	pid := PID(os.Getpid())

	b.ResetTimer()

	for b.Loop() {
		_, err := pidExecutablePath(pid)
		if err != nil {
			b.Fatalf("pidExecutablePath(%d): %v", pid, err)
		}
	}
}

func BenchmarkInfoNameWithExecutablePath(b *testing.B) {
	pid := PID(os.Getpid())
	path, err := pidExecutablePath(pid)
	if err != nil {
		b.Fatalf("pidExecutablePath(%d): %v", pid, err)
	}
	info := Info{PID: pid, ExecutablePath: path}

	b.ResetTimer()

	for b.Loop() {
		_, err := info.Name()
		if err != nil {
			b.Fatalf("Name: %v", err)
		}
	}
}

func BenchmarkInfoNameWithoutExecutablePath(b *testing.B) {
	info := Info{PID: PID(os.Getpid())}

	b.ResetTimer()

	for b.Loop() {
		_, err := info.Name()
		if err != nil {
			b.Fatalf("Name: %v", err)
		}
	}
}

type benchmarkTCPConnection struct {
	remoteAddr string
	sourcePort int
}

func newBenchmarkTCPConnection(tb testing.TB) *benchmarkTCPConnection {
	tb.Helper()

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("listen: %v", err)
	}

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	clientConn, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		ln.Close()
		tb.Fatalf("dial: %v", err)
	}

	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErr:
		clientConn.Close()
		ln.Close()
		tb.Fatalf("accept: %v", err)
	case <-time.After(5 * time.Second):
		clientConn.Close()
		ln.Close()
		tb.Fatal("accept: timed out")
	}

	remoteAddr := clientConn.LocalAddr().String()
	tcpAddr, ok := clientConn.LocalAddr().(*net.TCPAddr)
	if !ok {
		clientConn.Close()
		serverConn.Close()
		ln.Close()
		tb.Fatalf("client local address has type %T, want *net.TCPAddr", clientConn.LocalAddr())
	}

	conn := &benchmarkTCPConnection{
		remoteAddr: remoteAddr,
		sourcePort: tcpAddr.Port,
	}
	tb.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
		ln.Close()
	})

	return conn
}
