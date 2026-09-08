# Territory coverage

Design decisions settled, 2026-09-08. This spec joins [report territory coverage gaps](roadmap/report-territory-coverage-gaps.md) and [territory coverage audit](roadmap/territory-coverage-audit.md). It records the agreed behavior and architectural boundaries. The [work order](territory-coverage-work-order.md) specifies the exact wire schema, implementation placement, and validation. The pair completed Mercurius review with a `ready_to_build` verdict; implementation has not started.

## The problem

Terminus selects rubric qualities by whether their territory reaches the review's starting-point files. It reports selected and excluded qualities, but that does not answer the inverse question: which files receive no project-local scrutiny? A new package can match none of the local rules, receive only general conventions, and return `clean` without exposing that gap.

Code and canon live in separate repositories. Terminus holds both, so it can compare their paths directly. The comparison is a lint of territory reach: it identifies gaps and stale patterns, while the operator and agent decide which invariants should apply. A matching quality does not prove that all relevant invariants are represented.

The primary user is an agent working on a project through MCP. It needs structured evidence for proposing canon changes, then a way to rerun the check against those changes. CLI output serves the same work for a human.

## Agreed behavior

Rubric-based reviews report starting-point files reached by no project-local quality in the selected rubric. Reporting is on by default. There is no positive `coverage:` declaration to maintain when a new tree appears.

Each rubric may declare intentional exceptions beside `qualities:`:

```yaml
coverage_exclusions:
  - docs/**
  - demo/**
```

These patterns suppress uncovered-file complaints only. They do not remove files from the changeset, prevent findings on those files, or affect quality selection. An absent or empty list means no exceptions. Each named rubric owns its own exceptions because different review lenses may legitimately expect different local scrutiny.

Coverage is separate from the findings-based verdict. Gaps neither widen the selected quality set nor change `clean`, `not_clean`, or finding blocking status. Selecting every local quality would supply more rules without establishing that any is appropriate for the uncovered package.

A read-only audit is available through MCP and a CLI counterpart. For a project's rubric and full tracked tree, it reports uncovered files and quality territory patterns that match no tracked file. An optional coverage map shows which project-local qualities reach which files, grouped by directory without hiding file-level differences. It provides names and refs rather than a coverage score or a threshold that equates more qualities with better scrutiny.

The audit runs no reviewer, creates no review record, and edits neither project nor canon. Agents use its evidence to propose ordinary, reviewable canon file edits and run the audit again. Automatic canon mutation is outside this feature.

## One coverage model

The review and audit use the same rubric interpretation and territory matching semantics. Existing territory patterns are repository-relative slash-path globs with recursive `**`; a trailing slash means the tree beneath it. Coverage exclusions use that same language.

Centralize coverage logic in one shared calculation. Reviews and audits supply their respective file sets and use the same rules for project-local identity, territory matching, and coverage exclusions. MCP serialization and CLI formatting sit outside that calculation; neither adapter independently decides whether a file is covered. Keep wire-schema knowledge out of the coverage model.

Project-local qualities are those whose canon refs are beneath `projects/<resolved-project>/` and that appear in the selected rubric. This uses the existing canon organization without adding tier metadata. A quality omitted from this rubric cannot account for this rubric's reach; general conventions and qualities imported from another project's directory do not satisfy the local-coverage check.

A project-local quality with no territory reaches every input file, consistent with today's always-applies selection behavior. One such quality eliminates zero-coverage reports for that rubric; this is intentional. The detailed coverage map still shows where additional, more specific qualities apply.

For each input file, determine its matching local qualities and matching coverage exclusions independently. A file with no local match and no exclusion is reportable as uncovered. An exclusion may suppress a complaint even though the file is also reached by a quality; it does not erase that relationship from the coverage map.

A rubric with no project-local qualities still receives a coverage assessment. Structured review and audit results identify the absent local tier and retain every uncovered, non-excluded file path. The CLI gives a compact notice with the uncovered count instead of listing every path, for example `coverage: rubric contains no project-local qualities; 42 files uncovered`. The count reflects the input scope and exclusions; an absent local tier does not itself disable reporting or imply complete coverage.

Dead-pattern detection is a separate audit result. Test each explicit territory pattern on the rubric's composed qualities against the full tracked tree, including trees excluded from coverage complaints. A pattern is not dead merely because its matches were intentionally excluded from gap reporting. A quality with no territory has no explicit pattern to label dead.

## Review-time reporting

