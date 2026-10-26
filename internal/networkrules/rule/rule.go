package rule

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/contenttype"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rulemodifiers"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rulemodifiers/removejsconstant"
)

// Rule represents modifiers of a rule.
type Rule struct {
	// string representation
	RawRule string
	// FilterName is the name of the filter that the rule belongs to.
	FilterName *string

	// mods is nil for rules without modifiers, which are most of them.
	// Keeping the modifiers behind a pointer shrinks Rule from 128 to 32 bytes.
	mods *ruleModifiers
}

// ruleModifiers is a rule's parsed modifiers.
//
// ModifierInterner shares one block between all rules with the same modifier text,
// so a block must not be modified after parsing. A feature that edits a rule's
// modifiers must copy the block first.
type ruleModifiers struct {
	types contenttype.Mask // 0 = any type, including Unknown

	document  bool
	important bool
	all       bool

	conditions []rulemodifiers.ConditionModifier // domain, method, third-party, header
	actions    []rulemodifiers.ActionModifier
	queries    []rulemodifiers.QueryModifier
}

// Document reports whether the rule has the document modifier.
func (rm *Rule) Document() bool {
	return rm.mods != nil && rm.mods.document
}

// Important reports whether the rule has the important modifier.
func (rm *Rule) Important() bool {
	return rm.mods != nil && rm.mods.important
}

// All reports whether the rule has the all modifier.
// On primary rules, it implies Document. It is kept separate so that "@@...$all" exceptions,
// which embed Rule, still cancel non-document rules.
func (rm *Rule) All() bool {
	return rm.mods != nil && rm.mods.all
}

// types returns the content types the rule applies to.
func (rm *Rule) types() contenttype.Mask {
	if rm.mods == nil {
		return 0
	}
	return rm.mods.types
}

// conditions returns the condition modifiers, which must all match for the rule to apply.
func (rm *Rule) conditions() []rulemodifiers.ConditionModifier {
	if rm.mods == nil {
		return nil
	}
	return rm.mods.conditions
}

// ActionModifiers returns the rule's action modifiers.
func (rm *Rule) ActionModifiers() []rulemodifiers.ActionModifier {
	if rm.mods == nil {
		return nil
	}
	return rm.mods.actions
}

// QueryModifiers returns the rule's query modifiers.
func (rm *Rule) QueryModifiers() []rulemodifiers.QueryModifier {
	if rm.mods == nil {
		return nil
	}
	return rm.mods.queries
}

