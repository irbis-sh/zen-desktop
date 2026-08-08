package localcdn

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/httprewrite"
	"golang.org/x/net/html"
)

// URLMatcher reports whether the given URL should have its integrity and
// crossorigin attributes stripped.
type URLMatcher func(rawURL string) bool

// FilterHTML rewrites an HTML response, removing integrity and crossorigin
// attributes from <script>, <link rel="stylesheet">, and <style> tags whose
// src/href URL matches the matcher.
//
// The response body is decoded by httprewrite.StreamRewrite before rewriting
// and the result is sent to the client as uncompressed chunked content, which
// matches the existing injection pipeline.
func FilterHTML(res *http.Response, match URLMatcher) error {
	return httprewrite.StreamRewrite(res, func(original io.ReadCloser, modified *io.PipeWriter) {
		defer original.Close()
		defer modified.Close()

		z := html.NewTokenizer(original)
		for {
			switch token := z.Next(); token {
			case html.ErrorToken:
				modified.CloseWithError(z.Err())
				return
			case html.StartTagToken:
				name, _ := z.TagName()
				if bytes.Equal(name, []byte("script")) || bytes.Equal(name, []byte("link")) || bytes.Equal(name, []byte("style")) {
					if rewritten, ok := rewriteTag(z.Raw(), z, match); ok {
						modified.Write(rewritten)
						continue
					}
				}
				modified.Write(z.Raw())
			default:
				modified.Write(z.Raw())
			}
		}
	})
}

// rewriteTag consumes the tokenizer's tag attributes and returns a copy of the
// raw tag without integrity/crossorigin attributes when the tag references a
// matching resource.
func rewriteTag(raw []byte, z *html.Tokenizer, match URLMatcher) ([]byte, bool) {
	lower := bytes.ToLower(raw)
	if !bytes.Contains(lower, []byte("integrity")) && !bytes.Contains(lower, []byte("crossorigin")) {
		consumeAttributes(z)
		return nil, false
	}

	var resourceURL string
	for {
		key, val, more := z.TagAttr()
		switch strings.ToLower(string(key)) {
		case "src", "href":
			resourceURL = string(val)
		}
		if !more {
			break
		}
	}
	if resourceURL == "" || !match(resourceURL) {
		return nil, false
	}

	return stripAttributes(raw, "integrity", "crossorigin"), true
}

// consumeAttributes advances the tokenizer past a tag's attributes.
func consumeAttributes(z *html.Tokenizer) {
	for {
		_, _, more := z.TagAttr()
		if !more {
			return
		}
	}
}

// stripAttributes removes the named attributes (case-insensitively) from a
// single HTML start tag, preserving all other bytes.
func stripAttributes(raw []byte, names ...string) []byte {
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[strings.ToLower(name)] = struct{}{}
	}

	var out bytes.Buffer
	out.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		if isSpace(raw[i]) {
			nameStart, nameLen := attrNameAt(raw, i+1)
			if nameLen > 0 {
				if _, ok := nameSet[strings.ToLower(string(raw[nameStart:nameStart+nameLen]))]; ok {
					i = skipAttribute(raw, nameStart+nameLen) - 1
					continue
				}
			}
		}
		out.WriteByte(raw[i])
	}
	return out.Bytes()
}

// isSpace reports whether b is HTML whitespace.
func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

// attrNameAt returns the start and length of an HTML attribute name beginning
// at position i. It returns a zero length when no valid attribute starts there.
func attrNameAt(raw []byte, i int) (start, length int) {
	if i >= len(raw) {
		return 0, 0
	}
	start = i
	for i < len(raw) {
		c := raw[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == ':' || c == '_' {
			i++
			continue
		}
		break
	}
	length = i - start
	if length == 0 {
		return 0, 0
	}
	// The name must be followed by whitespace, '=', '/', '>', or EOF.
	if i < len(raw) {
		after := raw[i]
		if !isSpace(after) && after != '=' && after != '/' && after != '>' {
			return 0, 0
		}
	}
	return start, length
}

// skipAttribute returns the index just past an attribute's value (or past its
// name when it has no value), given the index just past the name.
func skipAttribute(raw []byte, i int) int {
	for i < len(raw) && isSpace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != '=' {
		return i
	}
	i++
	for i < len(raw) && isSpace(raw[i]) {
		i++
	}
	if i >= len(raw) {
		return i
	}
	if raw[i] == '"' || raw[i] == '\'' {
		quote := raw[i]
		i++
		for i < len(raw) && raw[i] != quote {
			i++
		}
		if i < len(raw) {
			i++
		}
		return i
	}
	for i < len(raw) && !isSpace(raw[i]) && raw[i] != '>' {
		i++
	}
	return i
}
