# Scrutiny

A local tool for reviewing GitHub pull requests: split and unified diffs, tree-sitter highlighting, go-to-definition and find usages through Language Servers, and the full GitHub review workflow, served as a webpage from your own machine. Me and claude threw this together as fast as possible as I was sick of fighting with GitHub.

## Install

Requires Go, `git`, a C compiler (tree-sitter uses cgo) and an authenticated [`gh`](https://cli.github.com) CLI.

```sh
go install github.com/Coombszy/scrutiny/cmd/scrutiny@latest
```

This puts `scrutiny` in `$(go env GOPATH)/bin` (or `$GOBIN`), which needs to be on your `$PATH`. Use `@v0.1.0` in place of `@latest` to install a specific release.

To update to a new release, run the same command again: `@latest` resolves to the newest tagged release and replaces the installed binary. The version you're running is shown faintly in the top bar.

To build from a checkout instead (needs [`just`](https://github.com/casey/just)):

```sh
just build   # builds ./bin/scrutiny
```

## Use

```sh
just run         # build and serve in one step
scrutiny serve   # or run an installed binary (-port N to change the port)
```

Then open [http://localhost:7878](http://localhost:7878) and paste a PR URL on the Dashboard.

To jump from GitHub to Scrutiny, install [`userscript/scrutiny.user.js`](userscript/scrutiny.user.js) in [Tampermonkey](https://www.tampermonkey.net) (or Violentmonkey). While Scrutiny is running, every pull request page on github.com gets a **Goto Scrutiny** button that opens the PR in Scrutiny in a new tab. If you serve on a port other than 7878, set it from the script's **Set Scrutiny port** menu command.

Code navigation needs a Language Server: install one from **Language Servers** on the Settings page (the cog in the top bar), or have it on `$PATH` (e.g. `gopls`).

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
