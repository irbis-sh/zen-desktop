package app

import (
	"log"

	nrule "github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
	"github.com/irbis-sh/zen-desktop/internal/process"
)

type filterEventKind string

const (
	filterChannel                       = "filter:action"
	filterEventBlock    filterEventKind = "block"
	filterEventRedirect filterEventKind = "redirect"
	filterEventModify   filterEventKind = "modify"
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
	e.emit(filterChannel, newFilterEvent(filterEventBlock, method, url, "", referer, rules, processInfo))
}

func (e *frontendEvents) OnFilterRedirect(method, url, to, referer string, rules []nrule.Rule, processInfo process.Info) {
	e.emit(filterChannel, newFilterEvent(filterEventRedirect, method, url, to, referer, rules, processInfo))
}

func (e *frontendEvents) OnFilterModify(method, url, referer string, rules []nrule.Rule, processInfo process.Info) {
	e.emit(filterChannel, newFilterEvent(filterEventModify, method, url, "", referer, rules, processInfo))
}
