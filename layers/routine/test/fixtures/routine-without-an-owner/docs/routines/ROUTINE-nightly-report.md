---
id: nightly-report
title: Build the nightly report
status: active
schedule: "0 2 * * *"
command: ./scripts/report.sh
budget: 20
review_by: 2099-01-01
---

# Build the nightly report

Nobody's. Which means that when it breaks, whoever notices is whoever needed the
report — and on a scheduled job, nobody notices.
