package networkrules

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/irbis-sh/zen-desktop/internal/fetchmeta"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/contenttype"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/exceptionrule"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rulemodifiers"
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
	matchReq := rulemodifiers.NewRequest(req)

	primaryRules := nr.primaryStore.Match(reqURL, func(r *rule.Rule) bool {
		return r.MayModifyReq(matchReq) && r.ShouldMatchReq(matchReq, reqType)
	})
	if len(primaryRules) == 0 {
		return nil, false, ""
	}

	exceptions := nr.exceptionStore.Match(reqURL, func(er *exceptionrule.ExceptionRule) bool {
		return er.ShouldMatchReq(matchReq, reqType)
	})

	clickedLink := fetchmeta.IsUserActivatedNavigation(req)
	initialURL := req.URL.String()
	query := matchReq.Query

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
		if r.ModifyReqQuery(query) {
			queryModified = true
			modified = true
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

// MatchCNAMEs checks names, the CNAME chain of req's host, nearest first. It returns
// name, the first of them for which a blocking rule applies to req as if it were req's
// host, and r, that rule. ok is false if no rule applies to any of them. It doesn't
// change req.
//
// Each name is matched as a third-party request to "scheme://name/" from the page that
// made req, with req's method and content type. The path is left out because a rule
// with a path was written for the tracker's own URLs, not for the site's.
//
// An exception cancels a rule if it matches either the target's request or req itself,
// so that "@@||site.example^" allows a request to site.example whatever its CNAME chain
// holds.
func (nr *NetworkRules) MatchCNAMEs(req *http.Request, names []string) (name string, r rule.Rule, ok bool) {
	reqType := contenttype.FromRequest(req)
	matchReq := rulemodifiers.NewRequest(req)

	for _, target := range names {
		targetURL := req.URL.Scheme + "://" + target + "/"
		targetReq := matchReq.WithHost(target)
		candidates := nr.primaryStore.Match(targetURL, func(r *rule.Rule) bool {
			return r.IsBlockRule() && r.ShouldMatchReq(targetReq, reqType)
		})
		// The usual case. Exceptions are matched only for a name that has a candidate.
		if len(candidates) == 0 {
			continue
		}

		exceptions := nr.exceptionStore.Match(targetURL, func(er *exceptionrule.ExceptionRule) bool {
			return er.ShouldMatchReq(targetReq, reqType)
		})
		exceptions = append(exceptions, nr.exceptionStore.Match(renderURLWithoutPort(req.URL), func(er *exceptionrule.ExceptionRule) bool {
			return er.ShouldMatchReq(matchReq, reqType)
		})...)
	nextCandidate:
		for _, candidate := range candidates {
			for _, ex := range exceptions {
				if ex.Cancels(candidate) {
					continue nextCandidate
				}
			}
			return target, *candidate, true
		}
	}
	return "", rule.Rule{}, false
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
