---
# Scrutiny-x1tq
title: 'Highlighter port + tree-sitter adapter: Go, YAML, JSON, Markdown, shell; Gruvbox Dark theme'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T18:15:02Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

`internal/highlight` port (lines of `Token{Text, Class}`), plain fallback. `internal/highlight/treesitter` adapter: Go, JSON, YAML, bash, Markdown (block grammar vendored with its MIT licence, since the upstream module has no Go binding; inline Markdown is not highlighted). Inner captures override outer ones and earlier patterns win on ties. Highlights whole blobs so multi-line constructs colour correctly. Gruvbox Dark capture colours live in `static/scrutiny.css` (the drop-in Theme format is M6).
