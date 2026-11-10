---
# Scrutiny-esiu
title: PR page fails when the base branch no longer exists
status: todo
type: bug
created_at: 2026-11-10T18:18:15Z
updated_at: 2026-11-10T18:18:15Z
---

FetchPR always fetches refs/heads/<base>. For merged stacked PRs whose base branch was deleted this fails with a 500. Fall back to the PR's base SHA from GitHub (or fetch refs/pull/N/head only) when the branch is gone.
