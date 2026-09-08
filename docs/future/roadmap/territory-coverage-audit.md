---
title: territory coverage audit
state: inbox
created: 2026-09-08
tags: [feature]
---

Add a command that audits a project's rubric against its full tracked tree: every file inside the declared `coverage:` that no project-local quality reaches, and every territory pattern that reaches no file at all. Run at onboarding and after adding or renaming a package; nothing about a review changes.

## why

The review-time report in `rubric-coverage-declaration` catches a gap when a change happens to touch the uncovered file. This catches it before any change does, and it catches the other direction of the same drift — a territory still naming a package that was renamed or removed, which a review can never notice because no file matches it.

Its first run on an external project produced the `internal/gateway` list the open card is about, so the canon edit that closes that card is made with the list in hand rather than from memory. Its first run across every project is the audit `terminus-canon`'s `territory-audit-command-trees` card describes.

## background

**Shape.** Either a new verb or a flag on `terminus rubrics`, which already composes the rubric and derives each quality's tier. The output is two lists per rubric: uncovered files, grouped by directory so a new package reads as one line, and dead patterns with the quality that carries them. A rubric with no `coverage:` reports only dead patterns.

**Not a review.** No reviewer runs, no record is written, and the verdict vocabulary is untouched. It reads the canon and the tracked file list and prints.
