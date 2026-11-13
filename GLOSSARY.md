# Scrutiny

A local tool for reviewing GitHub pull requests: richer diffs, code navigation and the full review workflow, served as a webpage from your own machine.

## Reviewing

**Pull Request (PR)**:
A proposed change from a head branch into a base branch on GitHub; the thing being reviewed.
_Avoid_: Merge request, MR, change

**Comparison**:
The pair of commits a diff is shown between: by default the PR's merge base against its head, or a contiguous run of commits from the head branch's current linear history.
_Avoid_: Range, interdiff, revision

**Since Last Review**:
The Comparison from the head commit at your last submitted review to the current head; unavailable once that commit has left the branch.
_Avoid_: New changes, delta

**Pending Review**:
A reviewer's not-yet-submitted review on GitHub, holding their draft comments until they submit it as Approve, Request changes or Comment, or discard it, deleting them.
_Avoid_: Draft, local draft

**Review Thread**:
A line-anchored conversation on a PR's diff that can be resolved or unresolved.
_Avoid_: Comment chain, discussion

**Review Comment**:
A single comment within a Review Thread.
_Avoid_: Inline comment, note

**Single Comment**:
A Review Comment posted straight away rather than held in a Pending Review, either starting a Review Thread or replying in one.
_Avoid_: Standalone comment, immediate comment

**Conversation Comment**:
A PR-level comment not anchored to any line.
_Avoid_: General comment, issue comment

**Reaction**:
One of GitHub's eight fixed emoji responses attached to a comment or PR.
_Avoid_: Emoji, like

**Viewed**:
A per-file, per-reviewer mark that you have reviewed a file, shared with GitHub and cleared when the file changes.
_Avoid_: Seen, read, done

**Check**:
A CI result (check run or commit status) reported against the PR's head commit; a Required Check is one that must pass before merging.
_Avoid_: Build, action, job

**Dashboard**:
The home page listing the PRs that matter to you, such as review requests, PRs you authored and PRs you opened recently.
_Avoid_: Inbox, home

## Stacks

**Stack**:
GitHub's native ordered chain of PRs, where each PR targets the head branch of the PR below it.
_Avoid_: Chain, train

**Parent / Child**:
In a Stack, the parent is the PR directly below (whose head branch this PR targets), and the child is the PR directly above.
_Avoid_: Previous/next, upstream/downstream

## Code navigation

**Language Server**:
An installable external LSP program that provides definitions, usages and symbols for one language.
_Avoid_: LSP (when meaning the program), plugin

**Indexing**:
The period after a Language Server starts and before it can answer navigation requests.
_Avoid_: Loading, warming up

**PR Weighting**:
Ranking search results so files and symbols added or removed by the PR come before the rest of the repo.
_Avoid_: Boost, priority

**Symbol Search**:
The ctrl+k fuzzy finder over the repo's files and symbols at head, ordered by PR Weighting.
_Avoid_: Global search, telescope, fuzzy finder

**PR Search**:
The ctrl+p finder, on every page, that jumps to a PR from the Dashboard's lists and those recently opened, or one named by URL, owner/repo#123 or (on a PR's pages) #123.
_Avoid_: PR switcher, quick open

## Interface

**Command Palette**:
The ctrl+j fuzzy finder over actions and settings, such as toggling the view mode or changing Theme.
_Avoid_: Settings search, command menu

**View Mode**:
Whether a diff is shown Split (side-by-side) or Unified.
_Avoid_: Layout, diff style

**Theme**:
A named set of colours for both the interface and syntax highlighting.
_Avoid_: Colour scheme, skin

**Userscript**:
The Tampermonkey script that adds a Goto Scrutiny button to GitHub PR pages while Scrutiny is running.
_Avoid_: Extension, plugin, bookmarklet

**Goto Ticket**:
A single-use pass, valid for 30 seconds, that the Userscript fetches so Scrutiny lets it open a PR page in GitHub's own tab, a navigation otherwise refused as cross-site.
_Avoid_: Token, nonce

**New Version**:
A Scrutiny release tag newer than the running release build, found on the Go module proxy and offered by the top bar's version; dev and dirty builds never look for one.
_Avoid_: Update, upgrade
