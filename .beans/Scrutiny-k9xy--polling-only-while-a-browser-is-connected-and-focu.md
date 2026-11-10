---
# Scrutiny-k9xy
title: Polling only while a browser is connected and focused; ETags; rate-limit backoff
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T21:54:55Z
parent: Scrutiny-evgt
---

See docs/PLAN.md.


## Summary of Changes

The page opens its EventSource only while visible (closes on `visibilitychange`/`pagehide`), so a connection means connected and focused. `live.Poller` polls only watched PRs, every 30s (immediately on first connect, not again on a quick reconnect), and stretches the interval ×4 while any GitHub rate limit (REST or GraphQL, from X-RateLimit-* headers) is under 10%. REST reads keep using ETags; GraphQL has none.
