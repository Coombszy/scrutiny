---
# Scrutiny-dnh7
title: Push Viewed changes made elsewhere to open pages
status: completed
type: bug
priority: normal
created_at: 2026-11-17T17:19:05Z
updated_at: 2026-11-17T17:19:05Z
---

`review.Update` has no Viewed field, so Viewed changes made on github.com or in another tab only show after a reload.

- `Update` carries the Viewed states that changed (by path), including Dismissed.
- The page updates the checkbox, the file's `viewed` class, the "Changed since viewed" badge and the File Tree row, without collapsing or expanding the file (no content moving under the reader).
- Files whose checkbox is disabled (their own request in flight) are skipped, so a poll that read GitHub just before a click cannot flip it back.

Test: a `Poll` test on the in-memory fake.


## Summary of Changes

`Poll` puts the files whose Viewed state changed in `Update.Viewed`, and the event stream sends them as a `viewed` event of JSON by path (an `update` event is now only written when it has HTML). `Scrutiny.applyViewed` shows each through `showViewed` without collapsing or opening the file, skipping a checkbox whose own request is in flight. The "Changed since viewed" badge is always rendered, hidden unless Dismissed, and `showViewed` (which now takes GitHub's state) keeps it right for local ticks, failed-tick undos and live changes. Verified in headless Chromium against fakeserve.

Limit: a tick made in another Scrutiny tab is not pushed, since that tab's write already updated the shared snapshot the poll compares against; that is Scrutiny-uvan.
