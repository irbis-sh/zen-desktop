package filterliststore

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
)

// errIncompleteBody reports that a body ended cleanly but carried fewer bytes
// than its Content-Length declared. Go's transport already turns a short
// length-delimited body into io.ErrUnexpectedEOF and chunked, gzip and HTTP/2
// framings self-verify, so this is defence in depth. The known hole is
// HTTP/1.0-style close-delimited bodies, which declare no length and cannot be
// verified at all. The sentinel wraps io.ErrUnexpectedEOF so consumers can
// match both transport-detected and store-detected truncation with a single
// errors.Is check.
var errIncompleteBody = fmt.Errorf("incomplete response body: %w", io.ErrUnexpectedEOF)

// errEmptyBody reports a body that ended cleanly without a single byte.
// A filter list is never empty, so this is a broken origin or intermediary;
// promoting it would install an empty authoritative copy.
var errEmptyBody = errors.New("empty response body")

// cachingReader streams a response body to the caller while copying it to a
// temporary file. When the body reaches a verified EOF, the completed copy is
// handed to onComplete for caching. A mid-body error or a Close before EOF
// abandons the copy, so a partial download is never cached.
//
// Unlike http.Response.Body, a cachingReader does not support Close
// concurrently with Read: all use must stay on one goroutine. Asynchronous
// cancellation (e.g. a stall watchdog) must cancel the request context instead
// of calling Close.
type cachingReader struct {
	body io.ReadCloser

	// tempFile is nil when there is nothing left to cache: creation failed,
	// the copy was abandoned, or completion already happened.
	tempFile *os.File

	// onComplete is called at most once, with the verified, fully downloaded
	// file. Ownership of the file transfers to the callback, which keeps the
	// reader free of any knowledge of what completion means.
	onComplete func(*os.File)

	contentLength int64 // from the response; -1 when unknown
	bytesRead     int64
}

func (r *cachingReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.bytesRead += int64(n)
		if r.tempFile != nil {
			if _, werr := r.tempFile.Write(p[:n]); werr != nil {
				// Caching is best-effort: keep streaming to the caller.
				log.Printf("failed to write to temp file: %v", werr)
				r.abandon()
			}
		}
	}

	switch {
	case err == nil:
		return n, nil
	case errors.Is(err, io.EOF):
		if r.contentLength >= 0 && r.bytesRead != r.contentLength {
			r.abandon()
			return n, fmt.Errorf("%w: got %d bytes, expected %d", errIncompleteBody, r.bytesRead, r.contentLength)
		}
		if r.bytesRead == 0 {
			r.abandon()
			return n, errEmptyBody
		}
		if r.tempFile != nil {
			tempFile := r.tempFile
			r.tempFile = nil
			r.onComplete(tempFile)
		}
		return n, io.EOF
	default:
		// The download broke mid-body; the partial copy must not be cached.
		// The caller sees the error through this Read, e.g. via scanner.Err().
		r.abandon()
		return n, err
	}
}

// Close closes the network body. When EOF has not been reached yet, the caller
// is abandoning the download deliberately, which is not a fetch failure: the
// partial copy is discarded without an error.
func (r *cachingReader) Close() error {
	err := r.body.Close()
	r.abandon()
	return err
}

func (r *cachingReader) abandon() {
	if r.tempFile == nil {
		return
	}
	name := r.tempFile.Name()
	r.tempFile.Close()
	if err := os.Remove(name); err != nil {
		log.Printf("failed to remove temp file: %v", err)
	}
	r.tempFile = nil
}
