---
# Scrutiny-rqwk
title: 'PR worktrees at head: create, update on head move, remove on close/merge'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T22:04:37Z
parent: Scrutiny-6gm4
---

See docs/PLAN.md.


## Summary of Changes

`RepoStore.Worktree` checks the PR head out into `$XDG_STATE_HOME/scrutiny/worktrees/<host>/<owner>/<repo>/<pr>` (git worktree from the bare clone), moving it when the head moves (Language Servers for it are reset); `RemoveWorktree` runs when a PR page shows it closed or merged. Tested against local repos.
