package cssrule

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/hostmatch"
	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

var (
	RuleRegex          = regexp.MustCompile(`.*#@?\$#.+`)
	primaryRuleRegex   = regexp.MustCompile(`(.*?)#\$#(.*)`)
	exceptionRuleRegex = regexp.MustCompile(`(.*?)#@\$#(.+)`)
)

type store interface {
	AddPrimaryRule(hostnamePatterns string, css string) error
	AddExceptionRule(hostnamePatterns string, css string) error
	Get(hostname string) []string
}

type Injector struct {
	store store
}

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

// GetAsset returns the CSS asset for the given hostname.
func (inj *Injector) GetAsset(hostname string) []byte {
	cssRules := inj.store.Get(hostname)
	log.Printf("got %d css rules for %q", len(cssRules), redacted.Redacted(hostname))
	if len(cssRules) == 0 {
		return nil
	}

	stylesheet := strings.Join(cssRules, "")
	return []byte(stylesheet)
}
