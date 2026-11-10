---
# Scrutiny-e7o5
title: 'Checks panel: required Checks, merge state, log links'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T21:54:55Z
parent: Scrutiny-evgt
---

See docs/PLAN.md.


## Summary of Changes

One GraphQL query (statusCheckRollup with isRequired, mergeStateStatus) feeds a header pill (merge state + failure/pending/success counts) that expands to the list: failures first, Required marked, links to logs. Updated live.
