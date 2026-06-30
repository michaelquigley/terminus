# CHANGELOG

## Unreleased

## v0.1.1

CHANGE: Persist Terminus's own review artifacts through the shared `df/dd` binding
substrate, so Terminus follows the `df-binding` convention it enforces. `status.json`
and `result.json` now marshal and unmarshal via `dd` instead of `encoding/json`; the
reviewer's raw output passes through a `json.RawMessage` converter untouched, and the
round-tripped values are preserved. On-disk keys are now sorted and previously-omitted
empty fields appear, but the MCP/CLI wire format is unchanged.

FIX: Lowercase the reviewer's triage guidance strings so they follow the
`lowercase-output` convention.

FEATURE: Ad-hoc reviews — `terminus review --quality <ref>` (repeatable) and the MCP
`start_review` `qualities` field review against canon quality refs directly, bypassing
rubric resolution. Useful for trying a quality or reviewing a project with no rubric
yet; no rubric file and no `project.repo` check are required. Ad-hoc qualities are
advisory unless `--blocking` / `qualities_blocking` is set, the recorded rubric is
`(ad-hoc)`, and `--quality` cannot be combined with `--rubric`.

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
