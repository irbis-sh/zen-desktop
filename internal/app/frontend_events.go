package app

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type frontendEvents struct {
	ctx context.Context
	// recordFilterHit is called once per request handled by an ad-blocking
	// filter list. It is wired to the shared stats counter.
	recordFilterHit func()
}

func newFrontendEvents(ctx context.Context, recordFilterHit func()) *frontendEvents {
	return &frontendEvents{ctx: ctx, recordFilterHit: recordFilterHit}
}

func (e *frontendEvents) emit(channel string, payload any) {
	runtime.EventsEmit(e.ctx, channel, payload)
}
