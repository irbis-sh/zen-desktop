package exceptionrule

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rulemodifiers"
)

type ExceptionRule struct {
	RawRule    string
	FilterName *string

	Modifiers       ExceptionModifiers
	ReqResModifiers []rulemodifiers.ReqResModifier
	QueryModifiers  []rulemodifiers.QueryModifier
	Document        bool
}

type ExceptionModifiers struct {
	AndModifiers []exceptionModifier
	OrModifiers  []exceptionModifier
}

type exceptionModifier interface {
	Cancels(rulemodifiers.Modifier) bool
	ShouldMatchReq(req *http.Request) bool
	ShouldMatchRes(res *http.Response) bool
}

func (er *ExceptionRule) Cancels(r *rule.Rule) bool {
	if er.Document && !r.Document {
		return false
	}

	if len(er.Modifiers.AndModifiers) == 0 && len(er.Modifiers.OrModifiers) == 0 && len(er.ReqResModifiers) == 0 && len(er.QueryModifiers) == 0 {
		return true
	}

	for _, exc := range er.Modifiers.AndModifiers {
		found := false
		for _, basic := range r.MatchingModifiers.And {
			if exc.Cancels(basic) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(er.Modifiers.OrModifiers) > 0 {
		found := false
		for _, exc := range er.Modifiers.OrModifiers {
			for _, basic := range r.MatchingModifiers.Or {
				if exc.Cancels(basic) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	for _, exc := range er.ReqResModifiers {
		found := false
		for _, basic := range r.ReqResModifiers {
			if exc.Cancels(basic) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	for _, exc := range er.QueryModifiers {
		found := false
		for _, basic := range r.QueryModifiers {
			if exc.Cancels(basic) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (er *ExceptionRule) ParseModifiers(modifiers []string) error {
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
			er.Document = true
			continue
		}

		var modifier rulemodifiers.Modifier
		isOr := false
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
		case isKind("all"):
			// TODO: should act as "popup" modifier once it gets implemented
			continue
		default:
			return fmt.Errorf("unknown modifier %s", m)
		}

		if err := modifier.Parse(m); err != nil {
			return err
		}

		switch typed := modifier.(type) {
		case exceptionModifier:
			if isOr {
				er.Modifiers.OrModifiers = append(er.Modifiers.OrModifiers, typed)
			} else {
				er.Modifiers.AndModifiers = append(er.Modifiers.AndModifiers, typed)
			}
		case rulemodifiers.ReqResModifier:
			er.ReqResModifiers = append(er.ReqResModifiers, typed)
		case rulemodifiers.QueryModifier:
			er.QueryModifiers = append(er.QueryModifiers, typed)
		default:
			log.Fatalf("got unknown modifier type %T for modifier %s", modifier, m)
		}

	}

	return nil
}

// ShouldMatchReq returns true if the rule should match the request.
func (er *ExceptionRule) ShouldMatchReq(req *http.Request) bool {
	// AndModifiers: All must match.
	for _, m := range er.Modifiers.AndModifiers {
		if !m.ShouldMatchReq(req) {
			return false
		}
	}

	// OrModifiers: At least one must match.
	if len(er.Modifiers.OrModifiers) > 0 {
		for _, m := range er.Modifiers.OrModifiers {
			if m.ShouldMatchReq(req) {
				return true
			}
		}
		return false
	}

	return true
}

// ShouldMatchRes returns true if the rule should match the response.
func (er *ExceptionRule) ShouldMatchRes(res *http.Response) bool {
	for _, m := range er.Modifiers.AndModifiers {
		if !m.ShouldMatchRes(res) {
			return false
		}
	}

	if len(er.Modifiers.OrModifiers) > 0 {
		for _, m := range er.Modifiers.OrModifiers {
			if m.ShouldMatchRes(res) {
				return true
			}
		}
		return false
	}

	return true
}
