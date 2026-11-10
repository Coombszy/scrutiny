---
# Scrutiny-krgv
title: Go to definition / find usages (ctrl-click + right-click menu), file view for unchanged files
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T22:04:37Z
parent: Scrutiny-6gm4
---

See docs/PLAN.md.


## Summary of Changes

Ctrl/cmd-click on head-side code (split right side; unified add/context lines; file view) goes to the definition; right-click opens a menu with Go to definition / Find usages. The column is computed from the caret position (UTF-16). Targets in PR files scroll the diff (loading/expanding the file) and flash the line; otherwise the file view (`…/blob?path=…#L<n>`, whole file at head, highlighted, navigable) opens; stdlib/module-cache targets show a toast. Find usages opens a side panel grouped by file with line text. Left side is a no-op.
