---
# Scrutiny-6r0c
title: Settings panel UI for installing/removing Language Servers
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:58Z
updated_at: 2026-11-10T22:11:17Z
parent: Scrutiny-349n
---

See docs/PLAN.md.


## Summary of Changes

`/settings/servers` (linked from the top bar): filterable list, installed first, with Install / Update / Remove, "Requires X", or why it cannot be installed; installs run in the background (`internal/servers`) and the row polls every second until done, showing errors inline; "Update registry" refreshes. `internal/servers` also resolves the launch command per language (installed package + known args like `--stdio`, else a known server on $PATH), looked up by the LSP manager each time a server starts.
