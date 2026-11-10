---
# Scrutiny-s5ph
title: Installers for npm, go and github release sources into $XDG_DATA_HOME/scrutiny/servers
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:58Z
updated_at: 2026-11-10T22:11:17Z
parent: Scrutiny-349n
---

See docs/PLAN.md.


## Summary of Changes

Installs into `servers/packages/<name>/` with a receipt, linking executables from `servers/bin/`: npm (`npm install --prefix … pkg@ver extra_packages…`), go (`GOBIN=… go install module@ver`), GitHub releases (best asset for OS/arch preferring gnu, unpacking .gz/.tar.gz/.zip in Go and .tar.xz/.bz2 via tar, `file:dest/` subdirectories, path-traversal checks). Verified for real: zls (GitHub tarball), bash-language-server (npm), gopls (go install).