Report coverage as a first-class field beside the verdict in `result.json` and the MCP `collect_review` response, and as a distinct line with file details in CLI output, using the compact notice above when the rubric has no local qualities. MCP tool descriptions and collect guidance must explain that a `clean` finding verdict can coexist with coverage gaps and direct the caller to inspect them.

For example, if local qualities reach `internal/config/**` and `internal/router/**`, adding `internal/gateway/handler.go` produces an uncovered-file report even if general-convention review returns no findings:

```text
verdict: clean
coverage: 1 file has no matching project-local quality
  internal/gateway/handler.go
```

The report describes the review's starting points, not every file the reviewer might follow. It uses the rubric and file set resolved for that review; collecting an old result must not recompute it from a subsequently edited canon. This keeps the report meaningful when an agent patches a territory and runs another review.

Coverage is computed before the reviewer starts and appears in `status.json` from the first status write. The completed result and collect response carry that same assessment unchanged, so a running review exposes its gaps immediately and a later verdict does not redefine its coverage.

Structured reporting uses one `coverage` object containing whether coverage was assessed, file counts, uncovered paths, and applied exclusions. It distinguishes an assessment with no gaps from an assessment where every input file was excluded, and from coverage not assessed with an explicit reason. Counts must also distinguish an empty input scope from a nonempty scope whose files were all excluded. The work order specifies exact member names and serialization details for this model.

Older results without a `coverage` field mean coverage information is unavailable. Reading them must not manufacture an assessed empty gap list or otherwise present missing historical information as proof of coverage.

Ad-hoc reviews (`--quality` or MCP `qualities`) explicitly report coverage as not assessed, with the reason that the review is ad-hoc. They continue to bypass rubric resolution; no rubric is loaded solely for coverage checking. The CLI says `coverage: not assessed — ad-hoc review`, and structured results carry the same distinction from an assessed empty gap list. An agent can request a separate rubric audit when it needs coverage evidence.

Review coverage uses the existing changeset file list unchanged, without an additional existence filter. Working-tree reviews include untracked additions and deleted paths; path and full reviews retain their tracked-file scope. A deleted path's coverage describes scrutiny of its removal. The standalone audit uses the same full tracked scope as a full review, so untracked additions enter the audit only after being added to Git. Coverage does not introduce its own interpretation of staged or unstaged renames and deletions: it checks the paths returned by the existing scope extraction. The work order records planning verification of those extraction cases and the integration tests that must preserve them.

## Audit through MCP and CLI

The MCP tool is named `audit_coverage` and takes required `repo_path`, an optional `rubric` defaulting to `rubric`, and `include_map` defaulting to false. The CLI counterpart is `terminus audit-coverage --repo <path> --rubric <name> [--include-map]`. Each request audits one rubric. The work order pins the exact response schema to the structure below.

The MCP tool description must advertise the optional map explicitly, including how to request it, its default, and why an agent would use it. The `include_map` input field must also carry a description in the exposed input schema. Map discovery must not depend on reading project documentation or first running an audit. Both descriptions explain that the map shows matching local qualities per file, including files that already have coverage, so it helps investigate differences in reach beyond zero-coverage gaps.

Suggested tool-description wording:

> audit one project rubric against its full tracked tree. repo_path is required; rubric defaults to `rubric`. returns uncovered files after coverage exclusions and territory patterns that match no tracked file. set include_map to true (default false) to also return a coverage map showing which project-local qualities reach each file, grouped by directory while preserving file-level differences. use the map to inspect uneven coverage even where files already match a quality. read-only: runs no reviewer, writes no review record, and makes no canon edits. use the evidence to propose canon changes, then rerun the audit to check their effect.

The tool resolves the project's configured canon and requested rubric using the same identity and validation rules as review. It returns its result directly; no start/monitor/collect cycle or review identifier is needed. An agent auditing several projects invokes it for each project, keeping canon tuning tied to the project it is working on.

The audit response identifies the project, rubric, and full tracked scope; contains the shared `coverage` object; reports dead patterns with their quality IDs and canon refs; and includes the directory-grouped map when requested. Review and audit use the same coverage-object model for their respective file sets.

Result contents:

| Content | Purpose |
| --- | --- |
| Project, rubric, and file scope | Identify what was checked; the audit scope is the full tracked tree. |
| Local quality identities and refs | Make the local tier inspectable and the canon files actionable. |
| Coverage exclusions and suppressed files | Distinguish intentional exceptions from actual territory matches. |
| Uncovered files | Preserve exact repository-relative paths even when presentation groups them. |
| Dead patterns with quality identities and refs | Point to the specific territory declaration to inspect. |
| Optional coverage map | Map each file to its matching local qualities and exclusions. |

