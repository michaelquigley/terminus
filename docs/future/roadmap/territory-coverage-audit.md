---
title: territory coverage audit
state: building
created: 2026-09-08
tags: [feature]
milestone: v0.1.x
log:
  - stamp: 2026-09-08
    note: draft spec at docs/future/territory-coverage.md; MCP audit and optional coverage map agreed, open decisions pending
---

Add a read-only MCP tool and CLI counterpart that audit a project's rubric against its full tracked tree: every file that no project-local quality in that rubric reaches, except files matching its `coverage_exclusions:`, and every quality territory pattern that reaches no tracked file at all. Check uncovered files by default, with no positive `coverage:` declaration required. Offer an optional coverage map showing which local qualities reach which files, grouped by directory without hiding file-level differences. Return structured evidence agents can use to propose canon edits and rerun the audit. Run at onboarding and after adding or renaming a package; the audit does not run or alter a review, or edit the canon.

## why

The review-time report in `report-territory-coverage-gaps` catches a gap when a change happens to touch the uncovered file. This catches it before any change does, and it catches the other direction of the same drift — a territory still naming a package that was renamed or removed, which a review can never notice because no file matches it.

Its first run on an external project produced the `internal/gateway` list the open card is about, so the canon edit that closes that card is made with the list in hand rather than from memory. Its first run across every project is the audit `terminus-canon`'s `territory-audit-command-trees` card describes.

## background

**Shape.** MCP is the primary agent surface; the CLI exposes the same audit capability. Results identify uncovered files and dead patterns with the qualities that carry them, plus an optional detailed coverage map. Directory grouping makes a new package legible without losing exact file paths or matching quality refs. Use quality identities rather than coverage scores: one matching quality does not prove every relevant invariant is covered. A rubric with no `coverage_exclusions:` checks all tracked files for gaps. The shared [territory coverage spec](../territory-coverage.md) records the agreed interfaces and behavior.

**Shared check.** Use the same definition of project-local coverage and the same rubric exclusions as `report-territory-coverage-gaps`; its remaining coverage decisions apply here too. Exclusions suppress uncovered-file complaints only. Check dead territory patterns against the full tracked tree, including excluded trees, so a pattern that reaches an intentionally excluded file is not falsely reported as dead.

**Not a review.** No reviewer runs, no review record is written, and the verdict vocabulary is untouched. It reads the canon and the tracked file list and returns evidence; canon edits stay in the ordinary reviewable file workflow.
