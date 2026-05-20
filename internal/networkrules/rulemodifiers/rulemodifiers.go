package rulemodifiers

import (
	"net/http"
	"net/url"
)

// Modifier is a Modifier of a rule.
type Modifier interface {
	Parse(string) error
	Cancels(Modifier) bool
}

// ConditionModifier restrict when a rule applies based on request/response metadata.
type ConditionModifier interface {
	Modifier
	ShouldMatchReq(*http.Request) bool
	ShouldMatchRes(*http.Response) bool
}

// ActionModifier modifies requests and responses.
type ActionModifier interface {
	Modifier
	ModifyReq(*http.Request) bool
	ModifyRes(*http.Response) (bool, error)
}

// QueryModifier modifies request query parameters.
// According to terminology, they are also "action modifiers", but are implemented separately for performance reasons.
type QueryModifier interface {
	Modifier
	ModifyQuery(url.Values) bool
}
