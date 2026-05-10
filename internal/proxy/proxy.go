package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/irbis-sh/process"
	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

// certGenerator is an interface capable of generating certificates for the proxy.
type certGenerator interface {
	GetCertificate(host string) (*tls.Certificate, error)
}

// filter is an interface capable of filtering HTTP requests.
type filter interface {
	HandleRequest(*http.Request, process.PID) (*http.Response, error)
	HandleResponse(*http.Request, *http.Response, process.PID) error
}

// ShouldProxyFunc should report whether requests from processPath should be handled by the proxy.
// Returning false makes the proxy tunnel/forward traffic without filtering or MITM.
type ShouldProxyFunc func(processPath string) bool

// Proxy is a forward HTTP/HTTPS proxy that can filter requests.
type Proxy struct {
	filter             filter
	certGenerator      certGenerator
	port               int
	server             *http.Server
	requestTransport   http.RoundTripper
	requestClient      *http.Client
	netDialer          *net.Dialer
	shouldProxy        ShouldProxyFunc
	transparentHosts   []string
	transparentHostsMu sync.RWMutex
}

func NewProxy(filter filter, certGenerator certGenerator, port int, shouldProxy ShouldProxyFunc) (*Proxy, error) {
	if filter == nil {
		return nil, errors.New("filter is nil")
	}
	if certGenerator == nil {
		return nil, errors.New("certGenerator is nil")
	}

	p := &Proxy{
		filter:        filter,
		certGenerator: certGenerator,
		port:          port,
		shouldProxy:   shouldProxy,
	}

	p.netDialer = &net.Dialer{
		// Such high values are set to avoid timeouts on slow connections.
		Timeout:   60 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	p.requestTransport = &http.Transport{
		DialContext:         p.netDialer.DialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 20 * time.Second,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
	}
	p.requestClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: p.requestTransport,
		// Let the client handle any redirects.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return p, nil
}

// Start starts the proxy on the given address.
//
// If Proxy was configured with a port of 0, the actual port will be returned.
func (p *Proxy) Start() (int, error) {
	p.server = &http.Server{
		Handler:           p,
		ReadHeaderTimeout: 10 * time.Second,
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "127.0.0.1", p.port))
	if err != nil {
		return 0, fmt.Errorf("listen: %v", err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	log.Printf("proxy listening on port %d", actualPort)

	go func() {
		if err := p.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("serve: %v", err)
		}
	}()

	return actualPort, nil
}

// Stop stops the proxy.
func (p *Proxy) Stop() error {
	if err := p.shutdownServer(); err != nil {
		return fmt.Errorf("shut down server: %v", err)
	}

	return nil
}

func (p *Proxy) shutdownServer() error {
	if p.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		// As per documentation:
		// Shutdown does not attempt to close nor wait for hijacked connections such as WebSockets. The caller of Shutdown should separately notify such long-lived connections of shutdown and wait for them to close, if desired. See RegisterOnShutdown for a way to register shutdown notification functions.
		// TODO: implement websocket shutdown
		return fmt.Errorf("server shutdown: %w", err)
	}

	return nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid, err := process.FindPIDByRequest(r)
	if err != nil {
		log.Printf("error finding request process: %v", err)
		pid = 0 // Defensively set to avoid potentially bogus PIDs
	}

	shouldProxy := true
	if p.shouldProxy != nil && pid != 0 {
		processPath, err := pid.ExecutablePath()
		if err != nil {
			log.Printf("error finding request process path: %v", err)
		}
		shouldProxy = p.shouldProxy(processPath)
	}

	if r.Method == http.MethodConnect {
		p.proxyConnect(w, r, pid, shouldProxy)
	} else {
		p.proxyHTTP(w, r, pid, shouldProxy)
	}
}

