package sysproxy

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/constants"
)

// TestRenderPacLocalEndpointWinsOverExclusions pins two properties of the local
// endpoint's PAC branch: it precedes exclusion matching, so excluding a parent
// domain cannot route the endpoint DIRECT (where nothing answers for it), and
// it carries no DIRECT fallback, because a direct connection to the endpoint
// can only fail. The matching tolerates reformatting of the template: only the
// carve-out's shape and its position relative to the dnsDomainIs matching
// matter, not the exact layout.
func TestRenderPacLocalEndpointWinsOverExclusions(t *testing.T) {
	t.Parallel()

	pac := string(renderPac(1234, []string{"irbis.sh"}))

	carveOutRe := regexp.MustCompile(
		fmt.Sprintf(`if \(host == "%s"\)\s*\{\s*return "PROXY 127\.0\.0\.1:1234";\s*\}`,
			regexp.QuoteMeta(constants.LocalEndpointHost)))
	carveOut := carveOutRe.FindStringIndex(pac)
	if carveOut == nil {
		t.Fatalf("PAC lacks the local endpoint carve-out (want a match for %q):\n%s", carveOutRe, pac)
	}
	if block := pac[carveOut[0]:carveOut[1]]; strings.Contains(block, "DIRECT") {
		t.Fatalf("local endpoint carve-out contains a DIRECT fallback:\n%s", block)
	}

	exclusionsIndex := strings.Index(pac, "dnsDomainIs(")
	if exclusionsIndex == -1 {
		t.Fatalf("PAC lacks exclusion matching:\n%s", pac)
	}

	if carveOut[0] > exclusionsIndex {
		t.Fatalf("local endpoint carve-out at %d comes after exclusion matching at %d:\n%s", carveOut[0], exclusionsIndex, pac)
	}
}
