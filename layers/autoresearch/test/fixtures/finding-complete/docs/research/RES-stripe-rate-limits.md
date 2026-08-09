---
id: res-stripe-rate-limits
title: Stripe's rate limit on the Payment Intents API
status: current
question: How many Payment Intents requests per second before 429s, and what happens at the limit?
sources:
  - https://docs.stripe.com/rate-limits
confidence: high
expires: 2099-01-01
updated: 2026-08-07
---

# Stripe's rate limit on the Payment Intents API

## Finding

100 requests/second in live mode, 25 in test mode, counted per account rather than
per API key. Over the limit returns 429 with `Retry-After` in seconds.

## What would change this

- A different number published on https://docs.stripe.com/rate-limits.
- A 429 observed below 100 requests/second in production.
