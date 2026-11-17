---
# Scrutiny-m70y
title: Revert a Viewed tick when its request fails
status: completed
type: bug
priority: normal
created_at: 2026-11-17T17:19:05Z
updated_at: 2026-11-17T17:19:05Z
---

Ticking Viewed collapses the file and marks its File Tree row before the POST (`scrutiny.js`, the `.viewed-toggle` change handlers). If the request fails, a toast shows but the box stays ticked and the file collapsed while GitHub says unviewed, so a reload "unticks" it. Quick tick/untick clicks can also reach GitHub out of order.

- Disable only that file's checkbox while its request is in flight (`hx-disabled-elt="this"`); other files stay clickable, so several can be ticked in a row.
- On `htmx:responseError` / `htmx:sendError` for a Viewed request, revert the checkbox, the file's `viewed` class and collapse, and the File Tree row. The toast stays.


## Summary of Changes

The Viewed checkbox carries `hx-disabled-elt="this"`, so only that file's box is disabled while its request runs. `showViewed` in scrutiny.js sets a file's checkbox, `viewed` class and collapse and fires `scrutiny:viewed`, which the File Tree follows; on `htmx:responseError` / `htmx:sendError` from a Viewed checkbox it is called with the opposite state. Verified in headless Chromium against fakeserve with the POST failed by request interception: other boxes stay enabled in flight, the failure undoes box, file and tree row and shows the toast, a success persists across a reload.
