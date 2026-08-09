---
id: res-cdn-cache-headers
title: Which cache headers the CDN honours
status: current
question: Does the CDN honour `stale-while-revalidate`, and on which plan?
sources:
  - https://example.com/cdn/docs/caching
confidence: medium
expires: 2024-03-01
updated: 2023-12-01
---

# Which cache headers the CDN honours

## Finding

`stale-while-revalidate` is honoured on the business plan and ignored on the free
plan, where it is silently dropped rather than rejected.

## What would change this

- The CDN's caching page listing the directive for the free plan.
- A revalidation observed on a free-plan property.
