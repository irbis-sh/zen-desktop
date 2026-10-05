//go:build prod

package hwkey

import (
	"errors"
	"log"
)

const (
	backendName  = "tpm"
	providerName = "Microsoft Platform Crypto Provider"

	// probeKeyName names the throwaway key Available creates and deletes.
	probeKeyName = "Zen hardware key probe"
)

func available() error {
	// Create, finalise and delete a named key. Finalising is the step that fails on machines
	// without a usable TPM. A named key is used rather than an ephemeral one because it is the
	// exact path Create takes, so a passing probe means Create works too.
	s, err := create(probeKeyName)
	if err != nil {
		log.Printf("hardware key unavailable: %v", err)
		return errors.New("no TPM 2.0")
	}
	s.Close()
	if err := remove(probeKeyName); err != nil {
		log.Printf("delete hardware key probe: %v", err)
	}
	return nil
}
