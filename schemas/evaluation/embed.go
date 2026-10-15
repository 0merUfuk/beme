package evaluationschema

import "embed"

// Files embeds the same versioned contracts used by public validation.
//
//go:embed *.schema.json
var Files embed.FS
