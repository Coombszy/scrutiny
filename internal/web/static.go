package web

import "embed"

// staticFS holds the browser assets served under /static/.
//
//go:embed static
var staticFS embed.FS
