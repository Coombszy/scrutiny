# Scrutiny

A local tool for reviewing GitHub pull requests: split and unified diffs, tree-sitter highlighting, go-to-definition and find usages through Language Servers, and the full GitHub review workflow, served as a webpage from your own machine. Me and claude threw this together as fast as possible as I was sick of fighting with GitHub.

## Install

Requires Go, `git`, a C compiler (tree-sitter uses cgo), [`just`](https://github.com/casey/just) and an authenticated [`gh`](https://cli.github.com) CLI.

```sh
just build   # builds ./bin/scrutiny
```

## Use

```sh
just run         # build and serve in one step
scrutiny serve   # or run an installed binary (-port N to change the port)
```

Then open [http://localhost:7878](http://localhost:7878) and paste a PR URL on the Dashboard.

Code navigation needs a Language Server: install one from **Language Servers** in the top bar, or have it on `$PATH` (e.g. `gopls`).

Settings live in `$XDG_CONFIG_HOME/scrutiny/config.toml` and changes made in the UI are written back to it. Themes, hotkeys and the diff collapse threshold can all be changed from **Settings**.

## Develop

```sh
just test      # run the tests (Language Server tests need gopls on $PATH or $SCRUTINY_GOPLS)
just run-live  # live reload on :7878
just run-fake  # the UI over in-memory fakes with a demo PR, no GitHub needed
```

Any `just` command enables the pre-commit hook in `.githooks`, which blocks a commit if [TruffleHog](https://github.com/trufflesecurity/trufflehog) finds a secret in the staged changes or the git history (so it needs `trufflehog` installed).

[AGENTS.md](AGENTS.md) covers the architecture and [GLOSSARY.md](GLOSSARY.md) the vocabulary.

## Licence

[MIT](LICENSE)
