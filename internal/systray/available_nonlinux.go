//go:build !linux

package systray

func Available() bool { return true }
