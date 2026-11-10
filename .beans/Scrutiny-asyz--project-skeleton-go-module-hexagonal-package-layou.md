---
# Scrutiny-asyz
title: 'Project skeleton: Go module, hexagonal package layout, justfile (build/test/generate/run), Templ + htmx embedded assets'
status: completed
type: task
priority: normal
created_at: 2026-11-10T17:46:56Z
updated_at: 2026-11-10T17:53:35Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

Go module `github.com/Coombszy/scrutiny`, `cmd/scrutiny serve`, `internal/web` with embedded htmx + SSE extension and a Templ layout, justfile (generate/build/test/vet/run), templ pinned as a Go tool. Generated `_templ.go` files are committed so `go build` works without generating first.
