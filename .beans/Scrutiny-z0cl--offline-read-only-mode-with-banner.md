---
# Scrutiny-z0cl
title: Offline read-only mode with banner
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T21:54:55Z
parent: Scrutiny-evgt
---

See docs/PLAN.md.


## Summary of Changes

On transport errors the GitHub adapter serves REST reads from the ETag cache and reports offline; GraphQL reads fail with ErrOffline and the service falls back to the cached review snapshot. Pages show an offline banner and CSS hides every review action; writes return 503. Polling flips the banner on and off.
