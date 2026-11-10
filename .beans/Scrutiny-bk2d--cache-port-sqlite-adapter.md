---
# Scrutiny-bk2d
title: Cache port + sqlite adapter
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:56Z
updated_at: 2026-11-10T18:01:06Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

`internal/cache` port (keyed entries with ETag + fetched-at; PR visits for the Dashboard) and in-memory fake. `internal/cache/sqlite` adapter on modernc.org/sqlite (pure Go, WAL), schema created on open.
