package scriptlet

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
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

// trustedPrefix marks scriptlets that may only come from trusted filter lists.
const trustedPrefix = "trusted-"

func (inj *Injector) AddRule(rule string, filterListTrusted bool) error {
	var body string
	var isUblock, isException bool
	var hostnamePatterns string

	if match := canonicalPrimary.FindStringSubmatch(rule); match != nil {
		hostnamePatterns, body = match[1], match[2]
	} else if match := canonicalExceptionRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatterns, body = match[1], match[2]
		isException = true
	} else if match := ublockPrimaryRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatterns, body = match[1], match[2]
		isUblock = true
	} else if match := ublockExceptionRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatterns, body = match[1], match[2]
		isUblock = true
		isException = true
	} else {
		return errUnsupportedSyntax
	}

	var args []string
	var err error
	if isUblock {
		args, err = parseUboArgList(body)
	} else {
		args, err = parseCanonicalArgList(body)
	}
	if err != nil {
		return fmt.Errorf("parse argument list: %v", err)
	}

	if !filterListTrusted && strings.HasPrefix(args[0], trustedPrefix) {
		return errUntrusted
	}

	al, err := newArgList(args)
	if err != nil {
		return fmt.Errorf("encode argument list: %v", err)
	}

	switch isException {
	case true:
		inj.store.AddExceptionRule(hostnamePatterns, al)
	case false:
		inj.store.AddPrimaryRule(hostnamePatterns, al)
	}

	return nil
}
