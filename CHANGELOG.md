# CHANGELOG

## Unreleased

CHANGE: `terminus review` now promotes a defaulted working-tree review to a full
review when the working tree is clean, announcing `working tree clean; reviewing
full tracked repo`. A bare `terminus review` on a committed repo previously
selected nothing and returned a vacuous `clean`; it now reviews the project. An
explicit `--kind working-tree` is always honored, and the MCP `start_review`
contract is unchanged.

FEATURE: Add Mercurius-style CI build integration using the shared `push`
framework. The workflow now vets, tests, builds a stamped linux-amd64 binary
with `push/ci/ldflags.sh`, verifies `terminus version` is stamped, uploads an
artifact, and drafts a release on `v*` tags.

CHANGE: Bare `terminus` now prints command help instead of starting the MCP
server. Use `terminus serve` as the explicit stdio MCP server entry point.

FEATURE: Foreground `terminus review` CLI command — a human operator can now run
the same review target as MCP `start_review` without starting the MCP server.
The command supports working-tree, paths, and full review modes, waits for the
review to complete, writes the same review artifacts, and prints verdict,
finding counts, prompt/log paths, and compact finding details. Non-clean reviews
are successful command executions; only operational failures return non-zero.

FEATURE: Initial Terminus review spine — a local MCP code-review broker with
strict canon quality parsing, hand-authored project rubrics, changeset narrowing,
working-tree/paths/full review modes, flat reviewer findings, rubric-derived
blocking classification, `clean`/`not_clean` verdicts, background review runs,
monitorable status files, and durable findings records written outside the
subject repo through `theharnessbody/record`.
