---
# Scrutiny-lpzd
title: Preserve comments and unset keys when the UI writes config.toml
status: todo
type: bug
created_at: 2026-11-10T18:18:15Z
updated_at: 2026-11-10T18:18:15Z
---

tomlfile.Save re-marshals the whole Settings struct, so toggling the View Mode drops comments/unknown keys and writes every default into the file. Update only the changed key (e.g. edit the TOML document), and serialise load-modify-save.
