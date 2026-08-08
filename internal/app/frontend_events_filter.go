package app

import (
	"log"

	"github.com/irbis-sh/zen-desktop/internal/localcdn"
	nrule "github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
	"github.com/irbis-sh/zen-desktop/internal/process"
)

type filterEventKind string

const (
	filterChannel                       = "filter:action"
	filterEventBlock    filterEventKind = "block"
	filterEventRedirect filterEventKind = "redirect"
	filterEventModify   filterEventKind = "modify"
	filterEventLocal    filterEventKind = "local"
)

type rulePayload struct {
	RawRule    string `json:"rawRule"`
	FilterName string `json:"filterName"`
}

type processPayload struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	DiskPath string `json:"diskPath"`
}

type filterEvent struct {
	Kind    filterEventKind `json:"kind"`
	Method  string          `json:"method"`
	URL     string          `json:"url"`
	To      string          `json:"to,omitempty"`
	Referer string          `json:"referer,omitempty"`
	Rules   []rulePayload   `json:"rules"`
	Process processPayload  `json:"process"`
	// Resource is the name of the library served by the local resource engine.
	Resource string `json:"resource,omitempty"`
	// Blocked is true when the local resource engine blocked a request for a
	// missing resource.
	Blocked bool `json:"blocked,omitempty"`
	// RequestedVersion is the version requested in the URL.
	RequestedVersion string `json:"requestedVersion,omitempty"`
	// ServedVersion is the version of the locally served copy.
	ServedVersion string `json:"servedVersion,omitempty"`
	// VersionDelta is "upgrade", "downgrade", or "" (see localcdn.VersionDelta).
	VersionDelta string `json:"versionDelta,omitempty"`
}

func newFilterEvent(kind filterEventKind, method, url, to, referer string, rules []nrule.Rule, processInfo process.Info) filterEvent {
	payloadRules := make([]rulePayload, len(rules))
	for i, rule := range rules {
		filterName := ""
		if rule.FilterName != nil {
			filterName = *rule.FilterName
		}

		payloadRules[i] = rulePayload{
			RawRule:    rule.RawRule,
			FilterName: filterName,
		}
	}

	processPayload := processPayload{ID: int(processInfo.PID), DiskPath: processInfo.ExecutablePath}
	if name, err := processInfo.Name(); err == nil {
		processPayload.Name = name
	} else {
		log.Printf("failed to resolve process name for pid %d: %v", processInfo.PID, err)
	}

	return filterEvent{
		Kind:    kind,
		Method:  method,
		URL:     url,
		To:      to,
		Referer: referer,
		Rules:   payloadRules,
		Process: processPayload,
	}
}

func (e *frontendEvents) OnFilterBlock(method, url, referer string, rules []nrule.Rule, processInfo process.Info) {
	if e.recordFilterHit != nil {
		e.recordFilterHit()
	}
	e.emit(filterChannel, newFilterEvent(filterEventBlock, method, url, "", referer, rules, processInfo))
}

func (e *frontendEvents) OnFilterRedirect(method, url, to, referer string, rules []nrule.Rule, processInfo process.Info) {
	if e.recordFilterHit != nil {
		e.recordFilterHit()
	}
	e.emit(filterChannel, newFilterEvent(filterEventRedirect, method, url, to, referer, rules, processInfo))
}

func (e *frontendEvents) OnFilterModify(method, url, referer string, rules []nrule.Rule, processInfo process.Info) {
	if e.recordFilterHit != nil {
		e.recordFilterHit()
	}
	e.emit(filterChannel, newFilterEvent(filterEventModify, method, url, "", referer, rules, processInfo))
}

// OnLocalServed emits an event for a request served by the local resource engine.
func (e *frontendEvents) OnLocalServed(method, url, referer, resource, cdnHost, requestedVersion, servedVersion string, processInfo process.Info) {
	event := newFilterEvent(filterEventLocal, method, url, "", referer, nil, processInfo)
	event.Resource = resource
	event.To = cdnHost
	event.RequestedVersion = requestedVersion
	event.ServedVersion = servedVersion
	event.VersionDelta = localcdn.VersionDelta(requestedVersion, servedVersion)
	e.emit(filterChannel, event)
}

// OnLocalBlocked emits an event for a request blocked because no local copy of
// a known CDN resource exists.
func (e *frontendEvents) OnLocalBlocked(method, url, referer, cdnHost string, processInfo process.Info) {
	event := newFilterEvent(filterEventLocal, method, url, "", referer, nil, processInfo)
	event.Blocked = true
	event.To = cdnHost
	e.emit(filterChannel, event)
}
