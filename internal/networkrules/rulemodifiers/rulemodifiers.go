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

// MatchingModifier defines whether a rule matches a request.
type MatchingModifier interface {
	Modifier
	ShouldMatchReq(*http.Request) bool
	ShouldMatchRes(*http.Response) bool
}

// ReqResModifier modifies requests and responses.
type ReqResModifier interface {
	Modifier
	ModifyReq(*http.Request) bool
	ModifyRes(*http.Response) (bool, error)
}

// QueryModifier modifies request query parameters.
type QueryModifier interface {
	Modifier
	ModifyQuery(url.Values) bool
}
