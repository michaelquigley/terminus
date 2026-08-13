---
title: surface territory-filtered qualities
state: horizon
created: 2026-08-12
tags: [enhancement]
milestone: v0.1.x
source: review-loop feedback, sterling config arc 2026-08-07
---

A review that filters project qualities out by territory reports `clean` without saying so, and an explicit `--quality` naming an excluded quality returns "No qualities selected" instead of running it. Report excluded qualities in the review output (a one-line count is enough), and let explicit `--quality` override territory — a human naming a quality by hand has already made the judgment the filter automates.

## why

Sterling stage 1 came back `clean` with all seven project qualities excluded by territory; the override was filtered too, so there was no way to ask the question, only to discover it hadn't been asked. The first review under the widened rubric found a real bug shipping since v1.
