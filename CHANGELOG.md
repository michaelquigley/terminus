# CHANGELOG

## Unreleased

## v0.1.0

FEATURE: Terminus review spine — a local MCP code-review broker. It points a
fresh-context reviewer (`pi`, `codex`, or `claude` via `theharnessbody`) at a repo
and review target, judges the code against a central canon of qualities selected
by a project rubric, and returns a structured `clean`/`not_clean` verdict. The
target (working-tree changes, paths, or the full tracked repo) is a set of
starting points, not a fence: the reviewer follows a change outward and files
findings wherever the problem lives. Qualities are self-contained markdown with
strict frontmatter (`id`, `applies_to`, `territory`); rubric-derived blocking
drives the verdict; runs are backgrounded, monitorable via status files, and
recorded immutably outside the subject repo through `theharnessbody/record`.

FEATURE: Reviewer prompt steers the reviewer to each quality's own
`discrimination`/`boundary` calibration, prefers fewer material findings (zero is
a valid result), sizes fixes conservatively, and states the output constraints
that otherwise fail a whole review (unique finding ids, selected quality ids,
null-not-empty `suggestion`).

FEATURE: Named rubrics per project — `projects/<project>/<name>.yaml`, default
`rubric.yaml`, selected with `terminus review --rubric <name>` or the MCP
`start_review` `rubric` field, and listed with `terminus rubrics`. Blocking is
per-rubric; the selected rubric is recorded in `status.json`/`result.json`.
Rubrics are independent hand-authored lists; include/compose is deferred.

FEATURE: CLI surface — `terminus serve` (stdio MCP server), `terminus review`
(foreground review; a clean working tree under the default `--kind` promotes to a
full review), `terminus rubrics`, `terminus monitor`, and `terminus version`. Bare
`terminus` prints command help. Non-clean reviews are successful executions; only
operational failures return non-zero.

FEATURE: CI build integration via the shared `push` framework — the workflow vets,
tests, builds a stamped linux-amd64 binary with `push/ci/ldflags.sh`, verifies
`terminus version` is stamped, uploads an artifact, and drafts a release on `v*`
tags.
