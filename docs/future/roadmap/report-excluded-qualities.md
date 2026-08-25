---
title: report excluded qualities
state: horizon
created: 2026-08-12
tags: [enhancement]
milestone: v0.1.x
source: review-loop feedback, sterling config arc 2026-08-07
log:
  - stamp: 2026-08-25
    note: failure mode corrected — the "No qualities selected" line lives in the review's _prompt.md, not command output; the command prints a vacuous clean, it never errors
---

Territory narrowing can silently drop qualities a review was configured to apply — and in the worst case it drops all of them. A review that runs against an empty quality set still runs and reports a vacuous `clean`; nothing in the output says the verdict covered nothing, and the only trace is a "No qualities selected" line buried in the review's `_prompt.md`. The natural escape hatch fails the same way: an explicit `--quality` runs through the same territory narrowing, so a quality a human named by hand can be filtered out of the review.

Two changes. Surface the exclusions: when territory drops rubric-listed qualities, say so where the verdict is reported — a one-line count in the CLI output, and the excluded quality refs in the review record (`result.json`) and collect response — so a `clean` verdict is honest about how much it actually covered. And let explicit `--quality` entries bypass territory: a human naming a quality by hand has already made the judgment the filter automates.

## why

Sterling stage 1 came back `clean` with all seven project qualities excluded by territory; the intended override was filtered too, so there was no way to ask the question, only to discover it hadn't been asked. The first review under the widened rubric found a real bug shipping since v1.