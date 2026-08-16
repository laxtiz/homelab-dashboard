package webassets

import "embed"

// FS embeds the built Vue frontend (web/dist) into the binary.
//
//go:embed dist
var FS embed.FS