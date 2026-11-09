package httprewrite_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/irbis-sh/zen-desktop/internal/httprewrite"
	"github.com/klauspost/compress/zstd"
)

func TestBufferRewriteKeepsEncoding(t *testing.T) {
	t.Parallel()

	// The first 1024 bytes are ASCII, so a guess from them would pick Windows-1252.
	original := `{"text":"` + strings.Repeat("x", 1100) + `é 😀"}`
	res := newGzipHTTPResponse(t, "application/json", []byte(original))

	err := httprewrite.BufferRewrite(res, func(src []byte) []byte { return src })
	if err != nil {
		t.Fatalf("BufferRewrite error: %v", err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != original {
		t.Errorf("body = %q, want %q", body, original)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if ce := res.Header.Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want none", ce)
	}
}

func TestIsUTF8Script(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		want   bool
	}{
		{"ASCII", `var a = "b";`, true},
		{"UTF-8", `var a = "表示 😀";`, true},
		{"GBK", "var a = \"\xd6\xd0\xce\xc4\";", false},
		{"Shift_JIS with a backslash second byte", "var a = \"\x95\x5c\";", false},
		{"ISO-2022-JP", "var a = \"\x1b$BI=<(\x1b(B\";", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := httprewrite.IsUTF8Script([]byte(tt.script)); got != tt.want {
				t.Errorf("IsUTF8Script(%q) = %v, want %v", tt.script, got, tt.want)
			}
		})
	}
}

func TestZstdRoundTrip(t *testing.T) {
	t.Parallel()

	page := []byte("<html><head></head><body>" + strings.Repeat("<p>Hello, world!</p>", 10000) + "</body></html>")
	frame := zstdStreamFrame(t, page, 8<<20)

	t.Run("StreamRewriteHTML", func(t *testing.T) {
		t.Parallel()

		res := newZstdHTTPResponse(frame)
		err := httprewrite.StreamRewriteHTML(res, func(src io.ReadCloser, dst *io.PipeWriter) {
			defer src.Close()
			_, err := io.Copy(dst, src)
			dst.CloseWithError(err)
		})
		if err != nil {
			t.Fatalf("StreamRewriteHTML error: %v", err)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !bytes.Equal(body, page) {
			t.Errorf("body differs from the original page")
		}
	})

	t.Run("BufferRewrite", func(t *testing.T) {
		t.Parallel()

		res := newZstdHTTPResponse(frame)
		if err := httprewrite.BufferRewrite(res, func(src []byte) []byte { return src }); err != nil {
			t.Fatalf("BufferRewrite error: %v", err)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !bytes.Equal(body, page) {
			t.Errorf("body differs from the original page")
		}
	})
}

func TestZstdWindowLimit(t *testing.T) {
	t.Parallel()

	streamBody := bytes.Repeat([]byte("<p>Hello, world!</p>"), 1<<16)

	// Single-segment frames have no window field, so their content size acts as the window.
	singleSegmentBody := bytes.Repeat([]byte("a"), 8<<20)

	// A frame that declares a 512 MiB window. Its 128 KiB RLE block makes it valid, so only the window can
	// make it fail.
	tinyFrame := []byte{
		0x28, 0xb5, 0x2f, 0xfd, // Magic number.
		0x00,             // Frame header descriptor: no content size, no checksum, not single segment.
		19 << 3,          // Window descriptor: exponent 19, mantissa 0, so a window of 1 << (10+19).
		0x03, 0x00, 0x10, // Block header: last block, RLE, size 1 << 17.
		'a', // The byte to repeat.
	}

	tests := []struct {
		name    string
		frame   []byte
		wantErr bool
	}{
		{"8 MiB window", zstdStreamFrame(t, streamBody, 8<<20), false},
		{"16 MiB window", zstdStreamFrame(t, streamBody, 16<<20), true},
		{"8 MiB single segment", zstdSingleSegmentFrame(t, singleSegmentBody), false},
		{"8 MiB + 1 byte single segment", zstdSingleSegmentFrame(t, append(singleSegmentBody, 'a')), true},
		{"tiny frame declaring a 512 MiB window", tinyFrame, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := newZstdHTTPResponse(tt.frame)
			err := httprewrite.BufferRewrite(res, func(src []byte) []byte { return src })
			if (err != nil) != tt.wantErr {
				t.Fatalf("BufferRewrite error = %v, want error: %v", err, tt.wantErr)
			}
		})
	}
}

func TestZstdStopReadingEarly(t *testing.T) {
	t.Parallel()

	// AppendHTMLHeadContents copies the rest of the page once it has found </head>, so with </head>
	// near the start of a large page, the decoder is mid-body when the client stops reading.
	// The page is compressed outside the bubble, because the encoder starts goroutines of its own.
	page := append([]byte("<html><head></head><body>"), genAlphanumByteArray(4<<20)...)
	frame := zstdStreamFrame(t, page, 8<<20)

	// synctest.Test panics if any goroutine started in the bubble is still blocked when it returns.
	synctest.Test(t, func(t *testing.T) {
		res := newZstdHTTPResponse(frame)
		if err := httprewrite.AppendHTMLHeadContents(res, []byte("<script></script>")); err != nil {
			t.Fatalf("AppendHTMLHeadContents error: %v", err)
		}
		if _, err := io.ReadFull(res.Body, make([]byte, 1024)); err != nil {
			t.Fatalf("read body: %v", err)
		}
		res.Body.Close()
	})
}

func newZstdHTTPResponse(frame []byte) *http.Response {
	res := newHTTPResponse(frame)
	res.Header.Set("Content-Encoding", "zstd")
	return res
}

// zstdStreamFrame compresses body as a stream and checks that the frame declares the given window.
// The encoder only writes the requested window for bodies over about 128 KB. Smaller bodies get a
// smaller window or a single-segment frame.
func zstdStreamFrame(t *testing.T, body []byte, window int) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf, zstd.WithWindowSize(window))
	if err != nil {
		t.Fatalf("create zstd writer: %v", err)
	}
	if _, err := zw.Write(body); err != nil {
		t.Fatalf("zstd write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zstd close: %v", err)
	}

	var h zstd.Header
	if err := h.Decode(buf.Bytes()); err != nil {
		t.Fatalf("decode zstd header: %v", err)
	}
	if h.SingleSegment || h.WindowSize != uint64(window) { // #nosec G115 -- window is a positive size from the test
		t.Fatalf("frame has single segment %v and window %d, want a stream frame with window %d", h.SingleSegment, h.WindowSize, window)
	}
	return buf.Bytes()
}

// zstdSingleSegmentFrame compresses body as a single-segment frame and checks that it is one.
func zstdSingleSegmentFrame(t *testing.T, body []byte) []byte {
	t.Helper()

	zw, err := zstd.NewWriter(nil, zstd.WithSingleSegment(true))
	if err != nil {
		t.Fatalf("create zstd writer: %v", err)
	}
	defer zw.Close()
	frame := zw.EncodeAll(body, nil)

	var h zstd.Header
	if err := h.Decode(frame); err != nil {
		t.Fatalf("decode zstd header: %v", err)
	}
	if !h.SingleSegment || h.FrameContentSize != uint64(len(body)) {
		t.Fatalf("frame has single segment %v and content size %d, want a single-segment frame of %d bytes", h.SingleSegment, h.FrameContentSize, len(body))
	}
	return frame
}
