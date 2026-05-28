package process

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFindPIDBySourcePort(t *testing.T) {
	t.Parallel()

	t.Run("finds owning process for active connection", func(t *testing.T) {
		t.Parallel()

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		serverConn := make(chan net.Conn, 1)
		go func() {
			c, _ := ln.Accept()
			serverConn <- c
		}()

		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		if sc := <-serverConn; sc != nil {
			defer sc.Close()
		}

		port := conn.LocalAddr().(*net.TCPAddr).Port

		pid, err := findPIDBySourcePort(uint16(port)) // #nosec G115 -- port will fit in uint16
		if err != nil {
			t.Fatalf("findPIDBySourcePort(%d): %v", port, err)
		}

		if int(pid) != os.Getpid() {
			t.Errorf("PID = %d, want %d", pid, os.Getpid())
		}
	})

	t.Run("returns ErrNotFound for unbound port", func(t *testing.T) {
		t.Parallel()

		if _, err := findPIDBySourcePort(0); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want %v", err, ErrNotFound)
		}
	})
}

func TestFindByRequest(t *testing.T) {
	t.Parallel()

	t.Run("finds owning process for local HTTP request", func(t *testing.T) {
		t.Parallel()

		type result struct {
			info Info
			err  error
		}

		results := make(chan result, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			info, err := FindByRequest(r)
			results <- result{info: info, err: err}
		}))
		defer srv.Close()

		resp, err := srv.Client().Get(srv.URL)
		if err != nil {
			t.Fatalf("GET %s: %v", srv.URL, err)
		}
		defer resp.Body.Close()

		got := <-results
		if got.err != nil {
			t.Fatalf("FindByRequest: %v", got.err)
		}
		if int(got.info.PID) != os.Getpid() {
			t.Errorf("PID = %d, want %d", got.info.PID, os.Getpid())
		}
		if got.info.ExecutablePath == "" {
			t.Errorf("ExecutablePath = %q, want non-empty path", got.info.ExecutablePath)
		}
	})

	t.Run("returns error for malformed RemoteAddr", func(t *testing.T) {
		t.Parallel()

		_, err := FindByRequest(&http.Request{RemoteAddr: "127.0.0.1"})
		if err == nil {
			t.Fatal("err = nil")
		}
	})

	t.Run("returns error for invalid source port", func(t *testing.T) {
		t.Parallel()

		_, err := FindByRequest(&http.Request{RemoteAddr: "127.0.0.1:meow"})
		if err == nil {
			t.Fatal("err = nil")
		}
	})
}
