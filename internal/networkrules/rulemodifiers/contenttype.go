package rulemodifiers

import (
	"net/http"
	"strings"
)

type ContentTypeModifier struct {
	contentType string
	inverted    bool
}

var _ ConditionModifier = (*ContentTypeModifier)(nil)

var (
	// secFetchDestMap maps Sec-Fetch-Dest header values to corresponding content type modifiers.
	secFetchDestMap = map[string]string{
		"empty":  "xmlhttprequest",
		"font":   "font",
		"frame":  "subdocument",
		"iframe": "subdocument",
		"image":  "image",
		"object": "object",
		"script": "script",
		"style":  "stylesheet",
		"audio":  "media",
		"track":  "media",
		"video":  "media",
	}
	// aliases maps content type aliases to their canonical names.
	aliases = map[string]string{
		"css": "stylesheet",
		"xhr": "xmlhttprequest",
	}
	// contentTypeMimeMap maps content type modifiers to MIME type matching functions
	// for response-side matching via the Content-Type header.
	contentTypeMimeMap = map[string]func(string) bool{
		"stylesheet": func(mime string) bool {
			return mime == "text/css"
		},
		"script": func(mime string) bool {
			return mime == "text/javascript" ||
				mime == "application/javascript" ||
				mime == "application/ecmascript"
		},
		"image": func(mime string) bool {
			return strings.HasPrefix(mime, "image/")
		},
		"media": func(mime string) bool {
			return strings.HasPrefix(mime, "audio/") ||
				strings.HasPrefix(mime, "video/")
		},
		"font": func(mime string) bool {
			return strings.HasPrefix(mime, "font/")
		},
		"subdocument": func(mime string) bool {
			return mime == "text/html"
		},
		"object": func(mime string) bool {
			// Common plugin content types embedded via <object> or <embed>.
			return mime == "application/x-shockwave-flash" ||
				mime == "application/x-java-applet" ||
				mime == "application/x-silverlight-app"
		},
	}
)

// mimeFromResponse extracts the MIME type from a response's Content-Type header,
// stripping any parameters (e.g., charset).
func mimeFromResponse(res *http.Response) string {
	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		return ""
	}
	mimeType, _, _ := strings.Cut(contentType, ";")
	return strings.TrimSpace(mimeType)
}

func (m *ContentTypeModifier) Parse(modifier string) error {
	if modifier[0] == '~' {
		m.inverted = true
		modifier = modifier[1:]
	}
	if canonical, ok := aliases[modifier]; ok {
		modifier = canonical
	}
	m.contentType = modifier
	return nil
}

func (m *ContentTypeModifier) ShouldMatchReq(req *http.Request) bool {
	secFetchDest := req.Header.Get("Sec-Fetch-Dest")
	if secFetchDest == "" {
		return false
	}
	contentType, ok := secFetchDestMap[secFetchDest]
	if m.contentType == "other" {
		if m.inverted {
			return ok
		}
		return !ok
	}
	if m.inverted {
		return contentType != m.contentType
	}
	return contentType == m.contentType
}

func (m *ContentTypeModifier) ShouldMatchRes(res *http.Response) bool {
	mimeType := mimeFromResponse(res)
	if mimeType == "" {
		return false
	}

	// xmlhttprequest is request-only; never matches on response.
	if m.contentType == "xmlhttprequest" {
		return false
	}

	// For "other": match when no known MIME type matches.
	if m.contentType == "other" {
		for _, matchFn := range contentTypeMimeMap {
			if matchFn(mimeType) {
				return m.inverted // known type found → not "other"
			}
		}
		return !m.inverted // no known type found → "other"
	}

	matchFn, ok := contentTypeMimeMap[m.contentType]
	if !ok {
		return false
	}

	matched := matchFn(mimeType)
	if m.inverted {
		return !matched
	}
	return matched
}

func (m *ContentTypeModifier) Cancels(modifier Modifier) bool {
	other, ok := modifier.(*ContentTypeModifier)
	if !ok {
		return false
	}

	return other.inverted == m.inverted && other.contentType == m.contentType
}
