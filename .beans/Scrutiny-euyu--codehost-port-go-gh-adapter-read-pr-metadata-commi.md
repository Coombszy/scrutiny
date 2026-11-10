---
# Scrutiny-euyu
title: 'CodeHost port + go-gh adapter (read): PR metadata, commits, Dashboard queries, ETag caching'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:56Z
updated_at: 2026-11-10T18:03:30Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

`domain` PR types (PullRequest, Commit, PRSummary). `internal/codehost` port + in-memory fake. `internal/codehost/github` adapter over REST with `gh auth` (go-gh `DefaultHTTPClient`): PR metadata, paginated commits (Link header), Dashboard via search (review-requested / author `@me`). Every GET revalidates the cached copy with `If-None-Match`; 304s are served from the Cache port (Link header kept for pagination). Tests use fixtures recorded from cli/go-gh#325. GraphQL is left for M2 review state.
