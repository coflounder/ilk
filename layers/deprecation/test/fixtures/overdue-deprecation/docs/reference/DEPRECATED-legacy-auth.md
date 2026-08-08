---
id: dep-legacy-auth
title: Legacy auth is being removed
status: current
updated: 2020-01-01
announced: 2019-06-01
remove_after: 2020-01-01
covers:
  - src/auth/legacy/**
---

# Legacy auth is being removed

The failure this layer exists for: the date passed, and the code it covers is still
in the tree.

The date is hardcoded in the past rather than computed, so this case means the same
thing in every year the suite is run.
