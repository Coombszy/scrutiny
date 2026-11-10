---
# Scrutiny-flt6
title: Runtime detection (node, go) with 'requires X' messages
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:58Z
updated_at: 2026-11-10T22:11:17Z
parent: Scrutiny-349n
---

See docs/PLAN.md.


## Summary of Changes

npm packages require node and npm, go packages require go; missing ones are listed per package ("Requires node") and Install refuses with ErrMissingRuntime without running anything.
