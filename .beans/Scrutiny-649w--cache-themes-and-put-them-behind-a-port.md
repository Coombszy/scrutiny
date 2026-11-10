---
# Scrutiny-649w
title: Cache themes and put them behind a port
status: todo
type: task
created_at: 2026-11-10T22:29:21Z
updated_at: 2026-11-10T22:29:21Z
---

web calls theme.Load (reading the themes directory) on every page render and /theme.css. Load once and reload on change (or on an mtime check), and give Themes a port + fake like the other adapters.