// proxyHTTP proxies the HTTP request to the remote server.
func (p *Proxy) proxyHTTP(w http.ResponseWriter, r *http.Request, pid process.PID, shouldProxy bool) {
	if shouldProxy {
		filterResp, err := p.filter.HandleRequest(r, pid)
		if err != nil {
			log.Printf("error handling request for %q: %v", redacted.Redacted(r.URL), err)
		}

		if filterResp != nil {
			filterResp.Write(w)
			return
		}
	}

	if isWS(r) {
		p.proxyWebsocket(w, r)
		return
	}

	r.RequestURI = ""
	r.Close = false

	// Check before removeHopHeaders strips Te.
	teTrailers := headerContains(r.Header, "Te", "trailers")

	removeHopHeaders(r.Header)

	if teTrailers {
		r.Header.Set("Te", "trailers")
	}

	if _, ok := r.Header["User-Agent"]; !ok {
		// If the outbound request doesn't have a User-Agent header set,
		// don't send the default Go HTTP client User-Agent.
		r.Header.Set("User-Agent", "")
	}

	var (
		roundTripMutex sync.Mutex
		roundTripDone  bool
	)
	trace := &httptrace.ClientTrace{
		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			roundTripMutex.Lock()
			defer roundTripMutex.Unlock()
			if roundTripDone {
				return nil
			}
			h := w.Header()
			for k, vv := range header {
				for _, v := range vv {
					h.Add(k, v)
				}
			}
			w.WriteHeader(code)
			clear(h)
			return nil
		},
	}
	r = r.WithContext(httptrace.WithClientTrace(r.Context(), trace))

	resp, err := p.requestClient.Do(r) // #nosec G704 -- this is a proxy; forwarding requests is its purpose
	roundTripMutex.Lock()
	roundTripDone = true
	roundTripMutex.Unlock()
	if err != nil {
		log.Printf("error making request: %v", redacted.Redacted(err)) // The error might contain information about the hostname we are connecting to.
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	removeHopHeaders(resp.Header)

	if shouldProxy {
		if err := p.filter.HandleResponse(r, resp, pid); err != nil {
			log.Printf("error handling response by filter: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}

	writeResp(w, resp)
}

// proxyConnect proxies the initial CONNECT and subsequent data between the
// client and the remote server.
func (p *Proxy) proxyConnect(w http.ResponseWriter, connReq *http.Request, pid process.PID, shouldProxy bool) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Fatal("http server does not support hijacking")
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		log.Printf("hijacking connection(%s): %v", redacted.Redacted(connReq.Host), err)
		return
	}
	defer clientConn.Close()

	host, _, err := net.SplitHostPort(connReq.Host)
	if err != nil {
		log.Printf("splitting host and port(%s): %v", redacted.Redacted(connReq.Host), err)
		return
	}

	if !shouldProxy {
		p.tunnel(clientConn, connReq)
		return
	}

	if !p.shouldMITM(host) || net.ParseIP(host) != nil {
		// TODO: implement upstream certificate sniffing
		// https://docs.mitmproxy.org/stable/concepts-howmitmproxyworks/#complication-1-whats-the-remote-hostname
		p.tunnel(clientConn, connReq)
		return
	}

	tlsCert, err := p.certGenerator.GetCertificate(host)
	if err != nil {
		log.Printf("getting certificate(%s): %v", redacted.Redacted(connReq.Host), err)
		return
	}

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n")); err != nil {
		log.Printf("writing 200 OK to client(%s): %v", redacted.Redacted(connReq.Host), err)
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*tlsCert},
		NextProtos:   []string{"h2", "http/1.1"},
		MinVersion:   tls.VersionTLS12,
	}

	tlsConn := tls.Server(clientConn, tlsConfig)
	defer tlsConn.Close()

	// Perform the TLS handshake manually so we can capture TLS errors
	// and add the host to transparentHosts before entering the server loop.
	if err := tlsConn.HandshakeContext(context.Background()); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "tls: ") {
			log.Printf("adding %s to ignored hosts", redacted.Redacted(host))
			p.addTransparentHost(host)
		}
		log.Printf("TLS handshake(%s): %v", redacted.Redacted(connReq.Host), err)
		return
	}

	ln := newSingleConnListener(tlsConn)

	srv := &http.Server{
		Handler:   p.connectHandler(connReq, host, ln, pid),
		TLSConfig: tlsConfig,
		ConnState: func(_ net.Conn, state http.ConnState) {
			if state == http.StateClosed {
				ln.Close()
			}
		},
		ReadHeaderTimeout: 20 * time.Second,
	}

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		log.Printf("serving connection(%s): %v", redacted.Redacted(connReq.Host), err)
	}
}

