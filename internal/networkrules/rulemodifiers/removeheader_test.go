package rulemodifiers

import (
	"errors"
	"net/http"
	"testing"
)

func TestRemoveHeaderModifier(t *testing.T) {
	t.Parallel()

	t.Run("returns error if input is invalid", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("notremoveheader"); err == nil {
			t.Error("error should be non-nil")
		} else if !errors.Is(err, ErrInvalidRemoveheaderModifier) {
			t.Errorf("err should be ErrInvalidModifier, is %s", err)
		}
	})

	t.Run("returns error on forbidden header", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("removeheader=Permissions-Policy"); err == nil {
			t.Errorf("error should be non-nil")
		} else if !errors.Is(err, ErrForbiddenHeader) {
			t.Errorf("error should be ErrForbiddenHeader, is %s", err)
		}
	})

	t.Run("returns error on forbidden request header", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("removeheader=request:accept"); err == nil {
			t.Error("error should be non-nil")
		} else if !errors.Is(err, ErrForbiddenHeader) {
			t.Errorf("error should be ErrForbiddenError, is %s", err)
		}
	})

	t.Run("returns error on forbidden request header in non-canonical form", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("removeheader=access-control-aLLow-oRigin"); err == nil {
			t.Error("error should be non-nil")
		} else if !errors.Is(err, ErrForbiddenHeader) {
			t.Errorf("error should be ErrForbiddenError, is %s", err)
		}
	})

	t.Run("removes request header", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("removeheader=request:Authorization"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		req := &http.Request{Header: http.Header{"Authorization": []string{"value"}}}
		if !rm.ModifyReq(req) {
			t.Error("expected request to be modified")
		}

		if req.Header.Get("Authorization") != "" {
			t.Error("expected request header to be removed")
		}
	})

	t.Run("doesn't report removing request header if it doesn't exist", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("removeheader=request:Authorization"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		req := &http.Request{Header: http.Header{}}
		if rm.ModifyReq(req) {
			t.Error("expected request to not be modified")
		}
	})

	t.Run("removes response header", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("removeheader=Refresh"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res := &http.Response{Header: http.Header{"Refresh": []string{"value1", "value2"}}}
		m, err := rm.ModifyRes(res)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !m {
			t.Error("expected response to be modified")
		}

		if res.Header.Get("Refresh") != "" {
			t.Error("expected response header to be removed")
		}
	})

	t.Run("doesn't report removing response header if it doesn't exist", func(t *testing.T) {
		t.Parallel()

		rm := &RemoveHeaderModifier{}
		if err := rm.Parse("removeheader=Refresh"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res := &http.Response{Header: http.Header{}}
		m, err := rm.ModifyRes(res)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m {
			t.Error("expected response to not be modified")
		}
	})

	t.Run("cancels", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			a        RemoveHeaderModifier
			b        RemoveHeaderModifier
			expected bool
		}{
			{
				"Should cancel - identical modifiers",
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "X-Test",
				},
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "X-Test",
				},
				true,
			},
			{
				"Should cancel - empty",
				RemoveHeaderModifier{},
				RemoveHeaderModifier{},
				true,
			},
			{
				"Should not cancel - different HeaderName",
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "X-Test",
				},
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "X-Different",
				},
				false,
			},
			{
				"Should not cancel - different Kind",
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "X-Test",
				},
				RemoveHeaderModifier{
					Kind:       2,
					HeaderName: "X-Test",
				},
				false,
			},
			{
				"Should not cancel - different Kind and HeaderName",
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "X-Test",
				},
				RemoveHeaderModifier{
					Kind:       2,
					HeaderName: "X-Other",
				},
				false,
			},
			{
				"Should not cancel - empty HeaderName",
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "",
				},
				RemoveHeaderModifier{
					Kind:       1,
					HeaderName: "X-Test",
				},
				false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				result := tt.a.Cancels(&tt.b)
				if result != tt.expected {
					t.Errorf("RemoveHeaderModifier.Cancels() = %t, want %t", result, tt.expected)
				}
			})
		}
	})
}
