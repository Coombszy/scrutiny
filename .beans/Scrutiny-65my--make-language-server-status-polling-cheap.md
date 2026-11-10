---
# Scrutiny-65my
title: Make Language Server status polling cheap
status: todo
type: task
created_at: 2026-11-10T22:29:21Z
updated_at: 2026-11-10T22:29:21Z
---

Each installing row polls every second, and every poll re-parses the whole Mason registry and runs LookPath for every package. Cache the parsed registry and runtime lookups in the servers service.
