---
# Scrutiny-iaru
title: 'scrutiny serve: 127.0.0.1 bind, Host/Origin check, CSRF middleware'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:56Z
updated_at: 2026-11-10T17:57:12Z
parent: Scrutiny-aylo
---

See docs/PLAN.md.


## Summary of Changes

`web.Secure` middleware: loopback-only Host and Origin (DNS-rebinding defence), constant-time CSRF check (header `X-CSRF-Token` or form `csrf_token`) on non-GET/HEAD/OPTIONS. Per-process token is injected via `hx-headers` on `<body>` and a `csrf-token` meta tag. `serve` binds 127.0.0.1 on the configured port (`-port` overrides).
