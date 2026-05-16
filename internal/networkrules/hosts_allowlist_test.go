package networkrules

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostsAllowlistRuleCancelsHostsBlock(t *testing.T) {
	t.Parallel()

	nr := New()

	isException, err := nr.ParseRule("0.0.0.0 example.com", nil)
	if err != nil {
		t.Fatalf("ParseRule(hosts) error: %v", err)
	}
	if isException {
		t.Fatal("hosts rule unexpectedly parsed as exception")
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Sec-Fetch-Dest", "document")

	_, blocked, _ := nr.ModifyReq(req)
	if !blocked {
		t.Fatal("expected hosts rule to block request")
	}

	filterName := "Allowlist"
	isException, err = nr.ParseRule("@@0.0.0.0 example.com", &filterName)
	if err != nil {
		t.Fatalf("ParseRule(hosts exception) error: %v", err)
	}
	if !isException {
		t.Fatal("hosts exception was not parsed as exception")
	}

	allowedReq := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	allowedReq.Header.Set("Sec-Fetch-User", "?1")
	allowedReq.Header.Set("Sec-Fetch-Dest", "document")

	_, blocked, _ = nr.ModifyReq(allowedReq)
	if blocked {
		t.Fatal("expected hosts exception to allow request")
	}
}
