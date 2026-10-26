// Package contenttype classifies requests and responses by the content types
// that rules name with modifiers such as $script and $image.
package contenttype

import (
	"net/http"
	"strings"
)

// Type is one content type. Each Type other than Unknown is a single bit, so
// Mask(t) converts it to a one-type Mask.
type Type uint16

const (
	Document Type = 1 << iota
	Subdocument
	Script
	Stylesheet
	Image
	Font
	Media
	Object
	XMLHTTPRequest
	WebSocket
	Ping
	Other

	// typeEnd is one past the highest type, so All includes every type above. Keep it last.
	typeEnd
)

// Unknown is the type of a request that carries nothing to classify it by, such
// as one from a native app. It has no bit, so only the zero Mask matches it.
const Unknown Type = 0

// Mask is a set of Types. The zero value means any type, Unknown included.
type Mask uint16

// All is every Type except Unknown.
const All = Mask(typeEnd - 1)

// Matches reports whether t is in m. As a special case, the zero Mask matches every
// Type, including [Unknown].
func (m Mask) Matches(t Type) bool {
	return m == 0 || m&Mask(t) != 0
}

var (
	// modifierTypes maps content type modifier names to their types.
	modifierTypes = map[string]Type{
		"document":       Document,
		"doc":            Document,
		"xmlhttprequest": XMLHTTPRequest,
		"xhr":            XMLHTTPRequest,
		"font":           Font,
		"subdocument":    Subdocument,
		"image":          Image,
		"object":         Object,
		"script":         Script,
		"stylesheet":     Stylesheet,
		"media":          Media,
		"websocket":      WebSocket,
		"ping":           Ping,
		"other":          Other,
	}
	// secFetchDestMap maps Sec-Fetch-Dest header values to their types.
	secFetchDestMap = map[string]Type{
		"document":  Document,
		"empty":     XMLHTTPRequest,
		"font":      Font,
		"frame":     Subdocument,
		"iframe":    Subdocument,
		"image":     Image,
		"object":    Object,
		"script":    Script,
		"style":     Stylesheet,
		"audio":     Media,
		"track":     Media,
		"video":     Media,
		"websocket": WebSocket,
	}
	// mimeTypeMap maps Content-Type MIME types, or their top-level part, to their types.
	mimeTypeMap = map[string]Type{
		"text/css":                      Stylesheet,
		"text/ping":                     Ping,
		"text/javascript":               Script,
		"application/javascript":        Script,
		"image":                         Image,
		"audio":                         Media,
		"video":                         Media,
		"font":                          Font,
		"application/x-shockwave-flash": Object,
	}
)

// ParseModifier returns the type that a content type modifier name, such as
// "script" or "xhr", stands for, and whether name is one.
func ParseModifier(name string) (Type, bool) {
	t, ok := modifierTypes[name]
	return t, ok
}

// FromRequest classifies req by its Fetch Metadata and ping headers, or by its
// WebSocket upgrade headers if it has no Fetch Metadata. It returns [Other] for an
// unrecognised Sec-Fetch-Dest value and [Unknown] if nothing classifies req.
func FromRequest(req *http.Request) Type {
	hasPingHeaders := req.Header.Get("Ping-To") != "" || req.Header.Get("Ping-From") != ""
	if hasPingHeaders && mimeType(req.Header) == "text/ping" {
		return Ping
	}

	secFetchDest := req.Header.Get("Sec-Fetch-Dest")
	if secFetchDest == "" {
		if headerContains(req.Header, "Connection", "upgrade") && headerContains(req.Header, "Upgrade", "websocket") {
			return WebSocket
		}
		return Unknown
	}
	if t, ok := secFetchDestMap[secFetchDest]; ok {
		return t
	}
	return Other
}

// FromResponse classifies res by its request, and by its Content-Type only if
// the request gives no type.
func FromResponse(req *http.Request, res *http.Response) Type {
	if t := FromRequest(req); t != Unknown {
		return t
	}

	mt := mimeType(res.Header)
	if mt == "" {
		return Unknown
	}
	if t, ok := mimeTypeMap[mt]; ok {
		return t
	}
	topLevel, _, _ := strings.Cut(mt, "/")
	if t, ok := mimeTypeMap[topLevel]; ok {
		return t
	}
	return Other
}

func headerContains(h http.Header, name, value string) bool {
	for _, v := range h[name] {
		for s := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(s), value) {
				return true
			}
		}
	}
	return false
}

// mimeType returns the lowercased MIME type of the Content-Type header, without parameters.
func mimeType(header http.Header) string {
	mt, _, _ := strings.Cut(header.Get("Content-Type"), ";")
	return strings.ToLower(strings.TrimSpace(mt))
}
