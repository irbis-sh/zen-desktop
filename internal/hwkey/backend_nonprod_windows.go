//go:build !prod

package hwkey

// Development builds keep the key in the software key storage provider, so the same NCrypt code
// runs on machines without a TPM.

const (
	backendName  = "software key storage"
	providerName = "Microsoft Software Key Storage Provider"
)

func available() error {
	return nil
}