// connectHandler returns an http.Handler that processes requests on a CONNECT-tunnelled TLS connection.
func (p *Proxy) connectHandler(connReq *http.Request, host string, ln *singleConnListener, pid process.PID) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.URL.Host = connReq.Host
		req.URL.Scheme = "https"
		req.RequestURI = ""
		req.Close = false

		// WebSocket upgrade is only done over HTTP/1.1.
		if isWS(req) && req.ProtoMajor == 1 {
			p.proxyWebsocketTLS(w, req)
			ln.Close()
			return
		}

		// Check before removeHopHeaders strips Te.
		teTrailers := headerContains(req.Header, "Te", "trailers")

		removeHopHeaders(req.Header)

		if teTrailers {
			req.Header.Set("Te", "trailers")
		}

		if _, ok := req.Header["User-Agent"]; !ok {
			// If the outbound request doesn't have a User-Agent header set,
			// don't send the default Go HTTP client User-Agent.
			req.Header.Set("User-Agent", "")
		}

		filterResp, err := p.filter.HandleRequest(req, pid)
		if err != nil {
			log.Printf("handling request for %q: %v", redacted.Redacted(req.URL), err)
		}
		if filterResp != nil {
			writeResp(w, filterResp)
			if filterResp.Body != nil {
				filterResp.Body.Close()
			}
			return
		}

		// Go's HTTP server always sets a non-nil value for req.Body.
		// RoundTrip interprets a non-nil Body as chunked, which causes strict servers to reject the request.
		if req.ContentLength == 0 {
			req.Body = nil
		}

		var (
			roundTripMutex sync.Mutex
			roundTripDone  bool
		)
		trace := &httptrace.ClientTrace{
			Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
				roundTripMutex.Lock()
				defer roundTripMutex.Unlock()
				if roundTripDone {
					return nil
				}
				h := w.Header()
				for k, vv := range header {
					for _, v := range vv {
						h.Add(k, v)
					}
				}
				w.WriteHeader(code)
				// Clear headers, which is not done automatically by ResponseWriter.WriteHeader() for 1xx responses.
				clear(h)
				return nil
			},
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

		resp, err := p.requestTransport.RoundTrip(req)
		roundTripMutex.Lock()
		roundTripDone = true
		roundTripMutex.Unlock()
		if err != nil {
			if strings.Contains(err.Error(), "tls: ") {
				log.Printf("adding %s to ignored hosts", redacted.Redacted(host))
				p.addTransparentHost(host)
			}
			log.Printf("roundtrip(%s): %v", redacted.Redacted(connReq.Host), err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		removeHopHeaders(resp.Header)

		if err := p.filter.HandleResponse(req, resp, pid); err != nil {
			log.Printf("error handling response by filter for %q: %v", redacted.Redacted(req.URL), err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		writeResp(w, resp)
	})
}

// shouldMITM returns true if the host should be MITM'd.
func (p *Proxy) shouldMITM(host string) bool {
	p.transparentHostsMu.RLock()
	defer p.transparentHostsMu.RUnlock()

	for _, transparentHost := range p.transparentHosts {
		if host == transparentHost || strings.HasSuffix(host, "."+transparentHost) {
			return false
		}
	}

	return true
}

// addTransparentHost adds a host to the list of hosts that should be MITM'd.
func (p *Proxy) addTransparentHost(host string) {
	p.transparentHostsMu.Lock()
	defer p.transparentHostsMu.Unlock()

	p.transparentHosts = append(p.transparentHosts, host)
}

// tunnel tunnels the connection between the client and the remote server
// without inspecting the traffic.
func (p *Proxy) tunnel(w net.Conn, r *http.Request) {
	remoteConn, err := net.Dial("tcp", r.Host) // #nosec G704 -- this is a proxy; forwarding connections is its purpose
	if err != nil {
		log.Printf("dialing remote(%s): %v", redacted.Redacted(r.Host), err)
		w.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer remoteConn.Close()

	if _, err := w.Write([]byte("HTTP/1.1 200 OK\r\n\r\n")); err != nil {
		log.Printf("writing 200 OK to client(%s): %v", redacted.Redacted(r.Host), err)
		return
	}

	linkBidirectionalTunnel(w, remoteConn)
}

// writeResp writes the response (status code, headers, and body) to the ResponseWriter.
//
// writeResp closes resp.Body to populate trailers for HTTP/1.1 chunked responses.
// The caller's deferred Body.Close is still safe (double close is benign for HTTP response bodies).
func writeResp(w http.ResponseWriter, resp *http.Response) {
	// Announce trailers before writing the status line so net/http can
	// emit a proper Trailer header in the chunked response.
	announcedTrailers := len(resp.Trailer)
	if announcedTrailers > 0 {
		trailerKeys := make([]string, 0, len(resp.Trailer))
		for k := range resp.Trailer {
			trailerKeys = append(trailerKeys, k)
		}
		w.Header().Add("Trailer", strings.Join(trailerKeys, ", "))
	}

	for h, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(h, vv)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if resp.Body != nil {
		var dst io.Writer = w
		if isStreamingResponse(resp) {
			rc := http.NewResponseController(w)
			dst = &flushWriter{w: w, flush: rc.Flush}
		}
		_, err := io.Copy(dst, resp.Body)
		// Close the body before reading trailers;
		// resp.Trailer is only populated after the body is fully consumed and closed.
		resp.Body.Close()
		if err != nil {
			panic(http.ErrAbortHandler)
		}
	}

	if len(resp.Trailer) > 0 {
		// Force chunking if we saw a response trailer.
		// This prevents net/http from calculating the length for short
		// bodies and adding a Content-Length.
		http.NewResponseController(w).Flush()
	}

	if len(resp.Trailer) == announcedTrailers {
		for h, v := range resp.Trailer {
			for _, vv := range v {
				w.Header().Add(h, vv)
			}
		}
		return
	}
	for h, v := range resp.Trailer {
		for _, vv := range v {
			w.Header().Add(http.TrailerPrefix+h, vv)
		}
	}
}

func linkBidirectionalTunnel(src, dst io.ReadWriter) {
	doneC := make(chan struct{}, 2)
	go tunnelConn(src, dst, doneC)
	go tunnelConn(dst, src, doneC)
	<-doneC
	<-doneC
}

// tunnelConn tunnels the data between src and dst.
func tunnelConn(dst io.Writer, src io.Reader, done chan<- struct{}) {
	if _, err := io.Copy(dst, src); err != nil && !isCloseable(err) {
		log.Printf("copying: %v", err)
	}
	done <- struct{}{}
}

// headerContains returns true if the named header contains the given value
// as a comma-separated token (case-insensitive).
func headerContains(h http.Header, name, value string) bool {
	for _, v := range h[name] {
		for _, s := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(s), value) {
				return true
			}
		}
	}
	return false
}