// ParseModifiers parses modifiers into the rule. Call it once, on a freshly constructed Rule.
func (rm *Rule) ParseModifiers(modifiers []string) error {
	// Parse into a local block and attach it only if some modifier is not a noop,
	// so rules like "||example.com^$_" keep mods nil.
	var mods ruleModifiers
	empty := true
	var included, excluded contenttype.Mask

	for _, m := range modifiers {
		if len(m) == 0 {
			return errors.New("empty modifier")
		}

		// Noop modifier is ignored
		if isNoopModifier(m) {
			continue
		}
		empty = false

		name, _, hasValue := strings.Cut(m, "=")

		var modifier rulemodifiers.Modifier

		if !hasValue {
			typeName, negated := strings.CutPrefix(name, "~")
			if t, ok := contenttype.ParseModifier(typeName); ok {
				if negated {
					excluded |= contenttype.Mask(t)
				} else {
					included |= contenttype.Mask(t)
				}
				continue
			}

			// Other flag modifiers.
			switch name {
			case "document", "doc":
				mods.document = true
				continue
			case "important":
				mods.important = true
				continue
			case "third-party", "~third-party":
				modifier = &rulemodifiers.ThirdPartyModifier{}
			case "removeparam":
				modifier = &rulemodifiers.RemoveParamModifier{}
			case "all":
				// TODO: should also act as "popup" modifier once it gets implemented
				mods.all = true
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
			mods.conditions = append(mods.conditions, typed)
		case rulemodifiers.ActionModifier:
			mods.actions = append(mods.actions, typed)
		case rulemodifiers.QueryModifier:
			mods.queries = append(mods.queries, typed)
		default:
			log.Fatalf("got unknown modifier type %T for modifier %s", modifier, m)
		}
	}

	if excluded != 0 {
		// As in uBO and AdGuard, a rule with only negated types applies to every other type.
		// So does a rule with $all, whatever positive types it also names.
		if included == 0 || mods.all {
			included = contenttype.All
		}
		mods.types = included &^ excluded
		if mods.types == 0 {
			// A zero mask would match every type instead of none.
			return errors.New("negated content types exclude every included type")
		}
	} else if !mods.all {
		// With $all, types stays 0: $all already covers any positive types.
		mods.types = included
	}

	if !empty {
		rm.mods = &mods
	}

	return nil
}

// splitModifiers splits by unescaped commas.
// Empty fields are preserved (like strings.Split).
func splitModifiers(s string) []string {
	var res []string
	var b strings.Builder
	escaped := false

	for _, r := range s {
		switch r {
		case '\\':
			if escaped {
				b.WriteRune('\\')
			}
			escaped = !escaped
		case ',':
			if escaped {
				b.WriteRune(',')
				escaped = false
			} else {
				res = append(res, b.String())
				b.Reset()
			}
		default:
			if escaped {
				b.WriteRune('\\')
				escaped = false
			}
			b.WriteRune(r)
		}
	}

	if escaped {
		b.WriteRune('\\')
	}
	res = append(res, b.String())
	return res
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

// ShouldMatchReq reports whether the rule matches a request of type t.
func (rm *Rule) ShouldMatchReq(req *http.Request, t contenttype.Type) bool {
	if req.Header.Get("Sec-Fetch-User") == "?1" && req.Header.Get("Sec-Fetch-Dest") == "document" && !rm.Document() && !rm.All() {
		return false
	}

	return rm.ModifiersMatchReq(req, t)
}

// ModifiersMatchReq returns true if the rule's matching modifiers match a request of type t.
func (rm *Rule) ModifiersMatchReq(req *http.Request, t contenttype.Type) bool {
	if !rm.types().Matches(t) {
		return false
	}
	for _, m := range rm.conditions() {
		if !m.ShouldMatchReq(req) {
			return false
		}
	}
	return true
}

// ShouldMatchRes reports whether the rule matches a response of type t.
func (rm *Rule) ShouldMatchRes(res *http.Response, t contenttype.Type) bool {
	return rm.ModifiersMatchRes(res, t)
}

// ModifiersMatchRes returns true if the rule's matching modifiers match a response of type t.
func (rm *Rule) ModifiersMatchRes(res *http.Response, t contenttype.Type) bool {
	if !rm.types().Matches(t) {
		return false
	}
	for _, m := range rm.conditions() {
		if !m.ShouldMatchRes(res) {
			return false
		}
	}
	return true
}

// ShouldBlockReq returns true if the request should be blocked.
func (rm *Rule) ShouldBlockReq(*http.Request) bool {
	return len(rm.ActionModifiers()) == 0 && len(rm.QueryModifiers()) == 0
}

// ModifyReq modifies a request. Returns true if the request was modified.
func (rm *Rule) ModifyReq(req *http.Request) (modified bool) {
	for _, modifier := range rm.ActionModifiers() {
		if modifier.ModifyReq(req) {
			modified = true
		}
	}

	return modified
}

// ModifyReqQuery modifies a request query. Returns true if the query was modified.
func (rm *Rule) ModifyReqQuery(query url.Values) (modified bool) {
	for _, qm := range rm.QueryModifiers() {
		if qm.ModifyQuery(query) {
			modified = true
		}
	}

	return modified
}

// ModifyRes modifies a response. Returns true if the response was modified.
func (rm *Rule) ModifyRes(res *http.Response) (modified bool, err error) {
	for _, modifier := range rm.ActionModifiers() {
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
