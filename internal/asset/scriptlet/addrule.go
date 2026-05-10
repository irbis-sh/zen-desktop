package scriptlet

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	// RuleRegex matches patterns for scriptlet rules in two formats:
	//
	//  1. #%#//scriptlet or #@%#//scriptlet for canonical rules.
	//  2. ##+js or #@#+js for uBlock-style rules.
	RuleRegex = regexp.MustCompile(`(?:#@?%#\/\/scriptlet)|(?:#@?#\+js)`)

	canonicalPrimary        = regexp.MustCompile(`(.*)#%#\/\/scriptlet\((.+)\)`)
	canonicalExceptionRegex = regexp.MustCompile(`(.*)#@%#\/\/scriptlet\((.+)\)`)
	ublockPrimaryRegex      = regexp.MustCompile(`(.*)##\+js\((.+)\)`)
	ublockExceptionRegex    = regexp.MustCompile(`(.*)#@#\+js\((.+)\)`)
	errUnsupportedSyntax    = errors.New("unsupported syntax")
	errUntrusted            = errors.New("trusted scriptlet in an untrusted filter list")
)

func (inj *Injector) AddRule(rule string, filterListTrusted bool) error {
	var al argList
	var isException bool
	var hostnamePatterns string

	if match := canonicalPrimary.FindStringSubmatch(rule); match != nil {
		hostnamePatterns = match[1]
		al = argList(match[2])
	} else if match := canonicalExceptionRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatterns = match[1]
		al = argList(match[2])
		isException = true
	} else if match := ublockPrimaryRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatterns = match[1]
		al = argList(match[2]).ConvertUboToCanonical()
	} else if match := ublockExceptionRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatterns = match[1]
		al = argList(match[2]).ConvertUboToCanonical()
		isException = true
	} else {
		return errUnsupportedSyntax
	}

	var err error
	al, err = al.Normalize()
	if err != nil {
		return fmt.Errorf("normalize argList: %v", err)
	}

	if !filterListTrusted && al.IsTrusted() {
		return errUntrusted
	}

	switch isException {
	case true:
		inj.store.AddExceptionRule(hostnamePatterns, al)
	case false:
		inj.store.AddPrimaryRule(hostnamePatterns, al)
	}

	return nil
}
