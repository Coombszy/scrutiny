# Scrutiny

A local tool for reviewing GitHub pull requests. See [docs/PLAN.md](docs/PLAN.md) and [GLOSSARY.md](GLOSSARY.md).

## Using it

```sh
scrutiny serve            # http://127.0.0.1:7878; paste a PR URL on the Dashboard
```

Settings live in `$XDG_CONFIG_HOME/scrutiny/config.toml` (UI changes write back to it):

```toml
port = 7878
theme = "gruvbox-dark"        # or gruvbox-light, or a file in themes/
view_mode = "split"           # or "unified"

[diff]
collapse_threshold = 1000     # changed lines before a file starts collapsed

[keys]                        # space-separated sequences; single keys only work outside inputs
symbol_search = "ctrl+k"
pr_search = "ctrl+p"
command_palette = "ctrl+j"
next_comment = "] c"
prev_comment = "[ c"
toggle_view_mode = "s"
refresh = "r"
```

Themes are TOML files in `$XDG_CONFIG_HOME/scrutiny/themes/<id>.toml` with `name`, `scheme` (`dark`/`light`), a `[ui]` table of interface colours and a `[syntax]` table of highlight capture colours; anything left out falls back to Gruvbox Dark (see `internal/theme/builtin`).

Code navigation needs a Language Server: install one from **Language Servers** in the top bar (Mason registry), or have it on `$PATH` (e.g. gopls).

## Development

Requires Go, `git`, a C compiler (tree-sitter uses cgo) and an authenticated `gh` CLI.

```sh
just build   # generate Templ and build ./bin/scrutiny
just test    # run the test suite (Language Server tests run when gopls is on $PATH or $SCRUTINY_GOPLS is set)
just run     # build and run `scrutiny serve`
just run-live  # live reload with air on :7878 (or --port N); edits rebuild, restart and reload the page
```

## Layout

- `cmd/scrutiny`: the CLI entry point.
- `internal/domain`: core types shared by every layer.
- `internal/xdg`: XDG config/state/data directories (used on Linux and macOS).
- `internal/<port>`: one package per port (`codehost`, `repostore`, `cache`, `settings`, `highlight`, `navigator` (CodeNavigator), `installer` (ServerInstaller)), holding the interface and its in-memory fake (`highlight.PlainHighlighter` doubles as the Highlighter's).
- `internal/<port>/<adapter>`: the real adapters (`codehost/github`, `repostore/gitcli`, `cache/sqlite`, `settings/tomlfile`, `highlight/treesitter`, `navigator/lsp`, `installer/mason`).
- `internal/review`: application services that compose the ports (PR pages, review activity, polling, navigation).
- `internal/live`: the SSE hub and the poller that pushes PR changes to open pages.
- `internal/servers`: which Language Server launches for a language, and background installs.
- `internal/theme`: built-in and drop-in Themes, rendered as CSS.
- `internal/metrics`: per-minute counts of GitHub and git activity (stored in the sqlite cache, kept a week), shown on the Settings page.
- `internal/web`: the driving HTTP adapter (handlers, Templ views, embedded static assets).
