package certstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The tests swap systemTrustCandidates, so they cannot run in parallel.

func TestGetSystemTrustInfoNoStore(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	swapSystemTrustCandidates(t, []systemTrustCandidate{
		{dir: missing, ext: ".pem", command: []string{"update-ca-trust", "extract"}},
	})

	_, _, err := getSystemTrustInfo()
	if !errors.Is(err, ErrNoSystemTrustStore) {
		t.Fatalf("got error %v, want ErrNoSystemTrustStore", err)
	}
}

func TestGetSystemTrustInfoFirstMatchWins(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	first := makeTrustDir(t)
	second := makeTrustDir(t)
	swapSystemTrustCandidates(t, []systemTrustCandidate{
		{dir: missing, ext: ".pem", command: []string{"skipped"}},
		{dir: first, ext: ".crt", command: []string{"update-ca-certificates"}},
		{dir: second, ext: ".pem", command: []string{"unreachable"}},
	})

	certFilename, command, err := getSystemTrustInfo()
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	if want := filepath.Join(first, systemTrustFilename+".crt"); certFilename != want {
		t.Errorf("got cert filename %q, want %q", certFilename, want)
	}
	if want := []string{"update-ca-certificates"}; !reflect.DeepEqual(command, want) {
		t.Errorf("got command %v, want %v", command, want)
	}
}

func swapSystemTrustCandidates(t *testing.T, candidates []systemTrustCandidate) {
	t.Helper()
	orig := systemTrustCandidates
	systemTrustCandidates = candidates
	t.Cleanup(func() {
		systemTrustCandidates = orig
	})
}

func makeTrustDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "anchors")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
