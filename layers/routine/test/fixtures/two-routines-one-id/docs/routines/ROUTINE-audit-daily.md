---
id: audit
title: Audit dependencies nightly
status: active
schedule: "0 4 * * *"
command: npm audit --audit-level=high
budget: 15
owner: platform
review_by: 2099-01-01
---

# Audit dependencies nightly

`ilk routine run audit` cannot tell which of these it was asked for.
