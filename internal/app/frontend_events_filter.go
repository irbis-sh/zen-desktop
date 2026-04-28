package app

import (
	"log"

	"github.com/irbis-sh/process"
	nrule "github.com/irbis-sh/zen-core/networkrules/rule"
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

func newFilterEvent(kind filterEventKind, method, url, to, referer string, rules []nrule.Rule, pid process.PID) filterEvent {
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

	processPayload := processPayload{ID: int(pid)}
	if name, err := pid.Name(); err == nil {
		processPayload.Name = name
	} else {
		log.Printf("failed to resolve process name for pid %d: %v", pid, err)
	}
	if path, err := pid.ExecutablePath(); err == nil {
		processPayload.DiskPath = path
	} else {
		log.Printf("failed to resolve process path for pid %d: %v", pid, err)
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

func (e *frontendEvents) OnFilterBlock(method, url, referer string, rules []nrule.Rule, pid process.PID) {
	e.emit(filterChannel, newFilterEvent(filterEventBlock, method, url, "", referer, rules, pid))
}

func (e *frontendEvents) OnFilterRedirect(method, url, to, referer string, rules []nrule.Rule, pid process.PID) {
	e.emit(filterChannel, newFilterEvent(filterEventRedirect, method, url, to, referer, rules, pid))
}

func (e *frontendEvents) OnFilterModify(method, url, referer string, rules []nrule.Rule, pid process.PID) {
	e.emit(filterChannel, newFilterEvent(filterEventModify, method, url, "", referer, rules, pid))
}
