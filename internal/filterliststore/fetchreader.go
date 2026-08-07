package filterliststore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// errStalled reports a download killed by the stall watchdog: the connection
// stayed open but the body stopped yielding bytes.
var errStalled = errors.New("download stalled")

// fetchReader wraps a download (a [cachingReader] over the response body) with
// the fetch lifecycle:
//   - a stall watchdog that cancels the request when the body stops making
//     progress, and
//   - release of the fetch slot at body EOF or Close, whichever comes first,
//     so a slot is never held through the caller's parse tail.
//
// Like cachingReader, it does not support Close concurrently with Read.
type fetchReader struct {
	inner        io.ReadCloser
	stallTimeout time.Duration
	watchdog     *time.Timer
	stalled      atomic.Bool
	cancel       context.CancelFunc // releases the request context
	finish       func()             // idempotent; frees the fetch slot
}

func (f *fetchReader) Read(p []byte) (int, error) {
	n, err := f.inner.Read(p)
	if n > 0 {
		// Benign race: the watchdog can fire between the read returning and
		// this Reset. The request context is then already cancelled, so the
		// next Read fails; the Reset only re-schedules a timer whose work is
		// done.
		f.watchdog.Reset(f.stallTimeout)
	}
	if err != nil {
		if f.stalled.Load() && !errors.Is(err, io.EOF) {
			// Both errors stay matchable: errStalled identifies the watchdog
			// kill, while the wrapped cause keeps the transport's own chain
			// (e.g. context.Canceled) intact.
			err = fmt.Errorf("%w: no data received for %v: %w", errStalled, f.stallTimeout, err)
		}
		f.terminate()
	}
	return n, err
}

func (f *fetchReader) Close() error {
	err := f.inner.Close()
	f.terminate()
	return err
}

// terminate ends the fetch lifecycle: the watchdog stops, the request context
// is released, and the fetch slot is freed. Reached from every terminal event
// of the stream - EOF, a read error, or Close - and safe to reach twice.
// On the EOF path the inner reader has already promoted the download, so the
// slot outlives promotion but not the parse of what the scanner has buffered.
func (f *fetchReader) terminate() {
	f.watchdog.Stop()
	f.cancel()
	f.finish()
}
