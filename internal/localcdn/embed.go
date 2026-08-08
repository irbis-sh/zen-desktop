package localcdn

import "embed"

//go:embed resources
var embeddedResources embed.FS

//go:embed resources.json
var resourcesManifest []byte
