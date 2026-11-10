---
# Scrutiny-ox3b
title: Refresh button on new head SHA; in-place comment updates
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T21:54:55Z
parent: Scrutiny-evgt
---

See docs/PLAN.md.


## Summary of Changes

`review.Service.Poll` diffs against the cached snapshot: a new head SHA (merge base recomputed) or new threads show a sticky Refresh banner; changed threads, Conversation, pending count and Checks are swapped in place. Unsent textarea drafts and focus survive the swaps.
