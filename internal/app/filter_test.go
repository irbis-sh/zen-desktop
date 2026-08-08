package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/filter"
	"github.com/irbis-sh/zen-desktop/internal/filterliststore"
)

func TestRunBuildPasses(t *testing.T) {
	t.Parallel()

	errCreate := errors.New("create filter: boom")

	tests := []struct {
		name string
		// expired marks the build deadline as already spent before pass 1.
		expired bool
		// aborted marks the build as aborted before pass 1.
		aborted bool
		// script holds one entry per expected pass.
		script []scriptedPass
		// wantModes doubles as the expected pass count.
		wantModes []filterliststore.FetchMode
		wantErr   error
	}{
		{
			name:      "clean first pass is accepted",
			script:    []scriptedPass{{}},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault},
		},
		{
			name: "failed lists alone do not trigger a rebuild",
			script: []scriptedPass{
				{outcome: filter.Outcome{Failed: true, Err: errors.New("no network and no cache")}},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault},
		},
		{
			name: "truncated pass is rebuilt under the next mode",
			script: []scriptedPass{
				{outcome: filter.Outcome{Truncated: true}},
				{},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault, filterliststore.ModePreferCache},
		},
		{
			name: "persistent truncation stops at the pass cap",
			script: []scriptedPass{
				{outcome: filter.Outcome{Truncated: true}},
				{outcome: filter.Outcome{Truncated: true}},
				{outcome: filter.Outcome{Truncated: true}},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault, filterliststore.ModePreferCache, filterliststore.ModeCacheOnly},
		},
		{
			// Failed lists may still have cached copies the spent deadline
			// kept Get from serving; the rescue pass must run and must be
			// cache-only.
			name: "deadline expiry with failed lists rebuilds cache-only",
			script: []scriptedPass{
				{outcome: filter.Outcome{Failed: true}, expireDeadline: true},
				{},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault, filterliststore.ModeCacheOnly},
		},
		{
			name: "truncation then deadline expiry still ends cache-only",
			script: []scriptedPass{
				{outcome: filter.Outcome{Truncated: true}},
				{outcome: filter.Outcome{Failed: true}, expireDeadline: true},
				{},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault, filterliststore.ModePreferCache, filterliststore.ModeCacheOnly},
		},
		{
			// A failed cache-only pass must not loop: with the network off the
			// table there is nothing left to ladder down to.
			name:    "spent deadline forces cache-only from the first pass",
			expired: true,
			script: []scriptedPass{
				{outcome: filter.Outcome{Failed: true}},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeCacheOnly},
		},
		{
			name: "construction error aborts the build",
			script: []scriptedPass{
				{err: errCreate},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault},
			wantErr:   errCreate,
		},
		{
			// StopProxy is waiting on proxyMu; a rebuild would only prolong
			// the teardown it is about to perform.
			name: "abort mid-pass stops the ladder",
			script: []scriptedPass{
				{outcome: filter.Outcome{Truncated: true}, abort: true},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault},
			wantErr:   errBuildAborted,
		},
		{
			name:      "abort before the first pass runs nothing",
			aborted:   true,
			wantModes: nil,
			wantErr:   errBuildAborted,
		},
		{
			// Deadline expiry and abort are separate signals precisely so a
			// late abort is not masked: on a shared context the deadline's
			// cause would win and the rescue pass would start the proxy the
			// user just asked to stop.
			name: "abort after deadline expiry still unwinds",
			script: []scriptedPass{
				{outcome: filter.Outcome{Failed: true}, expireDeadline: true},
				{abort: true},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault, filterliststore.ModeCacheOnly},
			wantErr:   errBuildAborted,
		},
		{
			name: "abort wins over a coinciding construction error",
			script: []scriptedPass{
				{err: errCreate, abort: true},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault},
			wantErr:   errBuildAborted,
		},
		{
			// Even a clean final pass must not be accepted after an abort:
			// StartProxy would go on to start servers the waiting StopProxy
			// immediately tears down.
			name: "abort during the final pass beats acceptance",
			script: []scriptedPass{
				{outcome: filter.Outcome{Truncated: true}},
				{outcome: filter.Outcome{Truncated: true}},
				{abort: true},
			},
			wantModes: []filterliststore.FetchMode{filterliststore.ModeDefault, filterliststore.ModePreferCache, filterliststore.ModeCacheOnly},
			wantErr:   errBuildAborted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			aborted := tt.aborted
			if tt.expired {
				cancel()
			}

			var gotModes []filterliststore.FetchMode
			err := runBuildPasses(ctx, func() bool { return aborted }, func(_ context.Context, mode filterliststore.FetchMode) (filter.Outcome, error) {
				pass := len(gotModes)
				gotModes = append(gotModes, mode)
				if pass >= len(tt.script) {
					t.Fatalf("unexpected pass %d in mode %v", pass+1, mode)
				}
				step := tt.script[pass]
				if step.expireDeadline {
					cancel()
				}
				if step.abort {
					aborted = true
				}
				return step.outcome, step.err
			})

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(gotModes, tt.wantModes) {
				t.Errorf("pass modes = %v, want %v", gotModes, tt.wantModes)
			}
		})
	}
}

// scriptedPass drives one expected pass of runBuildPasses.
type scriptedPass struct {
	outcome filter.Outcome
	err     error
	// expireDeadline spends the build deadline while this pass is running.
	expireDeadline bool
	// abort aborts the build while this pass is running.
	abort bool
}
