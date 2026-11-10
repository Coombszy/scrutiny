---
# Scrutiny-scbg
title: 'Review Threads and Review Comments: display, reply, resolve/unresolve'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T21:41:31Z
parent: Scrutiny-blsg
---

See docs/PLAN.md.


## Summary of Changes

CodeHost reads review state over GraphQL (threads, comments, Viewed, Pending Review, Reactions; paginated) and writes via mutations (fixture tests check operation + variables; every document also validated against the live schema with bogus IDs). Threads are placed on their diff lines when the Comparison ends at the PR head (LEFT only when it starts at the merge base); otherwise, or when outdated/off-diff, they are listed above the file. Threads on files outside the Comparison get their own section. Reply and resolve/unresolve swap the thread in place; comment bodies are GitHub's sanitised bodyHTML, with a strict CSP as a backstop.