// isCloseable returns true if the error is one that indicates the connection
// can be closed.
func isCloseable(err error) (ok bool) {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	switch err {
	case io.EOF, io.ErrClosedPipe, io.ErrUnexpectedEOF:
		return true
	default:
		return false
	}
}

// isStreamingResponse reports whether the response should be flushed
// to the client immediately (e.g. Server-Sent Events, chunked streams).
func isStreamingResponse(resp *http.Response) bool {
	if ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); ct == "text/event-stream" {
		return true
	}
	return resp.ContentLength == -1
}

// flushWriter wraps an io.Writer and calls flush after every Write
// to ensure streaming data reaches the client without buffering delay.
type flushWriter struct {
	w     io.Writer
	flush func() error
}

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if n > 0 {
		f.flush()
	}
	return n, err
}

// Hop-by-hop headers. These are removed when sent to the backend.
// As of RFC 7230, hop-by-hop headers are required to appear in the
// Connection header field. These are the headers defined by the
// obsoleted RFC 2616 (section 13.5.1) and are used for backward
// compatibility.
var hopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",      // canonicalized version of "TE"
	"Trailer", // spelling per https://www.rfc-editor.org/errata_search.php?eid=4522
	"Transfer-Encoding",
	"Upgrade",
}

func removeHopHeaders(header http.Header) {
	// RFC 7230, section 6.1: Remove headers listed in the "Connection" header.
	for _, f := range header["Connection"] {
		for _, sf := range strings.Split(f, ",") {
			if sf = strings.TrimSpace(sf); sf != "" {
				header.Del(sf)
			}
		}
	}
	// RFC 2616, section 13.5.1: Remove a set of known hop-by-hop headers.
	// This behavior is superseded by the RFC 7230 Connection header, but
	// preserve it for backwards compatibility.
	for _, h := range hopHeaders {
		header.Del(h)
	}
}
