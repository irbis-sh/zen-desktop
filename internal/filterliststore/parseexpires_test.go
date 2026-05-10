package filterliststore

import (
	"testing"
	"time"
)

func TestParseExpires(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantOffset  time.Duration
		errExpected bool
	}{
		{
			name:       "4 days",
			input:      "! Expires: 4 days",
			wantOffset: 4 * 24 * time.Hour,
		},
		{
			name:       "4 day",
			input:      "! Expires: 4 day",
			wantOffset: 4 * 24 * time.Hour,
		},
		{
			name:       "12 hours",
			input:      "! Expires: 12 hours",
			wantOffset: 12 * time.Hour,
		},
		{
			name:       "12 hour",
			input:      "! Expires: 12 hour",
			wantOffset: 12 * time.Hour,
		},
		{
			name:       "5d shorthand",
			input:      "! Expires: 5d",
			wantOffset: 5 * 24 * time.Hour,
		},
		{
			name:       "18h shorthand",
			input:      "! Expires: 18h",
			wantOffset: 18 * time.Hour,
		},
		{
			name:        "invalid input, no match",
			input:       "no expires here",
			errExpected: true,
		},
		{
			name:       "zero duration",
			input:      "! Expires: 0 days",
			wantOffset: 0,
		},
		{
			name:        "unsupported unit",
			input:       "! Expires: 10 weeks",
			errExpected: true,
		},
		{
			name:        "whitespace and mixed case",
			input:       "   ! expires: 2 HOURS   ",
			errExpected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotTime, err := parseExpires([]byte(tt.input))

			if err != nil {
				if tt.errExpected {
					return
				}
				t.Errorf("expected no error, got %v", err)
			}

			if tt.errExpected {
				t.Error("expected error, got nil")
			}

			if gotTime != tt.wantOffset {
				t.Errorf("expected %v, got %v", tt.wantOffset, gotTime)
			}
		})
	}
}
