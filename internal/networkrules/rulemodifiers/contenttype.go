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
	// contentTypeMap maps Content-Type MIME types to corresponding content type modifiers.
	contentTypeMap = map[string]string{
		"text/css":                      "stylesheet",
		"text/javascript":               "script",
		"application/javascript":        "script",
		"image":                         "image",
		"audio":                         "media",
		"video":                         "media",
		"font":                          "font",
		"application/x-shockwave-flash": "object",
	}
)

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
	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		return false
	}

	// strip parameters like charset
	mimeType, _, _ := strings.Cut(contentType, ";")
	mimeType = strings.TrimSpace(mimeType)
	mimeType = strings.ToLower(mimeType)

	normalized, known := mapResponseContentTypeToModifier(mimeType)
	if m.contentType == "other" {
		if m.inverted {
			return known
		}

		return !known
	}

	if m.inverted {
		return normalized != m.contentType
	}

	return normalized == m.contentType
}

func mapResponseContentTypeToModifier(mimeType string) (string, bool) {
	if mapped, ok := contentTypeMap[mimeType]; ok {
		return mapped, true
	}

	// check top-level type
	before, _, _ := strings.Cut(mimeType, "/")
	if top, ok := contentTypeMap[before]; ok {
		return top, true
	}

	return "", false
}

func (m *ContentTypeModifier) Cancels(modifier Modifier) bool {
	other, ok := modifier.(*ContentTypeModifier)
	if !ok {
		return false
	}

	return other.inverted == m.inverted && other.contentType == m.contentType
}
