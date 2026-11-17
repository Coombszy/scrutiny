---
# Scrutiny-8xeo
title: Keep review state from going stale
status: completed
type: bug
priority: high
created_at: 2026-11-17T17:19:05Z
updated_at: 2026-11-17T17:19:05Z
---

The page or the sqlite cache can drift from GitHub: a ticked Viewed box showed as unviewed again after navigating away and back. Four small fixes, done as one commit.

- [x] **No stale page copies on Back.** The PR page has no `Cache-Control`, so Back/Forward can reuse the HTML from before any htmx changes (Viewed ticks, threads, replies, resolves, reactions, comments). Live updates never repair it: Poll diffs GitHub against the server cache, which already has the change. Fix: `Cache-Control: no-store` on every full-page render in `renderPage` (Dashboard, PR page, Settings); not on fragments or static assets.
- [x] **Writes finish when the browser leaves.** Review writes pass `r.Context()` to GitHub, so leaving mid-request can cancel a mutation, or let it land without the re-read that updates the cache (Discard retries for ~4s). Fix: all handlers in `review_handlers.go` except the Markdown preview, plus `forget`, use `context.WithTimeout(context.WithoutCancel(r.Context()), writeTimeout)`, one shared 30s `writeTimeout` (as `dashboardLists` does).
- [x] **Per-PR lock around review state.** `Poll` and `write` each read GitHub and save the whole snapshot unsynchronised, so a poll whose read began before a write can save after it, rolling the cache back. Then the next comment can try to start a second Pending Review (GitHub refuses), and Poll can push a "new threads" banner for the viewer's own thread. Fix: the review Service holds one mutex per PR for each whole operation: `write`, `Poll` (from reading the previous snapshot to saving), `PRPage`'s fresh read, and `Discard` including its backoff.
- [x] **A browser leaving isn't offline.** `graphql`, `get` and `post` in `codehost/github` call `status.failed()` on any `http.Do` error, including the caller's context ending, so a cancelled request marks GitHub offline until the next response (and `Poll` then skips head moves); `get` also silently falls back to its cached copy. Fix: when `ctx.Err() != nil`, skip `failed()` and the fallback, and return the context error unwrapped (so `fail()` stays quiet, and a `writeTimeout` deadline reads as a timeout).

Tests: handler tests for the header and for a write surviving a cancelled request; a race test on the in-memory fake (a poll overlapping a write); a `github` test that a cancelled context leaves the client Online.


## Summary of Changes

- `renderPage` sends `Cache-Control: no-store` unless the request is from htmx.
- `writeContext` (`web/review_handlers.go`) detaches the review write handlers and `forgetPR` from the request, bounded by a 30s `writeTimeout`; `fail` shows a hit deadline as 504 "GitHub did not answer in time".
- The review Service's `lockPR` holds a mutex per PR across `write`, `Poll`, `PRPage`'s fresh read, `Discard` with its re-reads, a cache-miss `snapshot`, and `ForgetPR`'s snapshot delete; `snapshotLocked` / `writeLocked` are the variants for callers already holding it.
- `codehost/github` returns the context's error, without marking GitHub offline or falling back to the cache, when the caller's context has ended.
- `codehost.Fake` fails reads and writes once the context is done (`reachable`), and has an `AfterReviewState` hook for interleaving tests.
