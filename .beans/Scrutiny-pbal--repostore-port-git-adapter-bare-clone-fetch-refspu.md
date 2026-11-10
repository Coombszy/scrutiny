---
# Scrutiny-pbal
title: 'RepoStore port + git adapter: bare clone, fetch refs/pull/*/head, blob at SHA, ls-tree, diff two commits'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:56Z
updated_at: 2026-11-10T17:59:51Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

`internal/domain` core types (Repo, PRRef, SHA, FileChange, FileDiff, Hunk, Line). `internal/repostore` port. `internal/repostore/gitcli` adapter: bare clone per repo, `FetchPR` fetches the base branch and `refs/pull/N/head` (targeted rather than `refs/pull/*/head`, to keep big repos cheap), merge-base, `Changes` (raw+numstat: status, renames, binary, line counts), `FileDiff` hunk parsing with line numbers, blob at SHA, ls-tree. Auth via `gh auth git-credential`. Tests use throwaway local repos as the remote; fetching from GitHub is checked manually.
