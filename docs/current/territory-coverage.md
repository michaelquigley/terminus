# Territory coverage

Rubric reviews report starting-point files that no project-local quality reaches. Reporting is enabled by default and is independent of the findings verdict: a review can be `clean` and still expose coverage gaps. Coverage neither widens quality selection nor suppresses findings.

## Rubrics and reach

A quality is project-local when its canon ref is beneath `projects/<resolved-project>/` and it appears in the selected, composed rubric. General conventions, qualities from another project, and local qualities omitted from this rubric do not count. A local quality without territory reaches every input file; that intentionally removes zero-coverage gaps, without proving that every relevant invariant has a quality.

Each named rubric may declare exceptions using the existing territory glob language:

```yaml
coverage_exclusions:
  - docs/**
  - demo/**
```

These patterns suppress uncovered-file complaints only. They do not remove starting points, alter selected qualities, or prevent findings on excluded files. An absent or empty list means no exceptions. Territory and exclusion patterns are validated even when the input scope is empty.

Canon loading owns rubric-name validation, filename selection, and reported identity for both review and audit. An empty request selects `rubric`; `architecture` and `architecture.yaml` select the same file and report `architecture`. A suffix is consumed once: `.yaml` is invalid, and a request for `architecture.yaml.yaml` selects that exact file or fails if it is missing.

## Review snapshots

The broker calculates coverage from the full composed rubric and the review's existing starting-point file set before invoking the reviewer. The first running `status.json`, completed or failed status, completed `result.json`, and live or restarted-broker collection retain that assessment. Editing canon during a review does not change its snapshot; a fresh audit reloads canon.

The report covers starting points, not every file the reviewer may follow. Working-tree scope includes untracked additions and deleted paths. Path and full scopes use tracked paths. No additional filesystem-existence filter is applied to coverage, so a deleted path can describe scrutiny of its removal.

Ad-hoc reviews bypass rubric loading and report `assessed: false` with `reason: ad_hoc`. Historical records without `coverage` remain unavailable; collection does not reconstruct them from current canon. The monitor CLI stays a lean status view; the coverage object is available in its status artifact.

## Coverage object

Review status, completed results, collection, and audit share this object:

| Member | Meaning |
| --- | --- |
| `assessed` | Required boolean; true for rubric assessments, including empty scopes and rubrics without local qualities. |
| `reason` | Omitted when assessed; `ad_hoc` for an ad-hoc review. |
| `file_counts` | Present only when assessed, with `total`, `excluded`, `covered`, and `uncovered` integers. |
| `local_qualities` | All local qualities in the composed rubric as `{id, ref}`, sorted by ref. |
| `uncovered_files` | Sorted repository-relative paths with neither a local match nor an exclusion. |
| `exclusions` | Distinct declared patterns, sorted, each as `{pattern, files}` with every matching input path. Unmatched patterns retain `files: []`. |

Counts are disjoint: `total = excluded + covered + uncovered`. Exclusions take precedence in the counts, and overlapping exclusions count a file once. Exclusion attribution and the optional map still retain a file's actual local-quality matches. `uncovered` equals the length of `uncovered_files`.

All collection members are arrays, including empty arrays, never `null`. Quality refs are extensionless canon-relative paths; file paths use repository-relative slash notation. Ad-hoc output is:

```json
{
  "assessed": false,
  "reason": "ad_hoc",
  "local_qualities": [],
  "uncovered_files": [],
  "exclusions": []
}
```

An empty scope has all counts zero. A fully excluded scope has a positive total equal to `excluded`. Neither is evidence of local scrutiny. When there are no local qualities, structured output retains the uncovered paths, while the CLI prints a compact absent-local-tier notice and count, or the empty/all-excluded scope explanation.

## Audit and optional map

Call MCP `audit_coverage` with required `repo_path`, optional `rubric` (default `rubric`), and optional `include_map` (default false). The CLI counterpart is `terminus audit-coverage --repo <path> --rubric <name> [--include-map]`, defaulting to the current directory. The result is immediate: there is no review id or start/monitor/collect cycle.

The audit uses the full tracked tree, runs no reviewer, writes no review record, allocates no broker job, and changes neither project nor canon. The CLI does not construct a reviewer or initialize review-log storage. Gaps and dead patterns are successful diagnostic output; invalid requests, invalid canon, and Git enumeration failures are errors.

| Result member | Meaning |
| --- | --- |
| `project` | Repository-basename project identity, validated against the rubric's `project.repo`. |
| `rubric` | Normalized identity of the loaded rubric. |
| `file_scope` | Always `full`. |
| `coverage` | The shared assessment above. |
| `dead_patterns` | `{quality: {id, ref}, pattern}` entries, sorted by ref then pattern. |
| `coverage_map` | Omitted unless requested; an array of `{directory, files}` groups when requested, including `[]` for an empty tree. |

Dead-pattern detection checks explicit territories on every composed quality, including general and foreign-project tiers. Duplicate ref/pattern pairs collapse. Patterns matching only coverage-excluded files are live; territory-free qualities have no explicit patterns to report as dead.

Map groups use the immediate parent directory, with `.` for repository-root files. Each file carries its exact path, matching local `{id, ref}` qualities, and matching declared exclusion patterns. Every input file appears, including covered and excluded files. Groups, files, quality refs, and patterns are sorted. For example:

```json
{
  "directory": "internal/config",
  "files": [
    {
      "file": "internal/config/load.go",
      "qualities": [{"id": "config-validation", "ref": "projects/example/config-validation"}],
      "exclusion_patterns": []
    },
    {
      "file": "internal/config/notes.md",
      "qualities": [],
      "exclusion_patterns": ["**/*.md"]
    }
  ]
}
```

MCP discovery describes the optional map and advertises `readOnlyHint: true` and `destructiveHint: false`. Use the map to inspect uneven reach even among already-covered files, propose canon edits, and rerun the audit. It is evidence for judging invariants, not a score or an automatic repair plan.

## Compatibility and limits

Full scope follows Git's index: untracked files are absent; staged additions are present; unstaged deletions remain tracked and can keep a pattern live; staged deletions disappear. An unstaged rename leaves the old tracked path until staging; a staged rename uses the destination. Existing Git parser limitations for unusual filenames remain unchanged.

Old binaries reject `coverage_exclusions` under strict rubric parsing. Install parser support before adding that key to shared canon, and reconnect MCP clients to the updated server before relying on audit discovery. Historical results need no backfill.

Coverage describes one rubric and one file set. It does not aggregate review passes, infer missing invariants, impose low-coverage thresholds, or automatically widen selection, block verdicts, or edit canon. [Follow-on boundaries](../future/territory-coverage-follow-ons.md) preserve the separately scoped work.
