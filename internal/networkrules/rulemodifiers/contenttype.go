package rulemodifiers

import (
	"net/http"
)

type ContentTypeModifier struct {
	contentType string
	inverted    bool
}

var _ MatchingModifier = (*ContentTypeModifier)(nil)

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

func (m *ContentTypeModifier) ShouldMatchRes(_ *http.Response) bool {
	return false
}

func (m *ContentTypeModifier) Cancels(modifier Modifier) bool {
	other, ok := modifier.(*ContentTypeModifier)
	if !ok {
		return false
	}

	return other.inverted == m.inverted && other.contentType == m.contentType
}
