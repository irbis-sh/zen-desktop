package extendedcss

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/irbis-sh/zen-desktop/internal/hostmatch"
	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

var (
	primaryRuleRegex   = regexp.MustCompile(`(.+?)#\??#(.+)`)
	exceptionRuleRegex = regexp.MustCompile(`(.+?)#@\??#(.+)`)

	//go:embed bundle.js
	defaultExtendedCSSBundle []byte
)

type store interface {
	AddPrimaryRule(hostnamePatterns string, body string) error
	AddExceptionRule(hostnamePatterns string, body string) error
	Get(hostname string) []string
}

// Injector injects extended CSS rules into HTML HTTP responses.
type Injector struct {
	// bundle contains the extended CSS JS bundle.
	bundle []byte
	// store stores and retrieves extended CSS rules by hostname.
	store store
}

func NewInjectorWithDefaults() (*Injector, error) {
	store := hostmatch.NewHostMatcher[string]()
	return newInjector(defaultExtendedCSSBundle, store)
}

func newInjector(bundleData []byte, store store) (*Injector, error) {
	if bundleData == nil {
		return nil, errors.New("bundleData is nil")
	}
	if store == nil {
		return nil, errors.New("store is nil")
	}

	return &Injector{
		bundle: bundleData,
		store:  store,
	}, nil
}

// AddRule adds an extended CSS rule to the injector.
func (inj *Injector) AddRule(rule string) error {
	if match := primaryRuleRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatters := match[1]
		selector := match[2]
		if err := inj.store.AddPrimaryRule(hostnamePatters, selector); err != nil {
			return fmt.Errorf("add primary rule: %v", err)
		}
		return nil
	} else if match := exceptionRuleRegex.FindStringSubmatch(rule); match != nil {
		hostnamePatterns := match[1]
		selector := match[2]
		if err := inj.store.AddExceptionRule(hostnamePatterns, selector); err != nil {
			return fmt.Errorf("add exception rule: %v", err)
		}
		return nil
	}
	return errors.New("unknown rule format")
}

// GetAsset returns the JS asset for the given hostname.
func (inj *Injector) GetAsset(hostname string) ([]byte, error) {
	rules := inj.store.Get(hostname)
	log.Printf("got %d extended-css rules for %q", len(rules), redacted.Redacted(hostname))
	if len(rules) == 0 {
		return nil, nil
	}

	joined := strings.Join(rules, "\n")
	encodedRules, err := json.Marshal(joined)
	if err != nil {
		return nil, fmt.Errorf("encode rules: %v", err)
	}

	var injection bytes.Buffer
	injection.Write(inj.bundle)
	injection.WriteString("\n(()=>{window.extendedCSS(")
	injection.Write(encodedRules)
	injection.WriteString(")})();")

	return injection.Bytes(), nil
}
