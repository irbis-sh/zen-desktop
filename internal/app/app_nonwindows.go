//go:build !windows

package app

import "context"

func (a *App) Startup(ctx context.Context) {
	a.commonStartup(ctx)
}
