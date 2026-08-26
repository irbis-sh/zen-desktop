package app

import "fmt"

type proxyEventKind string

// Only these states are handled via frontendEvents because the proxy can only start without any input from the user.
const (
	proxyChannel                            = "proxy:action"
	proxyStarting            proxyEventKind = "starting"
	proxyStarted             proxyEventKind = "started"
	proxyStartError          proxyEventKind = "startError"
	proxyStopping            proxyEventKind = "stopping"
	proxyStopped             proxyEventKind = "stopped"
	proxyStopError           proxyEventKind = "stopError"
	unsupportedDE            proxyEventKind = "unsupportedDE"
	caSystemTrustUnavailable proxyEventKind = "caSystemTrustUnavailable"
)

type proxyEvent struct {
	Kind  proxyEventKind `json:"kind"`
	Error string         `json:"error"`
}

func (e *frontendEvents) OnProxyStarting() {
	e.emit(proxyChannel, proxyEvent{Kind: proxyStarting})
}

func (e *frontendEvents) OnProxyStarted() {
	e.emit(proxyChannel, proxyEvent{Kind: proxyStarted})
}

func (e *frontendEvents) OnProxyStartError(err error) {
	e.emit(proxyChannel, proxyEvent{Kind: proxyStartError, Error: fmt.Sprint(err)})
}

func (e *frontendEvents) OnProxyStopping() {
	e.emit(proxyChannel, proxyEvent{Kind: proxyStopping})
}

func (e *frontendEvents) OnProxyStopped() {
	e.emit(proxyChannel, proxyEvent{Kind: proxyStopped})
}

func (e *frontendEvents) OnProxyStopError(err error) {
	e.emit(proxyChannel, proxyEvent{Kind: proxyStopError, Error: fmt.Sprint(err)})
}

func (e *frontendEvents) OnUnsupportedDE(err error) {
	e.emit(proxyChannel, proxyEvent{Kind: unsupportedDE, Error: fmt.Sprint(err)})
}

func (e *frontendEvents) OnCASystemTrustUnavailable(err error) {
	e.emit(proxyChannel, proxyEvent{Kind: caSystemTrustUnavailable, Error: fmt.Sprint(err)})
}
