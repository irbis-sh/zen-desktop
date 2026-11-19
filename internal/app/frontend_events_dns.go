package app

import "github.com/irbis-sh/zen-desktop/internal/dns/resolver"

const dnsStatusChannel = "dns:status"

func (e *frontendEvents) OnDNSStatus(status resolver.Status) {
	e.emit(dnsStatusChannel, status)
}
