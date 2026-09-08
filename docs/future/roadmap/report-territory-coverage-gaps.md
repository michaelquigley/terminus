---
title: report territory coverage gaps
state: researching
created: 2026-09-08
tags: [feature]
milestone: v0.1.x
log:
  - stamp: 2026-09-08
    note: draft spec at docs/future/territory-coverage.md; shared model with territory-coverage-audit, open decisions pending
---

By default, have rubric-based reviews report starting-point files that no project-local quality in the selected rubric reaches. Let each rubric suppress intentional gaps with a `coverage_exclusions:` list of territory globs beside `qualities:`, such as `docs/**` or `demo/**`. The report is a first-class field beside the verdict in `result.json`, the CLI output, and the MCP `collect_review` response, never only a line in the quality list. Report gaps without widening quality selection or changing the verdict calculation. Exclusions suppress coverage complaints only; they do not exclude files from review or change which qualities run. An absent `coverage_exclusions:` means report all gaps.

## why

Territory narrowing is a precision device: it keeps a reviewer from being handed sixteen qualities when three apply. Its failure mode is that it silently drops scrutiny. A project-local quality names the packages its invariant lives in, so a new package matches nothing, the review runs on the general conventions alone, and the verdict says `clean` with a straight face. An external project hit this twice — `internal/config` and `cmd/` on 2026-08-07, `internal/runrecord` on 2026-09-01 — and `internal/gateway` is open now. Each was closed by a one-line territory patch, and the card that raised the third asked for the general answer instead.

The root is a cross-repo coupling: adding a package to a project implies an edit in the canon, and neither repo knows about the other. Terminus is the one process that holds both trees at once, so it is where the coupling can be checked.

The selected and excluded qualities are already in the result, and a standing instruction to read them rather than the verdict has been in force since August. It still took a journal note to catch the second instance. A field nobody is made to read is not a fix; a report that names the files, on the surface the verdict is read from, is.

## background

**Default reporting, explicit exceptions.** No positive `coverage:` declaration is required. A new tree should expose a gap automatically, including a new `cmd/` tree that an `internal/**` declaration would miss. The rubric author names intentional exceptions with `coverage_exclusions:`. Existing rubrics gain reporting by default and may need exclusions for trees where project-local scrutiny is not expected. This also makes the command-tree choice behind `terminus-canon`'s `territory-audit-command-trees` card explicit: report its gaps unless the rubric excludes it.

**Reporting only, initially.** Selecting the whole project-local tier may supply rules about unrelated packages; it does not establish that an uncovered package has an appropriate quality. Report the gap alongside the findings-based verdict so the operator can extend an applicable quality's territory or add a missing rule. This is a check of territory reach, not proof that the rules are sufficient.

**Design.** The shared [territory coverage spec](../territory-coverage.md) records the agreed behavior and boundaries. The work order will specify exact output members and serialization details.

**Related.** `territory-coverage-audit` is the same check over the full tracked tree, run by hand rather than per review.
