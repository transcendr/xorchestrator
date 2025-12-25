package workflow

import "embed"

// builtinTemplates embeds all built-in workflow templates from the templates directory.
//
//go:embed templates/*.md
var builtinTemplates embed.FS
