---
# Scrutiny-uvan
title: Live comment updates can be missed with two tabs on one PR
status: todo
type: bug
created_at: 2026-11-10T22:15:23Z
updated_at: 2026-11-10T22:15:23Z
---

Poll diffs against one shared cached snapshot per PR. Loading the PR in another tab moves that baseline, so an already-open tab never sees comments that arrived in between (new heads are caught up on connect). Fix: version the snapshot and let each stream ask for changes since the version its page rendered.

The same baseline also loses updates for a hidden page: the stream closes while hidden, another tab of the PR receives the update and moves the baseline, and on reconnect only head moves are caught up (`?head=` in `web/live.go`). Versioned snapshots fix both.

It also stops Viewed changes made in one Scrutiny tab reaching another open on the same PR (Scrutiny-dnh7 pushes only those made on github.com): the ticking tab's write re-reads and saves the snapshot, so the next poll sees no change.
