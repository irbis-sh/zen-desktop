package contenttype

import (
	"net/http"
	"testing"
)

func TestFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers http.Header
		want    Type
	}{
		{"document", http.Header{"Sec-Fetch-Dest": {"document"}}, Document},
		{"empty", http.Header{"Sec-Fetch-Dest": {"empty"}}, XMLHTTPRequest},
		{"font", http.Header{"Sec-Fetch-Dest": {"font"}}, Font},
		{"frame", http.Header{"Sec-Fetch-Dest": {"frame"}}, Subdocument},
		{"iframe", http.Header{"Sec-Fetch-Dest": {"iframe"}}, Subdocument},
		{"image", http.Header{"Sec-Fetch-Dest": {"image"}}, Image},
		{"object", http.Header{"Sec-Fetch-Dest": {"object"}}, Object},
		{"script", http.Header{"Sec-Fetch-Dest": {"script"}}, Script},
		{"style", http.Header{"Sec-Fetch-Dest": {"style"}}, Stylesheet},
		{"audio", http.Header{"Sec-Fetch-Dest": {"audio"}}, Media},
		{"track", http.Header{"Sec-Fetch-Dest": {"track"}}, Media},
		{"video", http.Header{"Sec-Fetch-Dest": {"video"}}, Media},
		{"websocket", http.Header{"Sec-Fetch-Dest": {"websocket"}}, WebSocket},
		{"unmapped Sec-Fetch-Dest", http.Header{"Sec-Fetch-Dest": {"manifest"}}, Other},
		{"no headers", http.Header{}, Unknown},
		{
			name: "ping takes precedence over Sec-Fetch-Dest",
			headers: http.Header{
				"Content-Type":   {"text/ping; charset=utf-8"},
				"Ping-To":        {"http://example.com"},
				"Sec-Fetch-Dest": {"empty"},
			},
			want: Ping,
		},
		{
			name:    "ping Content-Type without ping headers",
			headers: http.Header{"Content-Type": {"text/ping"}, "Sec-Fetch-Dest": {"empty"}},
			want:    XMLHTTPRequest,
		},
		{
			name:    "WebSocket upgrade without Sec-Fetch-Dest",
			headers: http.Header{"Connection": {"keep-alive, Upgrade"}, "Upgrade": {"websocket"}},
			want:    WebSocket,
		},
		{
			name:    "non-WebSocket upgrade without Sec-Fetch-Dest",
			headers: http.Header{"Connection": {"upgrade"}, "Upgrade": {"h2c"}},
			want:    Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := FromRequest(&http.Request{Header: tt.headers}); got != tt.want {
				t.Errorf("FromRequest() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFromResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reqHeaders  http.Header
		contentType string
		want        Type
	}{
		{"request type wins over MIME type", http.Header{"Sec-Fetch-Dest": {"document"}}, "text/javascript", Document},
		{"exact MIME type", http.Header{}, "text/css", Stylesheet},
		{"top-level MIME type", http.Header{}, "image/jpeg", Image},
		{"MIME type with parameters", http.Header{}, "image/jpeg; charset=utf-8", Image},
		{"mixed-case MIME type", http.Header{}, "Application/JavaScript", Script},
		{"unmapped MIME type", http.Header{}, "application/weird", Other},
		{"no Content-Type", http.Header{}, "", Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := &http.Response{Header: http.Header{}}
			if tt.contentType != "" {
				res.Header.Set("Content-Type", tt.contentType)
			}
			if got := FromResponse(&http.Request{Header: tt.reqHeaders}, res); got != tt.want {
				t.Errorf("FromResponse() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMaskMatches(t *testing.T) {
	t.Parallel()

	if !Mask(0).Matches(Unknown) {
		t.Error("zero Mask does not match Unknown, want match")
	}
	if All.Matches(Unknown) {
		t.Error("All matches Unknown, want no match")
	}
	for _, typ := range modifierTypes {
		if !All.Matches(typ) {
			t.Errorf("All does not match %d, want match", typ)
		}
	}
	if m := Mask(Script | Image); m.Matches(Stylesheet) || !m.Matches(Image) {
		t.Errorf("Mask(Script|Image) mismatches Stylesheet or Image")
	}
}
