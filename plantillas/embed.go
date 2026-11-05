// Package plantillas embeds the templates basekit copies into projects.
// Everything stack-specific lives here as data, never in the CLI code.
package plantillas

import "embed"

// FS holds core/, skills/ and stacks/. The all: prefix keeps dotfiles such as
// .make/, .env.example, .dockerignore and .gitkeep.
//
//go:embed all:core all:skills all:stacks
var FS embed.FS
