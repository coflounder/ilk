---
id: res-oauth-refresh-window
title: How long a refresh token stays valid
status: current
question: How long can a refresh token go unused before the provider revokes it?
sources:
  - https://example.com/auth/docs/refresh-tokens
confidence: medium
updated: 2026-08-07
---

# How long a refresh token stays valid

## Finding

Ninety days of non-use, after which the token is revoked and the user must consent
again.

## What would change this

- The provider changing the window on its refresh-token page.
- A revocation observed earlier than ninety days.
