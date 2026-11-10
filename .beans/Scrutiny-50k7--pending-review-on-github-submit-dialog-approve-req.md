---
# Scrutiny-50k7
title: Pending Review on GitHub + submit dialog (Approve / Request changes / Comment); anchor to right-hand commit of Comparison
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T21:41:31Z
parent: Scrutiny-blsg
---

See docs/PLAN.md.


## Summary of Changes

New threads and replies go into the GitHub Pending Review, created on demand at the Comparison's right-hand commit (replies: the PR head). Because GitHub anchors a thread to its review's commit, adding a thread is refused (409, with an explanation; buttons hidden) when the Pending Review is on another commit, or on the LEFT side when the Comparison does not start at the merge base. Review button shows the pending count (updated out-of-band); the submit dialog offers Comment/Approve/Request changes and creates+submits in one step when nothing is pending.
