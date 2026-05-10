package csp

import (
	"io"
	"net/http"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/httprewrite"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// patchMetaCSPsBatch mutates HTML <meta> tags for multiple CSP operations in a single pass.
func patchMetaCSPsBatch(res *http.Response, operations []PatchOperation) error {
	if res.Body == nil || res.Body == http.NoBody {
		return nil
	}

	err := httprewrite.StreamRewrite(res, func(src io.ReadCloser, dst *io.PipeWriter) {
		defer src.Close()

		z := html.NewTokenizer(src)
		for {
			tt := z.Next()
			switch tt {
			case html.ErrorToken:
				dst.CloseWithError(z.Err())
				return

			case html.StartTagToken, html.SelfClosingTagToken:
				raw := append([]byte{}, z.Raw()...) // z.Token() modifies the underlying buffer, so we need to make a copy
				tok := z.Token()

				if tok.DataAtom != atom.Meta {
					dst.Write(raw)
					continue
				}

				var hasCSP bool
				contentInd := -1 // Track the index of the content= attribute

				for i, a := range tok.Attr {
					if strings.EqualFold(a.Key, "http-equiv") &&
						strings.EqualFold(a.Val, "content-security-policy") {
						hasCSP = true
					}

					if strings.EqualFold(a.Key, "content") && contentInd == -1 {
						contentInd = i
					}
				}

				if !hasCSP || contentInd == -1 || tok.Attr[contentInd].Val == "" {
					dst.Write(raw)
					continue
				}

				// Apply all operations to this meta tag's CSP
				var changed bool
				contentVal := tok.Attr[contentInd].Val
				patchedContent := contentVal

				for _, op := range operations {
					patched, patchChanged := patchPolicies([]string{patchedContent}, op.Nonce, op.Kind, op.ResourceURL)
					if patchChanged {
						patchedContent = patched[0]
						changed = true
					}
				}

				if !changed {
					dst.Write(raw)
					continue
				}

				tok.Attr[contentInd].Val = patchedContent
				dst.Write([]byte(tok.String()))

			default:
				dst.Write(z.Raw())
			}
		}
	})

	return err
}
