package rulemodifiers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

var _ MatchingModifier = (*DomainModifier)(nil)

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

func (m *DomainModifier) ShouldMatchReq(req *http.Request) bool {
	var hostname string
	referer := req.Header.Get("Referer")
	// Allow empty "Referer" header to make inverted rules work.
	if referer != "" {
		url, err := url.Parse(referer)
		if err != nil {
			return false
		}
		hostname = url.Hostname()
	} else {
		hostname = req.URL.Hostname()
	}

	matches := false
	for _, entry := range m.entries {
		if entry.MatchDomain(hostname) {
			matches = true
			break
		}
	}
	if m.inverted {
		return !matches
	}
	return matches
}

func (m *DomainModifier) ShouldMatchRes(_ *http.Response) bool {
	return false
}

type domainModifierEntry struct {
	regular string
	tld     string
	regexp  *regexp.Regexp
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
		m.tld = strings.TrimSuffix(entry, ".*")
		if len(m.tld) == 0 {
			return errors.New("tld is empty")
		}
		return nil
	}

	m.regular = entry
	return nil
}

func (m *domainModifierEntry) MatchDomain(domain string) bool {
	switch {
	case m.regular != "":
		return m.regular == domain || strings.HasSuffix(domain, "."+m.regular)
	case m.tld != "":
		segments := strings.Split(domain, ".")
		return (len(segments) > 1 && segments[len(segments)-2] == m.tld) || (len(segments) > 2 && segments[len(segments)-3] == m.tld)
	case m.regexp != nil:
		return m.regexp.MatchString(domain)
	default:
		return false
	}
}

func (m *DomainModifier) Cancels(modifier Modifier) bool {
	other, ok := modifier.(*DomainModifier)
	if !ok || len(m.entries) != len(other.entries) || m.inverted != other.inverted {
		return false
	}

	used := make(map[int]struct{}, len(other.entries))

	for _, entry := range m.entries {
		matchFound := false
		for i, otherEntry := range other.entries {
			if _, alreadyUsed := used[i]; alreadyUsed {
				continue
			}
			if entryEqual(entry, otherEntry) {
				used[i] = struct{}{}
				matchFound = true
				break
			}
		}
		if !matchFound {
			return false
		}
	}

	return true
}

func entryEqual(a, b domainModifierEntry) bool {
	if a.regular != b.regular || a.tld != b.tld {
		return false
	}

	if a.regexp == nil && b.regexp == nil {
		return true
	}
	if a.regexp == nil || b.regexp == nil {
		return false
	}
	return a.regexp.String() == b.regexp.String()
}
