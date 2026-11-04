package rulemodifiers

import (
	"net/http"
	"net/url"
)

// Modifier is a Modifier of a rule.
//
// Cancels is declared on ActionModifier and QueryModifier only. An exception's condition
// modifiers decide whether it applies, never which rules it cancels (see ExceptionRule.Cancels).
type Modifier interface {
	Parse(string) error
}

// ConditionModifier restrict when a rule applies based on request/response metadata.
type ConditionModifier interface {
	Modifier
	ShouldMatchReq(Request) bool
	ShouldMatchRes(*http.Response) bool
}

// ActionModifier modifies requests and responses.
type ActionModifier interface {
	Modifier
	ModifyReq(*http.Request) bool
	ModifyRes(*http.Response) (bool, error)
	Cancels(Modifier) bool
}

// QueryModifier modifies request query parameters.
// According to terminology, they are also "action modifiers", but are implemented separately for performance reasons.
type QueryModifier interface {
	Modifier
	ModifyQuery(url.Values) bool
	Cancels(Modifier) bool
}
