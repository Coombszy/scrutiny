---
# Scrutiny-jbll
title: SSE hub with connection tracking
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T21:54:55Z
parent: Scrutiny-evgt
---

See docs/PLAN.md.


## Summary of Changes

`internal/live.Hub` tracks event-stream connections per PR and fans Updates out (buffered, dropping for a stalled client). `GET …/pull/N/events` streams them as `update` events of out-of-band HTML (threads, Conversation, pending count, Checks, banners) with a heartbeat; request contexts derive from the server context so streams end on shutdown.
