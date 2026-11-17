---
# Scrutiny-p7it
title: File Tree beside the diff with change status and Current File eye
status: completed
type: feature
priority: normal
created_at: 2026-11-17T11:28:35Z
updated_at: 2026-11-17T12:55:24Z
---

A collapsible File Tree on the left of the PR page, like GitHub's.

- Sticky left column (~18rem), viewport-tall, own scroll; pushes the diff right. Desktop only.
- Real folder tree: folders expandable, all expanded at first; single-child folders merged; folders first, then files, alphabetical.
- Status icons: added `+` green, deleted `−` red, modified `•` yellow, renamed `→` blue (under the new path, old path in the tooltip; deletions under the old path).
- Viewed files dimmed with ✓, updated live on tick; 💬 count of unresolved Review Threads.
- Current File (the file section under the mouse pointer, re-checked on scroll) marked with an eye. Pointer over anything else keeps the last one; no eye before the first hover. The tree never auto-scrolls to it.
- Click scrolls to the file and sets `#file-N`; Viewed files stay collapsed.
- Collapsed, the tree is hidden (no rail); a `»` button leads the files bar and every sticky file header. Toggle with those, the tree's `«` button, the `t` hotkey (`toggle_file_tree`, rebindable) or the Command Palette. Saved as `file_tree = "open" | "collapsed"` in config.toml (default open).
- The file header stuck under the top bar stretches across the whole page; an open File Tree's head ("Files «") sits in that bar.
- The tree's width drags from its edge (or arrow keys on the handle), 160px to half the window, kept in localStorage; a double click resets it.


## Summary of Changes

File Tree beside the PR page's diff (`views/filetree.go`, `fileTreeView` in pr.templ, a block in scrutiny.js). The page's files are sorted by `review.TreeOrder`, so the diff, the tree and comment navigation share one order. Collapsed, the tree is hidden and `»` leads the files bar and each file header; the stuck file header spans the page, with the tree's head in that bar. `file_tree` setting with `POST /settings/file-tree` (no reload) and the `toggle_file_tree` hotkey. Verified in headless Chromium against fakeserve: Current File eye on hover and scroll, click-to-jump, Viewed sync, collapse/open, stuck bar alignment, width drag/keys/reset/persistence.
