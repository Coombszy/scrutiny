# Scrutiny — v1 Plan

A local Go daemon (`scrutiny serve`) that serves a better GitHub PR review UI on localhost. Vocabulary is defined in [GLOSSARY.md](../GLOSSARY.md); progress is tracked in `beans` (`beans list`, `beans roadmap`).

## Principles

- **Hexagonal architecture.** Domain and application services depend only on ports. Every external system (GitHub, git, language servers, tree-sitter, sqlite, config) sits behind an adapter, and HTTP/Templ is just the driving adapter.
- **GitHub is the source of truth** for review state. sqlite is a disposable cache; deleting it never loses data.
- **Config file is the source of truth** for settings. UI changes write back to it.
- **Cache-first.** Minimise upstream GitHub calls.
- **Linux and macOS**, built locally. No cross-compiled releases for now.

## Stack

- Go, Templ, htmx (with the SSE extension), sqlite.
- `github.com/cli/go-gh` for GitHub auth (reuses `gh auth`) plus GraphQL and REST.
- `git` binary (shelled out) for all repo operations.
- `github.com/tree-sitter/go-tree-sitter` (cgo) for highlighting.
- LSP for code navigation, with servers installed from the Mason registry.
- `justfile` for build, test, generate and run commands.

## Ports and adapters

| Port | Responsibility | v1 adapter |
|---|---|---|
| `CodeHost` | PRs, Stacks, Review Threads, Pending Reviews, Reactions, Checks, Viewed, Dashboard queries | go-gh GraphQL and REST |
| `RepoStore` | fetch, blob at SHA, list files at SHA, diff two commits, PR worktrees | `git` binary |
| `CodeNavigator` | definition, usages, workspace symbols, Indexing state | LSP client |
| `ServerInstaller` | list, install and remove Language Servers; runtime detection | Mason registry |
| `Highlighter` | source to highlight spans | tree-sitter |
| `Cache` | cached GitHub responses, ETags, derived data | sqlite |
| `Settings` | user settings, keybinds, theme, thresholds | TOML file |

`CodeNavigator` is deliberately swappable: a tree-sitter tags-based adapter could replace LSP without touching the domain.

## Filesystem layout

XDG on both Linux and macOS, respecting the `XDG_*` env vars:

- `$XDG_CONFIG_HOME/scrutiny/config.toml`: settings (source of truth).
- `$XDG_CONFIG_HOME/scrutiny/themes/*.toml`: extra Themes.
- `$XDG_STATE_HOME/scrutiny/repos/<host>/<owner>/<repo>.git`: bare clones, fetching `refs/pull/*/head`.
- `$XDG_STATE_HOME/scrutiny/worktrees/<host>/<owner>/<repo>/<pr>`: one worktree per PR at head, for Language Servers.
- `$XDG_STATE_HOME/scrutiny/cache.db`: sqlite cache.
- `$XDG_DATA_HOME/scrutiny/servers/`: installed Language Servers.

## Key behaviours

### Server and security
- `scrutiny serve` binds to `127.0.0.1` on a configurable port.
- Rejects requests whose `Host` or `Origin` header isn't localhost (DNS-rebinding defence).
- CSRF token on every mutating request.
- github.com only. URLs mirror GitHub with the host included, e.g. `/github.com/owner/repo/pull/123`.

### Diffs
- Split and unified View Modes.
- **Comparison**: merge-base to head by default, or a contiguous commit range from the branch's current linear history (`first^..last`). A **Since Last Review** shortcut appears when that commit is still on the branch.
- Files over a configurable changed-line threshold (default 1000) and generated files (`linguist-generated`, known lockfiles) start collapsed with a "Load diff" button.
- Binary files show "binary changed". Renames show `old → new`.
- Each file's diff is loaded lazily via htmx as it scrolls into view.
- Highlighting via tree-sitter for Go, YAML, JSON, Markdown and shell. Other files render as plain text.

### Review
- GitHub's model 1:1: Review Threads (resolve/unresolve), Review Comments, Conversation Comments, and the 8 GitHub Reactions.
- Draft comments go straight into the GitHub **Pending Review**. The submit dialog offers Approve, Request changes and Comment.
- While viewing a commit range, new comments attach to the range's right-hand commit. Commenting is disabled on files outside the diff.
- **Viewed** checkbox per file, synced with GitHub.
- Next/previous comment steps through unresolved threads by default, with a toggle to include resolved ones.

### Checks and freshness
- Read-only Checks panel for the head commit: required Checks marked, merge state (blocked, behind, clean, conflicting), links to logs. Shown as a header pill that expands.
- Polls GitHub every ~30s using ETag conditional requests, and only while a browser is connected (SSE) **and** the page is focused (Page Visibility API).
- Backs off when the rate limit drops below 10% remaining.
- Pushes changes over SSE: a new head SHA shows the Refresh button, and comments and Checks update in place.
- When GitHub is unreachable: read-only mode served from the cache and bare clone, with a banner; review actions disabled.

### Code navigation
- Ctrl-click or right-click "Go to definition" / "Find usages", via `CodeNavigator`. Head side only; a no-op on the left side in v1.
- Covers the whole repo at head: navigating into unchanged files opens a plain file view.
- One Language Server per (PR worktree, language), started lazily and stopped after 10 minutes idle. An **Indexing** spinner shows in the top right.
- **Symbol Search** (ctrl+k): files from `git ls-tree` at head plus LSP workspace symbols, ordered by PR Weighting, with green and red markers for added and removed items.

### Language Server installer
- Settings panel that lists Language Servers from `mason-org/mason-registry` and installs or removes them.
- v1 install sources: `npm`, `go`, `github` release.
- Detects missing runtimes (e.g. node, go) and reports them instead of installing them.

### Interface
- **Command Palette** (ctrl+j): actions and settings, e.g. toggle View Mode, refresh, change Theme, rebind hotkeys.
- Every hotkey is rebindable. Single-key bindings only work when no input is focused: `]c`/`[c` next/previous comment, `s` toggle View Mode, `r` refresh.
- Themes: Gruvbox Dark (default) and Gruvbox Light, plus drop-in theme files. A Theme covers the UI chrome and the tree-sitter capture colours.
- Stack navigation (Parent/Child) uses GitHub's native Stacks only.

## Testing

- Each port has an in-memory fake next to it, used by fast service tests.
- `CodeHost`: tests against recorded GraphQL and REST fixtures. Never calls live GitHub.
- `RepoStore`: diff and blob logic tested against throwaway local `git init` repos. Fetching and worktrees against real repos are **tested manually**.
- `CodeNavigator`: integration tests against a real gopls.

## Milestones

1. **Read-only diff viewer**: `scrutiny serve`, security middleware, config, cache, `RepoStore`, `CodeHost` (read), Dashboard, PR page with split/unified diff, tree-sitter highlighting, Comparison picker, large/generated/binary/rename handling, lazy loading.
2. **Reviewing**: Review Threads, Review Comments, Conversation Comments, Reactions, Pending Review and submit dialog, Viewed, next/previous comment.
3. **Freshness and Checks**: SSE hub, focus- and connection-aware polling, ETags, rate-limit backoff, Refresh button, Checks panel, offline mode.
4. **Code navigation**: PR worktrees, LSP client, `CodeNavigator`, go-to-definition, find usages, file view, Symbol Search, Indexing spinner.
5. **Language Server installer**: Mason registry reader, npm/go/github installers, runtime detection, settings panel.
6. **Polish**: Stacks, Command Palette, rebindable hotkeys, Gruvbox Light and theme files, Since Last Review.
