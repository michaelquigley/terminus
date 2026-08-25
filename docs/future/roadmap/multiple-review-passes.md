---
title: multiple review passes
state: researching
created: 2026-08-25
tags: [epic, spike]
milestone: v0.1.x
---

include support for multiple review passes. this will be useful for the [[grouped-reviews]] implementation, and also for additional types of review passes.

today an agent or operator can manually invoke different configurations; this feature would allow multiple passes/configurations to all aggregate into a single "review cycle" and report.

## discussion

the report-excluded-qualities work (2026-08-25) landed per-pass coverage: each review records its selected and excluded qualities on its result and status. a cycle that aggregates multiple passes needs a rule for what honest coverage means across them. the natural one, stated once: a quality counts as covered for the cycle if any pass selected it, and the cycle's excluded set is the union of the per-pass exclusions minus every quality some pass selected. two passes, one covering go files and the other the rest, are then genuinely clean at cycle level even though each pass alone would print a vacuous-scope line — a quality pass one excluded and pass two ran is covered, so it never appears as cycle-level excluded. without that rule, an aggregation that just concatenates the per-pass exclusion lists will alarm on vacuous scope for fully covered cycles, because the raw per-pass union can name a quality another pass selected and thereby covered.

a second seam rides in: the territory bypass means a `--quality` pass inherits the starting-points-not-a-fence model, so "spot-check this quality on these files" silently becomes "this quality on the whole tree, reached from these files." fine for forcing a question a filter would answer no to, but a multi-pass design should name it deliberately — a pass that means "check quality x on the files i changed" is a different review shape than "check quality x everywhere", and the design should decide which shape a named pass is.
