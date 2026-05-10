package scriptlet

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"log"

	"github.com/irbis-sh/zen-desktop/internal/hostmatch"
	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

var (
	//go:embed bundle.js
	defaultScriptletsBundle []byte
)

type store interface {
	AddPrimaryRule(hostnamePatterns string, body argList) error
	AddExceptionRule(hostnamePatterns string, body argList) error
	Get(hostname string) []argList
}

// Injector injects scriptlets into HTML HTTP responses.
type Injector struct {
	// bundle contains the scriptlets JS bundle.
	bundle []byte
	// store stores and retrieves scriptlets by hostname.
	store store
}

func NewInjectorWithDefaults() (*Injector, error) {
	store := hostmatch.NewHostMatcher[argList]()
	return newInjector(defaultScriptletsBundle, store)
}

// newInjector creates a new Injector with the embedded scriptlets.
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

// GetAsset returns the scriptlet injection asset for the given hostname.
func (inj *Injector) GetAsset(hostname string) ([]byte, error) {
	argLists := inj.store.Get(hostname)
	log.Printf("got %d scriptlets for %q", len(argLists), redacted.Redacted(hostname))
	if len(argLists) == 0 {
		return nil, nil
	}

	var injection bytes.Buffer
	injection.Write(inj.bundle)
	injection.WriteString("(()=>{")
	for _, argLst := range argLists {
		if err := argLst.GenerateInjection(&injection); err != nil {
			return nil, fmt.Errorf("generate injection for scriptlet %q: %v", argLst, err)
		}
	}
	injection.WriteString("})();")

	return injection.Bytes(), nil
}
