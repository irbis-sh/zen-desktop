package rulemodifiers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/httprewrite"
	"golang.org/x/net/html"
)

type ScrambleJSModifier struct {
	keys []*regexp.Regexp
}

var _ ReqResModifier = (*ScrambleJSModifier)(nil)

var scrambleJSRegex = regexp.MustCompile(`^scramblejs=(.+)$`)

func (s *ScrambleJSModifier) Parse(modifier string) error {
	match := scrambleJSRegex.FindStringSubmatch(modifier)
	if match == nil {
		return errors.New("invalid syntax")
	}

	rawKeys := strings.Split(match[1], "|")
	s.keys = make([]*regexp.Regexp, len(rawKeys))
	for i, key := range rawKeys {
		if len(key) == 0 {
			return errors.New("empty keys are not allowed")
		}

		re, err := regexp.Compile(regexp.QuoteMeta(key))
		if err != nil {
			return fmt.Errorf("compile key %q: %v", key, err)
		}
		s.keys[i] = re
	}

	return nil
}

func (*ScrambleJSModifier) ModifyReq(*http.Request) bool {
	return false
}

func (s *ScrambleJSModifier) ModifyRes(res *http.Response) (bool, error) {
	contentType := res.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false, nil
	}
	switch mediaType {
	case "text/html":
		if err := replaceInInlineHTML(res, s.keys); err != nil {
			return false, fmt.Errorf("replace in inline HTML: %v", err)
		}
		return true, nil
	case "text/javascript", "application/javascript":
		if err := replaceInJS(res, s.keys); err != nil {
			return false, fmt.Errorf("replace in JS: %v", err)
		}
		return true, nil
	}
	return false, nil
}

func (s *ScrambleJSModifier) Cancels(modifier Modifier) bool {
	other, ok := modifier.(*ScrambleJSModifier)
	if !ok {
		return false
	}

	if len(s.keys) != len(other.keys) {
		return false
	}
	for i := range s.keys {
		if s.keys[i] != other.keys[i] {
			return false
		}
	}

	return true
}

// replaceInInlineHTML replaces matched keys with unique random values in HTML responses.
func replaceInInlineHTML(res *http.Response, keys []*regexp.Regexp) error {
	return httprewrite.StreamRewrite(res, func(original io.ReadCloser, modified *io.PipeWriter) {
		defer original.Close()
		z := html.NewTokenizer(original)

	parseLoop:
		for {
			switch token := z.Next(); token {
			case html.ErrorToken:
				modified.CloseWithError(z.Err())
				break parseLoop
			case html.StartTagToken:
				modified.Write(z.Raw())
				if name, _ := z.TagName(); !bytes.Equal(name, []byte("script")) {
					continue parseLoop
				}
				next := z.Next()
				if next != html.TextToken {
					modified.Write(z.Raw())
					continue parseLoop
				}
				script := z.Raw()
				newScript := replaceKeys(script, keys)
				modified.Write(newScript)
			default:
				modified.Write(z.Raw())
			}
		}
	})
}

// replaceInJS replaces matched keys with unique random values in JS responses.
func replaceInJS(res *http.Response, keys []*regexp.Regexp) error {
	return httprewrite.BufferRewrite(res, func(src []byte) []byte {
		return replaceKeys(src, keys)
	})
}

// replaceKeys replaces each occurrence of keys with unique random strings.
func replaceKeys(script []byte, keys []*regexp.Regexp) []byte {
	// anfragment: This is potentially inefficient when running on large script arrays/multiple keys
	// due to multiple passes/script array copies. Preprocess keys into an Aho-Corasick or build a single regexp
	// if this gets determined as a bottleneck.
	for _, key := range keys {
		script = key.ReplaceAllFunc(script, func(_ []byte) []byte {
			return genRandomIdent(10)
		})
	}
	return script
}

// genRandomIdent returns a string that has a random alpha character in position 0
// and random alphanumerical characters in every other position.
func genRandomIdent(length int) []byte {
	const alphaCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const fullCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	if length == 0 {
		return []byte{}
	}

	b := make([]byte, length)

	// This makes the modifier compatible with replacing identifiers, which in JS should not begin with a numerical character.
	b[0] = alphaCharset[rand.IntN(len(alphaCharset))] // #nosec G404 -- Not used for security-related purposes
	for i := 1; i < length; i++ {
		b[i] = fullCharset[rand.IntN(len(fullCharset))] // #nosec G404 -- Not used for security-related purposes
	}
	return b
}
