package asset

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/irbis-sh/zen-desktop/internal/asset/cosmetic"
	"github.com/irbis-sh/zen-desktop/internal/asset/cssrule"
	"github.com/irbis-sh/zen-desktop/internal/asset/extendedcss"
	"github.com/irbis-sh/zen-desktop/internal/asset/jsrule"
	"github.com/irbis-sh/zen-desktop/internal/asset/scriptlet"
	"github.com/irbis-sh/zen-desktop/internal/csp"
	"github.com/irbis-sh/zen-desktop/internal/httprewrite"
)

const (
	cosmeticCSSPath = "/cosmetic.css"
	cssRulePath     = "/cssrule.css"
	scriptletsPath  = "/scriptlets.js"
	extendedCSSPath = "/extendedcss.js"
	jsRulePath      = "/jsrule.js"
)

type kind int

const (
	scriptlets kind = iota
	jsRule
	extendedCSS
	cosmeticCSS
	cssRule
)

// Engine handles rule ingestion, HTML injection, and asset resolution.
type Engine struct {
	scriptlets  *scriptlet.Injector
	cosmetic    *cosmetic.Injector
	cssRules    *cssrule.Injector
	jsRules     *jsrule.Injector
	extendedCSS *extendedcss.Injector

	scriptletsURL  string
	jsRuleURL      string
	extendedCSSURL string
	cosmeticCSSURL string
	cssRuleCSSURL  string
}

// NewEngine constructs an Engine with default bundles and stores.
func NewEngine(assetServerPort int) (*Engine, error) {
	scriptlets, err := scriptlet.NewInjectorWithDefaults()
	if err != nil {
		return nil, fmt.Errorf("create scriptlets injector: %w", err)
	}
	extendedCSS, err := extendedcss.NewInjectorWithDefaults()
	if err != nil {
		return nil, fmt.Errorf("create extended css injector: %w", err)
	}
	assetServerURL, err := url.Parse("https://" + net.JoinHostPort(host, strconv.Itoa(assetServerPort)))
	if err != nil {
		return nil, fmt.Errorf("parse asset server url: %w", err)
	}

	return &Engine{
		scriptlets:  scriptlets,
		cosmetic:    cosmetic.NewInjector(),
		cssRules:    cssrule.NewInjector(),
		jsRules:     jsrule.NewInjector(),
		extendedCSS: extendedCSS,

		scriptletsURL:  getAssetURL(assetServerURL, scriptletsPath),
		jsRuleURL:      getAssetURL(assetServerURL, jsRulePath),
		extendedCSSURL: getAssetURL(assetServerURL, extendedCSSPath),
		cosmeticCSSURL: getAssetURL(assetServerURL, cosmeticCSSPath),
		cssRuleCSSURL:  getAssetURL(assetServerURL, cssRulePath),
	}, nil
}

// AddRule attempts to add a non-network rule. Returns handled=true if consumed.
func (e *Engine) AddRule(rule string, filterListTrusted bool) (handled bool, err error) {
	switch {
	case scriptlet.RuleRegex.MatchString(rule):
		if err := e.scriptlets.AddRule(rule, filterListTrusted); err != nil {
			return true, fmt.Errorf("add scriptlet: %w", err)
		}
		return true, nil
	case cosmetic.IsRule(rule):
		if err := e.cosmetic.AddRule(rule); err != nil {
			return true, fmt.Errorf("add cosmetic rule: %w", err)
		}
		return true, nil
	case extendedcss.IsRule(rule):
		if err := e.extendedCSS.AddRule(rule); err != nil {
			return true, fmt.Errorf("add extended css rule: %w", err)
		}
		return true, nil
	case filterListTrusted && cssrule.RuleRegex.MatchString(rule):
		if err := e.cssRules.AddRule(rule); err != nil {
			return true, fmt.Errorf("add css rule: %w", err)
		}
		return true, nil
	case filterListTrusted && jsrule.RuleRegex.MatchString(rule):
		if err := e.jsRules.AddRule(rule); err != nil {
			return true, fmt.Errorf("add js rule: %w", err)
		}
		return true, nil
	default:
		return false, nil
	}
}

// Inject appends asset tags for the matching hostname into HTML responses.
func (e *Engine) Inject(_ *http.Request, res *http.Response) error {
	scriptletsNonce := csp.NewNonce()
	jsRuleNonce := csp.NewNonce()
	extendedCSSNonce := csp.NewNonce()
	cosmeticCSSNonce := csp.NewNonce()
	cssRuleNonce := csp.NewNonce()

	operations := []csp.PatchOperation{
		{Nonce: scriptletsNonce, Kind: csp.Script, ResourceURL: e.scriptletsURL},
		{Nonce: jsRuleNonce, Kind: csp.Script, ResourceURL: e.jsRuleURL},
		{Nonce: extendedCSSNonce, Kind: csp.Script, ResourceURL: e.extendedCSSURL},
		{Nonce: cosmeticCSSNonce, Kind: csp.Style, ResourceURL: e.cosmeticCSSURL},
		{Nonce: cssRuleNonce, Kind: csp.Style, ResourceURL: e.cssRuleCSSURL},
	}
	if err := csp.PatchHeadersBatch(res, operations); err != nil {
		return fmt.Errorf("patch CSP headers: %w", err)
	}

	var injection bytes.Buffer
	injection.WriteString(scriptTag(e.scriptletsURL, scriptletsNonce))
	injection.WriteString(scriptTag(e.jsRuleURL, jsRuleNonce))
	injection.WriteString(scriptTag(e.extendedCSSURL, extendedCSSNonce))
	injection.WriteString(styleTag(e.cosmeticCSSURL, cosmeticCSSNonce))
	injection.WriteString(styleTag(e.cssRuleCSSURL, cssRuleNonce))

	if err := httprewrite.AppendHTMLHeadContents(res, injection.Bytes()); err != nil {
		return fmt.Errorf("append head contents: %w", err)
	}

	return nil
}

// assetBytes returns the asset content for a hostname and kind.
func (e *Engine) assetBytes(hostname string, kind kind) ([]byte, error) {
	switch kind {
	case cosmeticCSS:
		return e.cosmetic.GetAsset(hostname), nil
	case cssRule:
		return e.cssRules.GetAsset(hostname), nil
	case scriptlets:
		body, err := e.scriptlets.GetAsset(hostname)
		if err != nil {
			return nil, fmt.Errorf("scriptlets asset: %w", err)
		}
		return body, nil
	case extendedCSS:
		body, err := e.extendedCSS.GetAsset(hostname)
		if err != nil {
			return nil, fmt.Errorf("extended CSS asset: %w", err)
		}
		return body, nil
	case jsRule:
		body, err := e.jsRules.GetAsset(hostname)
		if err != nil {
			return nil, fmt.Errorf("js rules: %w", err)
		}
		return body, nil
	default:
		return nil, fmt.Errorf("unknown asset kind: %d", kind)
	}
}

func getAssetURL(base *url.URL, path string) string {
	return base.JoinPath(path).String()
}

func scriptTag(src, nonce string) string {
	return fmt.Sprintf(`<script nonce="%s" src="%s"></script>`, nonce, src)
}

func styleTag(href, nonce string) string {
	return fmt.Sprintf(`<link rel="stylesheet" nonce="%s" href="%s">`, nonce, href)
}
