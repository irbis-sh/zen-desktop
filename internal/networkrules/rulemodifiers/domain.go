package rulemodifiers

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var (
	// domainModifierRegex matches domain modifier entries.
	//
	// The need for this regex comes from the fact that domain modifiers can contain regular expressions,
	// which can contain the separator character (|). This makes it impossible to just split the modifier by the separator.
	domainModifierRegex = regexp.MustCompile(`~?((/.*/)|[^|]+)+`)
)

type DomainModifier struct {
	entries  []domainModifierEntry
	inverted bool
}

var _ ConditionModifier = (*DomainModifier)(nil)

func (m *DomainModifier) Parse(modifier string) error {
	eqIndex := strings.IndexByte(modifier, '=')
	if eqIndex == -1 || eqIndex == len(modifier)-1 {
		return errors.New("invalid domain modifier")
	}
	value := modifier[eqIndex+1:]

	m.inverted = strings.HasPrefix(value, "~")
	matches := domainModifierRegex.FindAllString(value, -1)
	m.entries = make([]domainModifierEntry, len(matches))
	for i, entry := range matches {
		inverted := len(entry) > 0 && entry[0] == '~'
		if inverted != m.inverted {
			return errors.New("cannot mix inverted and non-inverted method modifiers")
		}
		if inverted {
			entry = entry[1:]
		}

		m.entries[i] = domainModifierEntry{}
		if err := m.entries[i].Parse(entry); err != nil {
			return fmt.Errorf("parse entry %q: %w", entry, err)
		}
	}
	return nil
}

func (m *DomainModifier) ShouldMatchReq(req Request) bool {
	if !req.sourceHostOK {
		return false
	}

	matches := false
	for _, entry := range m.entries {
		if entry.matches(req) {
			matches = true
			break
		}
	}
	if m.inverted {
		return !matches
	}
	return matches
}

// ShouldMatchRes reports whether m matches res.
func (m *DomainModifier) ShouldMatchRes(res *http.Response) bool {
	if res.Request == nil {
		return false
	}
	// Reuse the request-side logic so the Referer validity check stays shared.
	return m.ShouldMatchReq(NewRequest(res.Request))
}

type domainModifierEntry struct {
	// suffix is the domain with a leading dot, such as ".example.com". It is
	// built once at parse time because every request checks every entry.
	suffix string
	// label is the part before ".*" in an entry like "example.*".
	label  string
	regexp *regexp.Regexp
}

func (m *domainModifierEntry) Parse(entry string) error {
	if len(entry) == 0 {
		return errors.New("entry is empty")
	}

	var err error
	if m.regexp, err = parseRegexp(entry); err != nil {
		return fmt.Errorf("parse regexp: %w", err)
	} else if m.regexp != nil {
		return nil
	}

	if strings.HasSuffix(entry, ".*") {
		m.label = strings.TrimSuffix(entry, ".*")
		if len(m.label) == 0 {
			return errors.New("label is empty")
		}
		return nil
	}

	m.suffix = "." + entry
	return nil
}

func (m *domainModifierEntry) matches(req Request) bool {
	switch {
	case m.suffix != "":
		return req.sourceHost == m.suffix[1:] || strings.HasSuffix(req.sourceHost, m.suffix)
	case m.label != "":
		return req.sourceLabel == m.label
	case m.regexp != nil:
		return m.regexp.MatchString(req.sourceHost)
	default:
		return false
	}
}
