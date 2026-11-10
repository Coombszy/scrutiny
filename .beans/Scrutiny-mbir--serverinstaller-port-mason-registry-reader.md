---
# Scrutiny-mbir
title: ServerInstaller port + Mason registry reader
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:57Z
updated_at: 2026-11-10T22:11:17Z
parent: Scrutiny-349n
---

See docs/PLAN.md.


## Summary of Changes

`internal/installer` port (Packages, Installed, Install, Remove, Missing) + fake. `installer/mason` downloads the latest mason-registry release's `registry.json.zip`, verifies it against `checksums.txt` (sha256), caches it in `$XDG_DATA_HOME/scrutiny/servers/registry.json` (kept if a refresh fails) and lists LSP packages with their source, version, required runtimes and, when not installable, why (unsupported source, no asset for this platform, bin needs dotnet/python/java…). Parses purls and registry templates (`{{version}}`, `strip_prefix`, `{{source.asset.*}}`).
