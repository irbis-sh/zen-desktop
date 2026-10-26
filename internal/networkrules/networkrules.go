package networkrules

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/irbis-sh/zen-desktop/internal/fetchmeta"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/contenttype"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/exceptionrule"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
)

type NetworkRules struct {
	primaryStore   *ruleStore[*rule.Rule]
	exceptionStore *ruleStore[*exceptionrule.ExceptionRule]
	// interner is nil after Compact.
	interner *rule.ModifierInterner
}

func New() *NetworkRules {
	return &NetworkRules{
		primaryStore:   newRuleStore[*rule.Rule](),
		exceptionStore: newRuleStore[*exceptionrule.ExceptionRule](),
		interner:       rule.NewModifierInterner(),
	}
}

func (nr *NetworkRules) ModifyReq(req *http.Request) (appliedRules []rule.Rule, shouldBlock bool, redirectURL string) {
	reqURL := renderURLWithoutPort(req.URL)
	reqType := contenttype.FromRequest(req)

	primaryRules := nr.primaryStore.Match(reqURL, func(r *rule.Rule) bool {
		return r.ShouldMatchReq(req, reqType)
	})
	if len(primaryRules) == 0 {
		return nil, false, ""
	}

	exceptions := nr.exceptionStore.Match(reqURL, func(er *exceptionrule.ExceptionRule) bool {
		return er.ShouldMatchReq(req, reqType)
	})

	clickedLink := fetchmeta.IsUserActivatedNavigation(req)
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
		if r.IsBlockRule() {
			// As in AdGuard, a block rule doesn't stop a page the user asked for unless it
			// opts in with $document or $all. Rules that modify the request still apply.
			if clickedLink && !r.BlocksNavigations() {
				continue
			}
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
	resType := contenttype.FromResponse(req, res)

	// Only action modifiers can change a response.
	primaryRules := nr.primaryStore.Match(url, func(r *rule.Rule) bool {
		return len(r.ActionModifiers()) > 0 && r.ShouldMatchRes(res, resType)
	})
	if len(primaryRules) == 0 {
		return nil, nil
	}

	// No action-modifier check here: a block-only exception such as @@||x^
	// cancels action rules too (see ExceptionRule.Cancels).
	exceptions := nr.exceptionStore.Match(url, func(er *exceptionrule.ExceptionRule) bool {
		return er.ShouldMatchRes(res, resType)
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
	// Rules added after Compact, such as allowlist exceptions, are too few to be
	// worth sharing, so free the interner's map.
	nr.interner = nil
	nr.primaryStore.Compact()
	nr.exceptionStore.Compact()
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
