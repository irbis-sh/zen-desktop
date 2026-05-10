package proxy

import (
	"net"
	"sync"
)

// singleConnListener is a net.Listener that returns exactly one connection
// from Accept and then blocks until Close is called.
type singleConnListener struct {
	conn net.Conn
	once sync.Once
	ch   chan struct{}
}

func newSingleConnListener(c net.Conn) *singleConnListener {
	return &singleConnListener{
		conn: c,
		ch:   make(chan struct{}),
	}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	var c net.Conn
	l.once.Do(func() { c = l.conn })
	if c != nil {
		return c, nil
	}
	// Block until Close is called.
	<-l.ch
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	select {
	case <-l.ch:
		// Already closed.
	default:
		close(l.ch)
	}
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}
