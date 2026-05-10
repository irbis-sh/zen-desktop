package filterliststore

import (
	"io"
)

// notifyReadCloser is an io.ReadCloser that notifies a channel when it is closed.
type notifyReadCloser struct {
	// reader is the underlying io.ReadCloser being wrapped.
	reader io.ReadCloser
	// notifyCh gets sent a notification when the reader is closed.
	notifyCh chan<- struct{}
	// notifySent indicates whether the notification has already been sent.
	notifySent bool
	// errCh gets sent a notification if an error occurs during reading or closing.
	errCh chan<- struct{}
	// errSent indicates whether the error notification has already been sent.
	errSent bool
}

func newNotifyReadCloser(reader io.ReadCloser) (*notifyReadCloser, <-chan struct{}, <-chan struct{}) {
	// Buffered channels to avoid blocking.
	notifyCh := make(chan struct{}, 1)
	errCh := make(chan struct{}, 1)
	return &notifyReadCloser{
		reader:   reader,
		notifyCh: notifyCh,
		errCh:    errCh,
	}, notifyCh, errCh
}

func (nrc *notifyReadCloser) Read(p []byte) (int, error) {
	n, err := nrc.reader.Read(p)
	if err != nil && err != io.EOF && !nrc.errSent {
		nrc.errCh <- struct{}{}
		nrc.errSent = true
	}
	return n, err
}

func (nrc *notifyReadCloser) Close() error {
	err := nrc.reader.Close()
	if err != nil && !nrc.errSent {
		nrc.errCh <- struct{}{}
		nrc.errSent = true
	}
	if !nrc.notifySent {
		nrc.notifyCh <- struct{}{}
		nrc.notifySent = true
	}
	return err
}
