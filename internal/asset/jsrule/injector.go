package jsrule

import (
	"errors"
	"fmt"
	"log"
	"regexp"

	"github.com/irbis-sh/zen-desktop/internal/hostmatch"
	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

type store interface {
	AddPrimaryRule(hostnamePatterns string, script string) error
	AddExceptionRule(hostnamePatterns string, script string) error
	Get(hostname string) []string
}

type Injector struct {
	store store
}

var (
	RuleRegex          = regexp.MustCompile(`.*#@?%#.+`)
	primaryRuleRegex   = regexp.MustCompile(`(.*)#%#(.+)`)
	exceptionRuleRegex = regexp.MustCompile(`(.*)#@%#(.+)`)

	injectionStart = []byte("(function(){")
	injectionEnd   = []byte("})();")
)

func NewInjector() *Injector {
	return &Injector{
		store: hostmatch.NewHostMatcher[string](),
	}
}

func (inj *Injector) AddRule(rule string) error {
	if match := primaryRuleRegex.FindStringSubmatch(rule); match != nil {
		if err := inj.store.AddPrimaryRule(match[1], match[2]); err != nil {
			return fmt.Errorf("add primary rule: %w", err)
		}
		return nil
	}

	if match := exceptionRuleRegex.FindStringSubmatch(rule); match != nil {
		if err := inj.store.AddExceptionRule(match[1], match[2]); err != nil {
			return fmt.Errorf("add exception rule: %w", err)
		}
		return nil
	}

	return errors.New("unsupported syntax")
}

// GetAsset returns the JS asset for the given hostname.
func (inj *Injector) GetAsset(hostname string) ([]byte, error) {
	scripts := inj.store.Get(hostname)
	log.Printf("got %d js rules for %q", len(scripts), redacted.Redacted(hostname))
	if len(scripts) == 0 {
		return nil, nil
	}

	var injection []byte
	injection = append(injection, injectionStart...)
	for _, script := range scripts {
		injection = append(injection, script...)
		if len(script) > 0 && script[len(script)-1] != ';' {
			injection = append(injection, ';')
		}
	}
	injection = append(injection, injectionEnd...)

	return injection, nil
}
