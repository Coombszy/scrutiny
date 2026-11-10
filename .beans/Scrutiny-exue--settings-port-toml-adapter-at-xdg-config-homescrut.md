---
# Scrutiny-exue
title: Settings port + TOML adapter at $XDG_CONFIG_HOME/scrutiny/config.toml (XDG on Linux and macOS)
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:56Z
updated_at: 2026-11-10T17:56:24Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

`internal/xdg` resolves config/state/data dirs from `XDG_*` (falling back to spec defaults on Linux and macOS). `internal/settings` defines the `Settings` struct, defaults, validation, the `Store` port and an in-memory fake. `internal/settings/tomlfile` loads over defaults (missing file = defaults) and saves atomically.
