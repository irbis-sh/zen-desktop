package networkrules

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/exceptionrule"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
	"github.com/irbis-sh/zen-desktop/internal/ruletree"
)

var (
	reHosts       = regexp.MustCompile(`^(?:0\.0\.0\.0|127\.0\.0\.1)\s(.+)`)
	reHostsIgnore = regexp.MustCompile(`^(?:0\.0\.0\.0|broadcasthost|local|localhost(?:\.localdomain)?|ip6-\w+)$`)
)

type ruleStore[T any] interface {
	Insert(string, T)
	Get(string) []T
	Compact()
}

type NetworkRules struct {
	primaryStore   ruleStore[*rule.Rule]
	exceptionStore ruleStore[*exceptionrule.ExceptionRule]
}

func New() *NetworkRules {
	return &NetworkRules{
		primaryStore:   ruletree.New[*rule.Rule](),
		exceptionStore: ruletree.New[*exceptionrule.ExceptionRule](),
	}
}

func (nr *NetworkRules) ParseRule(rawRule string, filterName *string) (isException bool, err error) {
	if matches := reHosts.FindStringSubmatch(rawRule); matches != nil {
		hostsField := matches[1]
		if commentIndex := strings.IndexByte(hostsField, '#'); commentIndex != -1 {
			hostsField = hostsField[:commentIndex]
		}

		// An IP address may be followed by multiple hostnames.
		//
		// As stated in https://man.freebsd.org/cgi/man.cgi?hosts(5):
		// "Items are separated by any number of blanks and/or tab characters."
		hosts := strings.Fields(hostsField)

		for _, host := range hosts {
			if reHostsIgnore.MatchString(host) {
				continue
			}

			pattern := fmt.Sprintf("||%s^", host)
			nr.primaryStore.Insert(pattern, &rule.Rule{
				RawRule:    rawRule,
				FilterName: filterName,
				Document:   true,
			})
		}

		return false, nil
	}

	if strings.HasPrefix(rawRule, "@@") {
		r := &exceptionrule.ExceptionRule{
			RawRule:    rawRule,
			FilterName: filterName,
		}

		pattern, modifiers, found := strings.Cut(rawRule[2:], "$")
		if pattern != "" && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
			// This is a regexp rule.
			// TODO: implement proper support for regexp rules.
			return true, nil
		}
		if found {
			modifiersArr := splitModifiers(modifiers)
			if err := r.ParseModifiers(modifiersArr); err != nil {
				return false, fmt.Errorf("parse modifiers: %v", err)
			}
		}
		nr.exceptionStore.Insert(pattern, r)

		return true, nil
	}

	r := &rule.Rule{
		RawRule:    rawRule,
		FilterName: filterName,
	}

	pattern, modifiers, found := strings.Cut(rawRule, "$")
	if pattern != "" && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
		// This is a regexp rule.
		// TODO: implement proper support for regexp rules.
		return false, nil
	}
	if found {
		modifiersArr := splitModifiers(modifiers)
		if err := r.ParseModifiers(modifiersArr); err != nil {
			return false, fmt.Errorf("parse modifiers: %v", err)
		}
	}
	nr.primaryStore.Insert(pattern, r)

	return false, nil
}

func (nr *NetworkRules) ModifyReq(req *http.Request) (appliedRules []rule.Rule, shouldBlock bool, redirectURL string) {
	reqURL := renderURLWithoutPort(req.URL)

	primaryRules := nr.primaryStore.Get(reqURL)
	primaryRules = filter(primaryRules, func(r *rule.Rule) bool {
		return r.ShouldMatchReq(req)
	})
	if len(primaryRules) == 0 {
		return nil, false, ""
	}

	exceptions := nr.exceptionStore.Get(reqURL)
	exceptions = filter(exceptions, func(er *exceptionrule.ExceptionRule) bool {
		return er.ShouldMatchReq(req)
	})

	initialURL := req.URL.String()

	var query url.Values
	if req.URL.RawQuery != "" {
		query = req.URL.Query()
	}

	var queryModified bool
outer:
	for _, r := range primaryRules {
		for _, ex := range exceptions {
			if ex.Cancels(r) {
				continue outer
			}
		}
		if r.ShouldBlockReq(req) {
			return []rule.Rule{*r}, true, ""
		}

		modified := r.ModifyReq(req)
		if query != nil {
			if r.ModifyReqQuery(query) {
				queryModified = true
				modified = true
			}
		}

		if modified {
			appliedRules = append(appliedRules, *r)
		}
	}

	if queryModified {
		// Re-encoding the same query params may cause subtle normalization changes
		// (e.g. parameter reordering), so only do it if they were actually modified.
		req.URL.RawQuery = query.Encode()
	}

	finalURL := req.URL.String()
	if initialURL != finalURL {
		return appliedRules, false, finalURL
	}

	return appliedRules, false, ""
}

func (nr *NetworkRules) ModifyRes(req *http.Request, res *http.Response) ([]rule.Rule, error) {
	url := renderURLWithoutPort(req.URL)

	primaryRules := nr.primaryStore.Get(url)
	primaryRules = filter(primaryRules, func(r *rule.Rule) bool {
		return r.ShouldMatchRes(res)
	})
	if len(primaryRules) == 0 {
		return nil, nil
	}

	exceptions := nr.exceptionStore.Get(url)
	exceptions = filter(exceptions, func(er *exceptionrule.ExceptionRule) bool {
		return er.ShouldMatchRes(res)
	})

	var appliedRules []rule.Rule
outer:
	for _, r := range primaryRules {
		for _, ex := range exceptions {
			if ex.Cancels(r) {
				continue outer
			}
		}

		m, err := r.ModifyRes(res)
		if err != nil {
			return nil, fmt.Errorf("apply %q: %v", r.RawRule, err)
		}
		if m {
			appliedRules = append(appliedRules, *r)
		}
	}

	return appliedRules, nil
}

func (nr *NetworkRules) Compact() {
	nr.primaryStore.Compact()
	nr.exceptionStore.Compact()
}

// filter returns a new slice containing only the elements of arr
// that satisfy the predicate.
func filter[T any](arr []T, predicate func(T) bool) []T {
	var res []T
	for _, el := range arr {
		if predicate(el) {
			res = append(res, el)
		}
	}
	return res
}

func renderURLWithoutPort(u *url.URL) string {
	stripped := url.URL{
		Scheme:   u.Scheme,
		Host:     u.Hostname(),
		Path:     u.Path,
		RawQuery: u.RawQuery,
	}

	return stripped.String()
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
