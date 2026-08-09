---
id: res-webhook-retry-schedule
title: How often the vendor retries a failed webhook
status: current
question: How many times, and over how long, does the vendor retry a webhook we 500 on?
sources:
  - support told us in a call
confidence: medium
expires: 2099-01-01
updated: 2026-08-07
---

# How often the vendor retries a failed webhook

## Finding

Eight attempts over 24 hours, with exponential backoff, then the endpoint is
disabled and an email is sent.

## What would change this

- The vendor's webhook documentation stating a different schedule.
- A ninth delivery observed, or a disable email arriving early.
