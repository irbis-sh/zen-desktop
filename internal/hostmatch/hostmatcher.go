package hostmatch

import (
	"errors"
	"strings"
	"sync"
)

var (
	errNoEmptyPattern = errors.New("empty patterns are not allowed")
)

type hostnameStore[T any] interface {
	Add(hostnamePattern string, data T)
	Get(hostname string) []T
}

// HostMatcher matches rules against hostnames, supporting wildcards and exceptions.
// It is safe for concurrent use.
type HostMatcher[T comparable] struct {
	// mu protects generic and genericExceptions slices.
	mu                sync.RWMutex
	generic           []T
	genericExceptions []T
	primaryStore      hostnameStore[T]
	exceptionStore    hostnameStore[T]
}

func NewHostMatcher[T comparable]() *HostMatcher[T] {
	return &HostMatcher[T]{
		primaryStore:   newTrieStore[T](),
		exceptionStore: newTrieStore[T](),
	}
}

func (hm *HostMatcher[T]) AddPrimaryRule(hostnamePatterns string, data T) error {
	if len(hostnamePatterns) == 0 {
		hm.mu.Lock()
		hm.generic = append(hm.generic, data)
		hm.mu.Unlock()
		return nil
	}

	patterns := strings.Split(hostnamePatterns, ",")
	for _, pattern := range patterns {
		if len(pattern) == 0 {
			return errNoEmptyPattern
		}
	}
	for _, pattern := range patterns {
		if pattern[0] == '~' {
			pattern = pattern[1:]
			hm.exceptionStore.Add(pattern, data)
			if !strings.HasPrefix(pattern, "*.") {
				hm.exceptionStore.Add("*."+pattern, data)
			}
			continue
		}

		hm.primaryStore.Add(pattern, data)
		if !strings.HasPrefix(pattern, "*.") {
			hm.primaryStore.Add("*."+pattern, data)
		}
	}

	return nil
}

func (hm *HostMatcher[T]) AddExceptionRule(hostnamePatterns string, data T) error {
	if len(hostnamePatterns) == 0 {
		hm.mu.Lock()
		hm.genericExceptions = append(hm.genericExceptions, data)
		hm.mu.Unlock()
		return nil
	}

	patterns := strings.Split(hostnamePatterns, ",")
	for _, pattern := range patterns {
		if len(pattern) == 0 {
			return errNoEmptyPattern
		}

		hm.exceptionStore.Add(pattern, data)
		if !strings.HasPrefix(pattern, "*.") {
			hm.exceptionStore.Add("*."+pattern, data)
		}
	}

	return nil
}

func (hm *HostMatcher[T]) Get(hostname string) []T {
	primary := hm.primaryStore.Get(hostname)
	exceptions := hm.exceptionStore.Get(hostname)

	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if len(hm.genericExceptions) == 0 && len(exceptions) == 0 {
		// Optimize the most common case.
		res := make([]T, len(hm.generic)+len(primary))
		copy(res, hm.generic)
		copy(res[len(hm.generic):], primary)
		return res
	}

	exceptionMap := make(map[T]struct{}, len(hm.genericExceptions)+len(exceptions))
	for _, ex := range hm.genericExceptions {
		exceptionMap[ex] = struct{}{}
	}
	for _, ex := range exceptions {
		exceptionMap[ex] = struct{}{}
	}

	var res []T
	for _, r := range hm.generic {
		if _, excluded := exceptionMap[r]; !excluded {
			res = append(res, r)
		}
	}
	for _, r := range primary {
		if _, excluded := exceptionMap[r]; !excluded {
			res = append(res, r)
		}
	}

	return res
}
