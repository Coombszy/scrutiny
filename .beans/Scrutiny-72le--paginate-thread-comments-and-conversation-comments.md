---
# Scrutiny-72le
title: Paginate thread comments and Conversation Comments beyond 100
status: todo
type: task
created_at: 2026-11-10T21:46:02Z
updated_at: 2026-11-10T21:46:02Z
---

ReviewState reads comments(first:100) per thread and comments(last:100) for the Conversation without paginating, so very long discussions are truncated. Page through both (PLAN.md: GitHub's model 1:1).