The optional map groups files by their immediate parent directory, preserving each file's exact matching qualities. A directory summary may list the union of matching qualities for orientation, but cannot stand in for those file-level relationships. For example, if one file matches two qualities and its neighbor matches only one, an agent must be able to identify which file lacks the second match. Excluded files remain visible in the map with their exclusion attribution.

The map helps investigate low coverage without pretending to infer missing invariants. If `internal/config/` matches `config-validation` and `no-ambient-authority`, while `cmd/sterling/` matches only `cli-conventions`, the agent has a reason to inspect whether the command should also satisfy `no-ambient-authority`. The tool does not declare that omission a defect merely from the differing lists.

Gaps and dead patterns are successful diagnostic results, not operational failures: the MCP response is a successful tool result and the CLI exits successfully. Invalid configuration, unreadable canon, and inability to enumerate the tracked tree produce ordinary MCP tool errors or a failing CLI exit, never an empty report that looks like success.

Use stable ordering for paths, patterns, and refs so successive audits are easy to compare. Exact output member names and serialization details belong in the work order.

## Scenarios the design must preserve

- A new command tree is reported without adding a coverage declaration. This is the blind spot an opt-in `internal/**` declaration would leave open.
- A `docs/**` exclusion suppresses uncovered documentation paths while general or local qualities that apply to them still run normally.
- A mixed change touching a covered package and an uncovered package reports the latter even though the review selected local qualities for the former.
- A package rename leaves a territory pattern matching no tracked files. The audit reports the stale pattern and, unless excluded or otherwise reached, the new paths as uncovered.
- A pattern matching only an intentionally excluded tree is still live.
- Two files in one directory with different matching qualities remain distinguishable in the detailed map.
- A canon edit between review dispatch and collection does not rewrite the earlier review's coverage; a fresh audit reflects the edit.

## Design decisions

The following decisions and the boundaries in the seam census are settled. The work order translates them into the concrete schema and implementation plan without changing their behavior.

1. **Project-local identity — settled.** The definition is recorded under One coverage model.
2. **Qualities without territory — settled.** The coverage rule and its tradeoff are recorded under One coverage model.
3. **No local tier and ad-hoc reviews — settled.** The no-local-tier behavior is recorded under One coverage model; ad-hoc reporting is recorded under Review-time reporting.
4. **Public interfaces and response structure — settled.** The audit tool/verb, inputs, one-rubric request, response structure, map-discovery requirements, map grouping, and operational-error distinction are recorded under Audit through MCP and CLI. Running-status visibility, the coverage object and its assessment distinctions, and historical-result handling are recorded under Review-time reporting. The work order specifies exact member names and serialization details.
5. **File-set semantics — settled.** The scope rule and extraction verification needed during planning are recorded under Review-time reporting.

## Seam census

| Boundary | Call and rationale |
| --- | --- |
| Coverage report / review execution | Settled: separate. Coverage exposes reach; it neither widens selection nor changes the verdict. Revisit automatic widening only as a deliberate later feature. |
| Audit evidence / canon mutation | Settled: separate. The tool reads and reports; the agent and operator own reviewable canon edits. |
| Coverage computation / presentation | Settled: one shared calculation feeding structured MCP data and CLI rendering. Both consumers must agree on matches; directory formatting must not define coverage. |
| Model / transport | Settled: keep MCP schema and serialization concerns at the transport boundary; the calculation expresses file-to-quality relationships. |
| Operational errors / diagnostic results | Settled: invalid input and failed reads use the service's error path; coverage gaps and dead patterns remain successful reports. |

## Deferred (and Why)

Automatic widening, coverage-based blocking, and automatic canon repair are deferred because reach alone does not tell Terminus which rule is missing or appropriate. Coverage scores and automatic low-coverage thresholds are deferred because quality counts are not a measure of invariant completeness.

The canon's [command-tree audit card](../../../terminus-canon/docs/future/roadmap/territory-audit-command-trees.md) remains separate work: survey projects, judge real omissions, and potentially add an authoring convention about invocation surfaces. This tool supplies evidence, not the judgment or the canon changes. That work need not wait for the tool.

The separate [changeset-exclusion proposal](terminus-changeset-exclusions.md) concerns which files participate in review. Coverage exclusions only suppress gap complaints and must not acquire that proposal's execution semantics.

Multi-pass aggregation remains with the existing [multiple-review-passes card](roadmap/multiple-review-passes.md). This spec describes one review and one rubric per audit request; a later cycle model must deliberately define file-level coverage rather than infer it from the union of selected qualities.
