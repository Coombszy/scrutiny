---
# Scrutiny-6ekd
title: Stacks via GitHub native stack fields; Parent/Child buttons
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:58Z
updated_at: 2026-11-10T22:22:02Z
parent: Scrutiny-10k3
---

See docs/PLAN.md.


## Summary of Changes

ReviewState reads GitHub's native `stackEntry` (position, size, entries). The PR header shows "Stack n/m" with ← Parent / Child → links (also in the Command Palette). Fixture re-recorded from cli/cli#14956, a real 8-PR Stack.
