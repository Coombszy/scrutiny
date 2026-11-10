---
# Scrutiny-esiu
title: PR page fails when the base branch no longer exists
status: completed
type: bug
priority: normal
created_at: 2026-11-10T18:18:15Z
updated_at: 2026-11-10T21:41:31Z
---

FetchPR always fetches refs/heads/<base>. For merged stacked PRs whose base branch was deleted this fails with a 500. Fall back to the PR's base SHA from GitHub (or fetch refs/pull/N/head only) when the branch is gone.


## Summary of Changes

CodeHost now returns the PR's base SHA; FetchPR falls back to fetching that SHA when `refs/heads/<base>` no longer exists (merged Stacks). Found on cli/cli#14956.
