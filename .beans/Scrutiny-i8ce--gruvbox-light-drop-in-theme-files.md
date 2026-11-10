---
# Scrutiny-i8ce
title: Gruvbox Light + drop-in theme files
status: completed
type: feature
priority: normal
created_at: 2026-11-10T17:46:58Z
updated_at: 2026-11-10T22:22:02Z
parent: Scrutiny-10k3
---

See docs/PLAN.md.


## Summary of Changes

`internal/theme`: Gruvbox Dark and Light built in (TOML), drop-in themes from `$XDG_CONFIG_HOME/scrutiny/themes/*.toml` override by id and fall back to Gruvbox Dark for unset values; keys and colours are validated so files cannot inject CSS. `/theme.css` serves the configured Theme (UI custom properties + `.hl-*` capture colours); the static stylesheet keeps only layout and font styles.
