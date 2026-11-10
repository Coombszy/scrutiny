---
# Scrutiny-aylo
title: 'M1: Read-only diff viewer'
status: completed
type: milestone
priority: normal
created_at: 2026-11-10T17:46:56Z
updated_at: 2026-11-10T18:18:15Z
---

See docs/PLAN.md.


## Summary of Changes

All M1 features done (see child beans). Code review fixes: owner/repo names validated against GitHub's charset (blocked a path traversal into the clone directory); merge-base cache key now includes the base branch. Follow-up bugs/tasks were filed for the remaining review findings.
