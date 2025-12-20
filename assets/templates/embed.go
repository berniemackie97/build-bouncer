package templates

import "embed"

//go:embed *.yaml *.json
var FS embed.FS
