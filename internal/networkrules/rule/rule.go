package rule

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/rulemodifiers"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rulemodifiers/removejsconstant"
)

// Rule represents modifiers of a rule.
type Rule struct {
	// string representation
	RawRule string
	// FilterName is the name of the filter that the rule belongs to.
	FilterName *string

	MatchingModifiers matchingModifiers
	ReqResModifiers   []rulemodifiers.ReqResModifier
	QueryModifiers    []rulemodifiers.QueryModifier

	// Document shows if rule has Document modifier.
	Document bool
}

type matchingModifiers struct {
	// And are modifiers that must all match for the rule to apply.
	And []rulemodifiers.MatchingModifier
	// Or are modifiers where at least one must match for the rule to apply.
	Or []rulemodifiers.MatchingModifier
}

func (rm *Rule) ParseModifiers(modifiers []string) error {
	for _, m := range modifiers {
		if len(m) == 0 {
			return errors.New("empty modifier")
		}

		isKind := func(kind string) bool {
			if len(m) > 0 && m[0] == '~' {
				return strings.HasPrefix(m[1:], kind)
			}
			return strings.HasPrefix(m, kind)
		}

		if isKind("document") || isKind("doc") {
			rm.Document = true
			continue
		}

		var modifier rulemodifiers.Modifier
		var isOr bool // true if modifier belongs to OrModifiers; false if it belongs to AndModifiers
		switch {
		case isKind("domain"):
			modifier = &rulemodifiers.DomainModifier{}
		case isKind("method"):
			modifier = &rulemodifiers.MethodModifier{}
		case isKind("xmlhttprequest"),
			isKind("xhr"),
			isKind("font"),
			isKind("subdocument"),
			isKind("image"),
			isKind("object"),
			isKind("script"),
			isKind("stylesheet"),
			isKind("media"),
			isKind("other"):
			modifier = &rulemodifiers.ContentTypeModifier{}
			isOr = true
		case isKind("third-party"):
			modifier = &rulemodifiers.ThirdPartyModifier{}
		case isKind("removeparam"):
			modifier = &rulemodifiers.RemoveParamModifier{}
		case isKind("header"):
			modifier = &rulemodifiers.HeaderModifier{}
		case isKind("removeheader"):
			modifier = &rulemodifiers.RemoveHeaderModifier{}
		case isKind("remove-js-constant"):
			modifier = &removejsconstant.Modifier{}
		case isKind("scramblejs"):
			modifier = &rulemodifiers.ScrambleJSModifier{}
		case isKind("jsonprune"):
			modifier = &rulemodifiers.JSONPruneModifier{}
		case isKind("all"):
			// TODO: should act as "popup" modifier once it gets implemented
			continue
		default:
			return fmt.Errorf("unknown modifier %q", m)
		}

		if err := modifier.Parse(m); err != nil {
			return err
		}

		switch typed := modifier.(type) {
		case rulemodifiers.MatchingModifier:
			if isOr {
				rm.MatchingModifiers.Or = append(rm.MatchingModifiers.Or, typed)
			} else {
				rm.MatchingModifiers.And = append(rm.MatchingModifiers.And, typed)
			}
		case rulemodifiers.ReqResModifier:
			rm.ReqResModifiers = append(rm.ReqResModifiers, typed)
		case rulemodifiers.QueryModifier:
			rm.QueryModifiers = append(rm.QueryModifiers, typed)
		default:
			log.Fatalf("got unknown modifier type %T for modifier %s", modifier, m)
		}
	}

	return nil
}

// ShouldMatchReq returns true if the rule should match the request.
func (rm *Rule) ShouldMatchReq(req *http.Request) bool {
	if req.Header.Get("Sec-Fetch-User") == "?1" && req.Header.Get("Sec-Fetch-Dest") == "document" && !rm.Document {
		return false
	}

	// AndModifiers: All must match.
	for _, m := range rm.MatchingModifiers.And {
		if !m.ShouldMatchReq(req) {
			return false
		}
	}

	// OrModifiers: At least one must match.
	if len(rm.MatchingModifiers.Or) > 0 {
		for _, m := range rm.MatchingModifiers.Or {
			if m.ShouldMatchReq(req) {
				return true
			}
		}
		return false
	}

	return true
}

// ShouldMatchRes returns true if the rule should match the response.
func (rm *Rule) ShouldMatchRes(res *http.Response) bool {
	// maybe add sec-fetch logic too
	for _, m := range rm.MatchingModifiers.And {
		if !m.ShouldMatchRes(res) {
			return false
		}
	}

	if len(rm.MatchingModifiers.Or) > 0 {
		for _, m := range rm.MatchingModifiers.Or {
			if m.ShouldMatchRes(res) {
				return true
			}
		}
		return false
	}

	return true
}

// ShouldBlockReq returns true if the request should be blocked.
func (rm *Rule) ShouldBlockReq(*http.Request) bool {
	return len(rm.ReqResModifiers) == 0 && len(rm.QueryModifiers) == 0
}

// ModifyReq modifies a request. Returns true if the request was modified.
func (rm *Rule) ModifyReq(req *http.Request) (modified bool) {
	for _, modifier := range rm.ReqResModifiers {
		if modifier.ModifyReq(req) {
			modified = true
		}
	}

	return modified
}

// ModifyReqQuery modifies a request query. Returns true if the query was modified.
func (rm *Rule) ModifyReqQuery(query url.Values) (modified bool) {
	for _, qm := range rm.QueryModifiers {
		if qm.ModifyQuery(query) {
			modified = true
		}
	}

	return modified
}

// ModifyRes modifies a response. Returns true if the response was modified.
func (rm *Rule) ModifyRes(res *http.Response) (modified bool, err error) {
	for _, modifier := range rm.ReqResModifiers {
		m, err := modifier.ModifyRes(res)
		if err != nil {
			return false, fmt.Errorf("modify response: %w", err)
		}
		if m {
			modified = true
		}
	}

	return modified, nil
}
