package cosmetic

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
	primaryRuleRegex   = regexp.MustCompile(`(.*?)##(.*)`)
	exceptionRuleRegex = regexp.MustCompile(`(.*?)#@#(.+)`)
)

type store interface {
	AddPrimaryRule(hostnamePatterns string, selector string) error
	AddExceptionRule(hostnamePatterns string, selector string) error
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
		css, err := sanitizeCSSSelector(match[2])
		if err != nil {
			return fmt.Errorf("sanitize css selector: %w", err)
		}
		if err := inj.store.AddPrimaryRule(match[1], css); err != nil {
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
	selectors := inj.store.Get(hostname)
	log.Printf("got %d cosmetic rules for %q", len(selectors), redacted.Redacted(hostname))
	if len(selectors) == 0 {
		return nil
	}

	stylesheet := generateBatchedCSS(selectors)
	return []byte(stylesheet)
}

func generateBatchedCSS(selectors []string) string {
	const batchSize = 100

	var builder strings.Builder
	for i := 0; i < len(selectors); i += batchSize {
		end := i + batchSize
		if end > len(selectors) {
			end = len(selectors)
		}
		batch := selectors[i:end]

		joinedSelectors := strings.Join(batch, ",")
		fmt.Fprintf(&builder, "%s{display:none!important;}", joinedSelectors)
	}

	return builder.String()
}
