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

	ConditionModifiers conditionModifiers
	ActionModifiers    []rulemodifiers.ActionModifier
	QueryModifiers     []rulemodifiers.QueryModifier

	// Document shows if rule has Document modifier.
	Document bool
	// Important shows if rule has Important modifier.
	Important bool
}

// TODO: The split between And and Or modifiers is somewhat convoluted and exists only to support ContentType.
// Remove it by grouping multiple ContentTypes into a single modifier and evaluating all modifiers with AND logic.

type conditionModifiers struct {
	// And are modifiers that must all match for the rule to apply.
	And []rulemodifiers.ConditionModifier
	// Or are modifiers where at least one must match for the rule to apply.
	Or []rulemodifiers.ConditionModifier
}

func (rm *Rule) ParseModifiers(modifiers []string) error {
	for _, m := range modifiers {
		if len(m) == 0 {
			return errors.New("empty modifier")
		}

		// Noop modifier is ignored
		if isNoopModifier(m) {
			continue
		}

		name, hasValue := cutModifierName(m)

		var modifier rulemodifiers.Modifier
		var isOr bool // true if the modifier belongs to ConditionModifiers.Or.

		if !hasValue {
			// Flag modifiers.
			switch name {
			case "document", "doc":
				rm.Document = true
				continue
			case "important":
				rm.Important = true
				continue
			case "xmlhttprequest",
				"xhr",
				"font",
				"subdocument",
				"image",
				"object",
				"script",
				"stylesheet",
				"media",
				"websocket",
				"ping",
				"other":
				modifier = &rulemodifiers.ContentTypeModifier{}
				isOr = true
			case "third-party":
				modifier = &rulemodifiers.ThirdPartyModifier{}
			case "removeparam":
				modifier = &rulemodifiers.RemoveParamModifier{}
			case "all":
				// TODO: should act as "popup" modifier once it gets implemented
				continue
			default:
				return fmt.Errorf("unknown modifier %q", m)
			}
		} else {
			// Parametrised modifiers.
			switch name {
			case "domain":
				modifier = &rulemodifiers.DomainModifier{}
			case "method":
				modifier = &rulemodifiers.MethodModifier{}
			case "removeparam":
				modifier = &rulemodifiers.RemoveParamModifier{}
			case "header":
				modifier = &rulemodifiers.HeaderModifier{}
			case "removeheader":
				modifier = &rulemodifiers.RemoveHeaderModifier{}
			case "remove-js-constant":
				modifier = &removejsconstant.Modifier{}
			case "scramblejs":
				modifier = &rulemodifiers.ScrambleJSModifier{}
			case "jsonprune":
				modifier = &rulemodifiers.JSONPruneModifier{}
			default:
				return fmt.Errorf("unknown modifier %q", m)
			}
		}

		if err := modifier.Parse(m); err != nil {
			return err
		}

		switch typed := modifier.(type) {
		case rulemodifiers.ConditionModifier:
			if isOr {
				rm.ConditionModifiers.Or = append(rm.ConditionModifiers.Or, typed)
			} else {
				rm.ConditionModifiers.And = append(rm.ConditionModifiers.And, typed)
			}
		case rulemodifiers.ActionModifier:
			rm.ActionModifiers = append(rm.ActionModifiers, typed)
		case rulemodifiers.QueryModifier:
			rm.QueryModifiers = append(rm.QueryModifiers, typed)
		default:
			log.Fatalf("got unknown modifier type %T for modifier %s", modifier, m)
		}
	}

	return nil
}

// isNoopModifier returns true if modifier is one or more underscores.
func isNoopModifier(modifier string) bool {
	for i := 0; i < len(modifier); i++ {
		if modifier[i] != '_' {
			return false
		}
	}
	return true
}

func cutModifierName(modifier string) (name string, hasValue bool) {
	if len(modifier) > 0 && modifier[0] == '~' {
		modifier = modifier[1:]
	}
	name, _, hasValue = strings.Cut(modifier, "=")
	return name, hasValue
}

// ShouldMatchReq returns true if the rule should match the request.
func (rm *Rule) ShouldMatchReq(req *http.Request) bool {
	if req.Header.Get("Sec-Fetch-User") == "?1" && req.Header.Get("Sec-Fetch-Dest") == "document" && !rm.Document {
		return false
	}

	return rm.ModifiersMatchReq(req)
}

// ModifiersMatchReq returns true if the rule's matching modifiers match the request.
func (rm *Rule) ModifiersMatchReq(req *http.Request) bool {
	// AndModifiers: All must match.
	for _, m := range rm.ConditionModifiers.And {
		if !m.ShouldMatchReq(req) {
			return false
		}
	}

	// OrModifiers: At least one must match.
	if len(rm.ConditionModifiers.Or) > 0 {
		for _, m := range rm.ConditionModifiers.Or {
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
	return rm.ModifiersMatchRes(res)
}

// ModifiersMatchRes returns true if the rule's matching modifiers match the response.
func (rm *Rule) ModifiersMatchRes(res *http.Response) bool {
	for _, m := range rm.ConditionModifiers.And {
		if !m.ShouldMatchRes(res) {
			return false
		}
	}

	if len(rm.ConditionModifiers.Or) > 0 {
		for _, m := range rm.ConditionModifiers.Or {
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
	return len(rm.ActionModifiers) == 0 && len(rm.QueryModifiers) == 0
}

// ModifyReq modifies a request. Returns true if the request was modified.
func (rm *Rule) ModifyReq(req *http.Request) (modified bool) {
	for _, modifier := range rm.ActionModifiers {
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
	for _, modifier := range rm.ActionModifiers {
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
