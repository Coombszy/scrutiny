---
# Scrutiny-oar7
title: 'PR page: split/unified diff, Comparison picker (merge-base..head and commit range)'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T18:15:02Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

`review.Service.PRPage` resolves the Comparison: merge base to head by default (merge base cached per head SHA; fetching only when the head is not local), or a contiguous run of PR commits compared from the parent of the first (the merge base for the first commit). Invalid ranges return 400. The PR page has a from/to commit picker, a Split/Unified toggle that writes `view_mode` back to config.toml, and file fragments rendered split (deletions paired with additions) or unified.
