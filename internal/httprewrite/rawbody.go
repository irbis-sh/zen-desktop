package httprewrite

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/hashicorp/go-multierror"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/net/html/charset"
)

// StreamRewriteHTML decompresses the body of a text/html response and passes it to the processor
// for transformation in a streaming fashion. The processor writes the transformed output to dst
// and is responsible for closing both streams.
//
// The processor gets the body in its original character encoding, so the browser decodes the page
// as it would without Zen. UTF-16 is the exception: it is converted to UTF-8 and Content-Type is
// relabelled. Processors must only insert ASCII and find insertion points by matching ASCII markup.
// Before parsing or matching an inline script, they must check it with [IsUTF8Script].
func StreamRewriteHTML(res *http.Response, processor func(src io.ReadCloser, dst *io.PipeWriter)) error {
	body, err := decompressBody(res)
	if err != nil {
		return err
	}

	// Browsers only take UTF-16 from a byte order mark or the header, never from <meta>,
	// so a short preview is enough. It's 3 bytes so that it also holds a UTF-8 byte order mark,
	// which wins over a contradicting header, as in browsers.
	// A longer preview would hold back the start of the page until it arrives.
	bufBody := bufio.NewReader(body)
	preview, err := bufBody.Peek(3)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read body preview: %v", err)
	}

	var src io.Reader = bufBody
	// In UTF-16, ASCII characters take two bytes each, so the processors could neither insert ASCII
	// nor find tags. Converting to UTF-8 is safe: browsers submit forms from UTF-16 pages as UTF-8
	// anyway, so servers get the same bytes.
	if enc, name, _ := charset.DetermineEncoding(preview, res.Header.Get("Content-Type")); name == "utf-16le" || name == "utf-16be" {
		src = enc.NewDecoder().Reader(bufBody)
		res.Header.Set("Content-Type", "text/html; charset=utf-8")
	}

	reader, writer := io.Pipe()

	go processor(struct {
		io.Reader
		io.Closer
	}{src, body}, writer)

	res.Body = reader
	// Content-Length cannot be determined ahead of time.
	// To get around this, the response is chunked to allow for HTTP connection reuse without having to TCP FIN terminate each connection.
	res.ContentLength = -1
	res.Header.Del("Content-Length")
	res.Header.Del("Content-Encoding")
	res.TransferEncoding = []string{"chunked"}
	return nil
}

// BufferRewrite reads and decompresses the HTTP response body, applies a transformation
// to it using the provided processor function, and replaces the original body
// with the transformed version.
//
// The processor gets the body in its original character encoding, and Content-Type is left alone.
func BufferRewrite(res *http.Response, processor func(src []byte) []byte) error {
	body, err := decompressBody(res)
	if err != nil {
		return err
	}
	defer body.Close()

	rawBody, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read body: %v", err)
	}

	processedBody := processor(rawBody)

	res.Body = io.NopCloser(bytes.NewReader(processedBody))
	res.ContentLength = int64(len(processedBody))
	res.Header.Set("Content-Length", fmt.Sprint(len(processedBody)))
	res.TransferEncoding = nil
	res.Header.Del("Content-Encoding")
	return nil
}

// IsUTF8Script reports whether script is valid UTF-8 and contains no ESC (0x1B) bytes.
// Rewriters that parse or match JS as bytes must only change scripts that pass. In legacy
// multi-byte encodings, part of a character can read as JS syntax, such as the backslash (0x5C)
// in Shift_JIS "表" (0x95 0x5C). ESC rules out ISO-2022-JP, which is 7-bit and would otherwise pass.
func IsUTF8Script(script []byte) bool {
	return utf8.Valid(script) && bytes.IndexByte(script, 0x1b) == -1
}

// decompressBody returns a reader of the decompressed response body.
// Closing it closes the original body.
func decompressBody(res *http.Response) (io.ReadCloser, error) {
	encoding := res.Header.Get("Content-Encoding")
	if encoding == "" {
		return res.Body, nil
	}
	decompressed, err := decompressReader(res.Body, encoding)
	if err != nil {
		return nil, fmt.Errorf("create decompressed reader for encoding %q: %v", encoding, err)
	}
	return struct {
		io.Reader
		io.Closer
	}{
		decompressed,
		&multiCloser{[]io.Closer{decompressed, res.Body}},
	}, nil
}

// zstdMaxWindow is 8 MiB, the largest window RFC 9659 allows in HTTP and the limit Chromium and Firefox
// enforce. A frame sets its own window and the decoder allocates it up front, so without a limit a
// few bytes can cost hundreds of MiB.
const zstdMaxWindow = 8 << 20

// decompressReader decompresses a reader using the specified compression algorithm.
// It does not decompress data encoded with multiple algorithms.
func decompressReader(reader io.ReadCloser, compressionAlg string) (io.ReadCloser, error) {
	// Reference: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Encoding
	switch strings.ToLower(compressionAlg) {
	case "gzip":
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %v", err)
		}
		return gzipReader, nil
	case "deflate":
		return flate.NewReader(reader), nil
	case "br":
		return io.NopCloser(brotli.NewReader(reader)), nil
	case "zstd":
		// One goroutine, so decoding runs inside Read. The default starts goroutines that keep running until
		// the body ends or the decoder is closed, and Close waits for them, so it hangs while upstream stalls.
		// Bodies arrive slower than they decode, so the extra goroutines wouldn't speed anything up.
		zstdReader, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(zstdMaxWindow))
		if err != nil {
			return nil, fmt.Errorf("create zstd reader: %v", err)
		}
		return zstdReader.IOReadCloser(), nil
	default:
		return nil, errors.New("unsupported encoding")
	}
}

// multiCloser wraps multiple io.Closers and ensures they are closed sequentially.
type multiCloser struct {
	closers []io.Closer
}

// Close iterates over each io.Closer and closes it, capturing any errors.
func (m *multiCloser) Close() error {
	var finalErr *multierror.Error
	for _, closer := range m.closers {
		if err := closer.Close(); err != nil {
			finalErr = multierror.Append(finalErr, err)
		}
	}
	return finalErr.ErrorOrNil()
}
