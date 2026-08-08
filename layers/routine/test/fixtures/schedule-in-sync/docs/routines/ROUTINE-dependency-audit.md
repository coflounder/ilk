---
id: dependency-audit
title: Audit dependencies for published advisories
status: active
schedule: "0 4 * * 1"
command: npm audit --audit-level=high
budget: 15
owner: platform
review_by: 2099-01-01
---

# Audit dependencies for published advisories

## What this is for

No dependency in this repository has a known high-severity advisory against it for
longer than a week.

## Why it has to be scheduled

The trigger is outside the repository: an advisory is published against a version we
already have, on somebody else's clock. Nothing anybody does here causes it, so there
is no commit for a check to hang off.

## What happens when it fails

The platform owner is paged by the workflow's own failure notification, and files the
upgrade as ordinary work. A failure here is never urgent enough to wake somebody.
