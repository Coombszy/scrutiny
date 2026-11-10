---
# Scrutiny-uvan
title: Live comment updates can be missed with two tabs on one PR
status: todo
type: bug
created_at: 2026-11-10T22:15:23Z
updated_at: 2026-11-10T22:15:23Z
---

Poll diffs against one shared cached snapshot per PR. Loading the PR in another tab moves that baseline, so an already-open tab never sees comments that arrived in between (new heads are caught up on connect). Fix: version the snapshot and let each stream ask for changes since the version its page rendered.
