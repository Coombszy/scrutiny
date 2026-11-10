---
# Scrutiny-jzjf
title: Rebindable hotkeys + single-key bindings
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:58Z
updated_at: 2026-11-10T22:22:02Z
parent: Scrutiny-10k3
---

See docs/PLAN.md.


## Summary of Changes

`[keys]` in config.toml (validated, merged over defaults): symbol_search ctrl+k, command_palette ctrl+j, next/prev_comment "] c"/"[ c", toggle_view_mode s, refresh r. One client dispatcher handles modifiers and multi-key sequences (1s timeout); unmodified bindings are ignored while an input has focus. Rebinding from the Command Palette records the keys and writes them to config.toml.
