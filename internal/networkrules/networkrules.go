package networkrules

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/exceptionrule"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
)

type NetworkRules struct {
	primaryStore   *ruleStore[*rule.Rule]
	exceptionStore *ruleStore[*exceptionrule.ExceptionRule]
}

func New() *NetworkRules {
	return &NetworkRules{
		primaryStore:   newRuleStore[*rule.Rule](),
		exceptionStore: newRuleStore[*exceptionrule.ExceptionRule](),
	}
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
