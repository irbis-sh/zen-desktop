package constants

const (
	AppName          = "Zen"
	AppNameLowercase = "zen"
	OrgName          = "Irbis"

	// LocalEndpointHost is the hostname the proxy answers for itself, serving
	// injected page assets. It is a real registrable name rather than a
	// loopback address or a *.localhost name: browsers route it through the
	// proxy instead of bypassing it and connecting to loopback directly, which
	// keeps injected asset loads exempt from local-network-access permission
	// prompts (Firefox skips those checks for proxied connections). The name
	// is never resolved while the proxy is running - CONNECT delegates
	// resolution to the proxy.
	//
	// The name deliberately has no DNS record. With Zen off, a browser-cached
	// page that still references it leaks a DNS query for the name, telling
	// the resolver this machine runs Zen. This is accepted: the query
	// carries little, needs a cached rewritten page to trigger, and filter
	// list fetches already reveal an ad blocker to the network. A record
	// would not remove the leak, and the obvious targets backfire: browsers
	// classify loopback and 0.0.0.0 answers by resolved IP as local addresses,
	// reintroducing the very permission prompts this design avoids, while an
	// unrouted address turns instant NXDOMAIN failures into page loads that
	// hang on blocking asset references. If a record is ever wanted, point it
	// at owned infrastructure that refuses port 443 fast, and keep it
	// maintained.
	LocalEndpointHost = "local.irbis.sh"
)
