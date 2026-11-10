---
# Scrutiny-zfdt
title: Large/generated/binary/rename handling with configurable collapse threshold; lazy per-file loading
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T18:15:02Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

Files over `diff.collapse_threshold` (default 1000) and generated files (`linguist-generated` read with `git check-attr --source=<head>`, or known lockfiles unless `linguist-generated=false`) start collapsed with a "Load diff" button. Binary files show "Binary file changed"; renames show `old → new` (and "renamed without changes" when there is nothing to show). Every other file loads its diff lazily with htmx `intersect once`.
