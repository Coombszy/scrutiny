---
# Scrutiny-0eol
title: Since Last Review shortcut
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:58Z
updated_at: 2026-11-10T22:22:02Z
parent: Scrutiny-10k3
---

See docs/PLAN.md.


## Summary of Changes

ReviewState records the head commit of the viewer's last submitted review; when it is still in the PR's commits and not the head, the Comparison picker offers "Since your last review" (the commits after it, to head; also in the Command Palette).
