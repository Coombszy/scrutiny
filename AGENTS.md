# Scrutiny

Go, Templ and htmx, in a hexagonal architecture: services depend only on ports, every external system sits behind an adapter.

- **Vocabulary**: use the terms in [GLOSSARY.md](GLOSSARY.md) in code, comments and UI text.
- **Work tracking**: issues are `beans` files in `.beans/` (`beans list`, `beans roadmap`).
- **Commands**: the `justfile`. Run `just generate` after editing `.templ` files.

## Principles

- **GitHub is the source of truth** for review state. The sqlite cache is disposable: deleting it never loses data.
- **The config file is the source of truth** for settings. UI changes write back to it.
- **Cache-first**: minimise upstream GitHub calls (ETag conditional requests, polling only while a page is connected and focused).
- **Loopback only**: `serve` binds `127.0.0.1`, rejects non-localhost `Host`/`Origin` and cross-site requests (`Sec-Fetch-Site`) other than to the Dashboard, `/health`, `/ticket` and navigations carrying a Goto Ticket, and requires a CSRF token on mutating requests.
- **Linux and macOS**, using XDG directories on both.

## Testing

- Service tests use each port's in-memory fake.
- `codehost/github` tests run against recorded GraphQL and REST fixtures; tests never call live GitHub.
- `repostore/gitcli` tests use throwaway local `git init` repos.
- `navigator/lsp` tests run against a real gopls.

## Files on disk

- `$XDG_CONFIG_HOME/scrutiny/config.toml` and `themes/*.toml`: settings and extra Themes.
- `$XDG_STATE_HOME/scrutiny/repos/<host>/<owner>/<repo>.git`: bare clones fetching `refs/pull/*/head`.
- `$XDG_STATE_HOME/scrutiny/worktrees/<host>/<owner>/<repo>/<pr>`: one worktree per PR at head, for Language Servers.
- `$XDG_STATE_HOME/scrutiny/cache.db`: the sqlite cache.
- `$XDG_DATA_HOME/scrutiny/servers/`: installed Language Servers.

## Layout

- `cmd/scrutiny`: the CLI entry point. `cmd/fakeserve`: the UI over in-memory fakes (`just run-fake`).
- `internal/domain`: core types shared by every layer.
- `internal/xdg`: XDG config/state/data directories (used on Linux and macOS).
- `internal/<port>`: one package per port (`codehost`, `repostore`, `cache`, `settings`, `highlight`, `release`, `navigator` (CodeNavigator), `installer` (ServerInstaller)), holding the interface and its in-memory fake (`highlight.PlainHighlighter` doubles as the Highlighter's).
- `internal/<port>/<adapter>`: the real adapters (`codehost/github`, `repostore/gitcli`, `cache/sqlite`, `settings/tomlfile`, `highlight/treesitter`, `navigator/lsp`, `installer/mason`, `release/goproxy`).
- `internal/review`: application services that compose the ports (PR pages, review activity, polling, navigation).
- `internal/live`: the SSE hub and the poller that pushes PR changes to open pages.
- `internal/servers`: which Language Server launches for a language, and background installs.
- `internal/theme`: built-in and drop-in Themes, rendered as CSS. A theme file is TOML with `name`, `scheme`, `[ui]` and `[syntax]` tables; missing keys fall back to Gruvbox Dark (`internal/theme/builtin`).
- `internal/release`: the port for finding Scrutiny's latest release, and the Checker that looks for a New Version on startup and every 10 minutes (release builds only); `internal/release/goproxy` asks the Go module proxy (`$GOPROXY`, default proxy.golang.org) for `@latest`.
- `internal/metrics`: per-minute counts of GitHub and git activity (stored in the sqlite cache, kept a week), shown on the Settings page.
- `internal/web`: the driving HTTP adapter (handlers, Templ views, embedded static assets).
- `userscript`: the Userscript adding a Goto Scrutiny button to GitHub PR pages; it checks `GET /health` and fetches Goto Tickets from `GET /ticket`, the routes any origin may call.
