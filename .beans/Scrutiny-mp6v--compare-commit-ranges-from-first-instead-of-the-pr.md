---
# Scrutiny-mp6v
title: Compare commit ranges from first^ instead of the previous PR commit
status: todo
type: bug
created_at: 2026-11-10T18:18:15Z
updated_at: 2026-11-10T18:18:15Z
---

review.resolve uses commits[first-1] from GitHub's PR commit list as the left side. With merge commits in the PR that is not the parent of the first selected commit. Resolve first^ via RepoStore (and refuse/flag non-linear history), per PLAN.md 'first^..last'.
