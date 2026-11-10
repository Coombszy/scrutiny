---
# Scrutiny-hm9w
title: CodeNavigator port + LSP client adapter; lazy start, 10-minute idle shutdown
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T22:04:37Z
parent: Scrutiny-6gm4
---

See docs/PLAN.md.


## Summary of Changes

`internal/navigator` port (Definition, References, Symbols, Indexing, Reset; UTF-16 columns, 1-based lines) + fake. `navigator/lsp`: minimal JSON-RPC/LSP client; one server per (worktree, language), started on first query, stopped after 10 minutes idle (reaper; restarts transparently); answers server requests (progress create, configuration, folders); Indexing from `$/progress` begin/end until the first answer. Integration tests run against real gopls when on PATH or `$SCRUTINY_GOPLS` (skip otherwise). `serve` uses an installed gopls or one on PATH (M5 installs more).
