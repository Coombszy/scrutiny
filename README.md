# Scrutiny

A local tool for reviewing GitHub pull requests. See [docs/PLAN.md](docs/PLAN.md) and [GLOSSARY.md](GLOSSARY.md).

## Development

Requires Go, `git`, a C compiler (tree-sitter uses cgo) and an authenticated `gh` CLI.

```sh
just build   # generate Templ and build ./bin/scrutiny
just test    # run the test suite (Language Server tests run when gopls is on $PATH or $SCRUTINY_GOPLS is set)
just run     # build and run `scrutiny serve`
```

## Layout

- `cmd/scrutiny`: the CLI entry point.
- `internal/domain`: core types shared by every layer.
- `internal/xdg`: XDG config/state/data directories (used on Linux and macOS).
- `internal/<port>`: one package per port (`codehost`, `repostore`, `cache`, `settings`, `highlight`), holding the interface and its in-memory fake (`highlight.PlainHighlighter` doubles as the Highlighter's).
- `internal/<port>/<adapter>`: the real adapters (`codehost/github`, `repostore/gitcli`, `cache/sqlite`, `settings/tomlfile`, `highlight/treesitter`).
- `internal/review`: application services that compose the ports.
- `internal/web`: the driving HTTP adapter (handlers, Templ views, embedded static assets).
