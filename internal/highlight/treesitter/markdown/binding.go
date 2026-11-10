// Package markdown is a cgo binding for the tree-sitter Markdown block
// grammar (tree-sitter-grammars/tree-sitter-markdown v0.5.3, MIT licensed; see
// LICENSE), vendored because the upstream module ships no Go binding.
package markdown

// #cgo CFLAGS: -std=c11 -fPIC -w
// #include "src/parser.c"
// #include "src/scanner.c"
import "C"

import "unsafe"

// Language returns the tree-sitter Language for Markdown blocks.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_markdown())
}
