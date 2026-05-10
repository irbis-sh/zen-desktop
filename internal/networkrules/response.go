package networkrules

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
)

// CreateBlockResponse creates a response for a blocked request.
func (nr *NetworkRules) CreateBlockResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusForbidden,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Proto:      req.Proto,
	}
}

type BlockInfo struct {
	Rule          string
	FilterList    string
	WhitelistPort int
}

//go:embed blockpage.html
var blockPageTpl string

var blockTmpl = template.Must(template.New("block").Parse(blockPageTpl))

func (nr *NetworkRules) CreateBlockPageResponse(req *http.Request, appliedRules []rule.Rule, whitelistPort int) (*http.Response, error) {
	var rawRule, filterList string
	if len(appliedRules) > 0 {
		// ModifyReq currently returns at most one rule when shouldBlock is true.
		// If this changes in the future, this logic may need to be updated.
		r := appliedRules[0]
		rawRule = r.RawRule
		if r.FilterName != nil {
			filterList = *r.FilterName
		}
	}

	var buf bytes.Buffer
	err := blockTmpl.Execute(&buf, BlockInfo{
		Rule:          rawRule,
		FilterList:    filterList,
		WhitelistPort: whitelistPort,
	})
	if err != nil {
		return nil, fmt.Errorf("parse html template: %w", err)
	}

	h := make(http.Header)
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")

	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        http.StatusText(http.StatusOK),
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Header:        h,
		ContentLength: int64(buf.Len()),
		Body:          io.NopCloser(&buf),
		Request:       req,
	}, nil
}

// CreateRedirectResponse creates a response for a redirected request.
func (nr *NetworkRules) CreateRedirectResponse(req *http.Request, to string) *http.Response {
	header := http.Header{
		"Location": []string{to},
	}
	if origin := req.Header.Get("Origin"); origin != "" && origin != "null" {
		header.Set("Access-Control-Allow-Origin", origin)
	}

	return &http.Response{
		// The use of 307 Temporary Redirect instead of 308 Permanent Redirect is intentional.
		// 308's can be cached by clients, which might cause issues in cases of erroneous redirects, changing filter rules, etc.
		StatusCode: http.StatusTemporaryRedirect,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Proto:      req.Proto,
		Header:     header,
	}
}
