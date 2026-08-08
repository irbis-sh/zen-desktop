// Package pagestyle holds the stylesheet shared by the HTML pages Zen serves in
// place of content (the block page and the proxy error page), so their look
// cannot drift apart.
package pagestyle

import (
	_ "embed"
	"html/template"
)

//go:embed style.css
var css string

// Shared is the stylesheet shared by Zen-served pages, typed for direct
// interpolation into a <style> element.
var Shared = template.CSS(css) // #nosec G203 -- css is a compile-time embedded asset, not user input.
