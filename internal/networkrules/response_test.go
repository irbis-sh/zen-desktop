package networkrules

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
)

func TestCreateBlockPageResponse(t *testing.T) {
	t.Parallel()

	nr := New()
	filterList := "Test List"
	req := newTestRequest(t, "https://ads.example.com/banner", nil)

	res, err := nr.CreateBlockPageResponse(req, []rule.Rule{{RawRule: "||ads.example.com^", FilterName: &filterList}}, 8080)
	if err != nil {
		t.Fatalf("create block page response: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	page := string(body)

	if !strings.Contains(page, "||ads.example.com^") {
		t.Error("body does not contain the applied rule")
	}
	if !strings.Contains(page, "Test List") {
		t.Error("body does not contain the filter list name")
	}
	if !strings.Contains(page, ".card {") {
		t.Error("body does not contain the shared stylesheet")
	}
}
