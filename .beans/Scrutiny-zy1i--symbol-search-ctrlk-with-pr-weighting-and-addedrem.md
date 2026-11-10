---
# Scrutiny-zy1i
title: Symbol Search (ctrl+k) with PR Weighting and added/removed markers
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T22:04:37Z
parent: Scrutiny-6gm4
---

See docs/PLAN.md.


## Summary of Changes

ctrl+k palette: files from `git ls-tree` at head + LSP workspace symbols for the languages the PR touches, fuzzy-scored (word starts, runs), PR files/symbols first (PR Weighting). Green marker: added files and symbols declared on added lines; red: files the PR deletes. Deviation: removed *symbols* are not detected (would need a server on the base too); removed files are.
