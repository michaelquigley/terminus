# Territory coverage work order

Grounded against commit `d2f4925`, 2026-09-08. Implements [territory coverage](territory-coverage.md). The spec owns behavior and rationale; this document owns code placement, data contracts, stages, and validation. Mercurius reviewed the pair with a `ready_to_build` verdict; all three advisory notes were resolved with Michael's agreement. Implementation has not started.

## Grounding in the current tree

| Existing surface | Integration point |
| --- | --- |
| `internal/canon/rubric.go` | Strict rubric parsing and project identity; add the optional exclusion list here. |
| `internal/canon/select.go` | Shared territory semantics, validation, normalization, composition, and narrowing. Reuse these for coverage. |
| `internal/changeset/changeset.go` | Working-tree files from harness Git status; tracked scopes from harness `scope.Resolve`. |
| `internal/broker/review.go` | `prepareReview` composes and narrows before dispatch; jobs retain selected/excluded qualities; `status`, `execute`, `writeResult`, `collectFromStored`, and cloning propagate data. |
| `internal/broker/types.go` | Existing service and result-file DTOs. |
| `internal/monitor/monitor.go` | Status DTO and `dd` persistence. Monitor imports must not depend on broker, which already imports monitor. |
| `internal/mcpserver/mcpserver.go` | Typed MCP registration, inferred schemas, and existing `toolErrorResult` mapping. |
| `cmd/terminus/review.go`, `cmd/terminus/main.go` | Foreground output, command registration, server startup, and monitoring. |
| `internal/config`, `internal/wiring` | Config loading is separate from log-directory creation; ordinary broker wiring constructs a reviewer. |

No dependency upgrade is planned. Use the pinned harness v0.1.1, df v1.0.3, and MCP SDK v1.5.0. The SDK reads field descriptions from `jsonschema` struct tags. Existing application JSON/YAML persistence uses `df/dd`; existing MCP transport marshaling is owned by the SDK.

## Shared calculation and adapters

Add `internal/coverage` for the calculation and plain domain types. It imports `canon` for composed qualities and the canonical matcher; it contains no JSON, YAML, MCP, persistence, or CLI formatting code. Its entry point accepts context, resolved project identity, the complete composed rubric, normalized input files, coverage exclusions, and options requesting dead-pattern analysis and/or the map. It returns an assessment plus requested audit detail. It performs no filesystem access. Check cancellation during potentially large file/pattern loops.

Expose the existing canonical file normalization and territory match functions from `internal/canon/select.go`, retaining their existing semantics and using them from both `Narrow` and coverage. Do not copy the matcher into the new package. Coverage works on the composed set before narrowing; a local quality selected for one file must not accidentally account for all other files.

For each normalized, unique input path, compute matching local quality refs and matching exclusion patterns independently. Sort and deduplicate returned paths, quality identities, and exclusion patterns. Local identity is the exact `projects/<project>/` prefix, including the trailing slash, on already-clean canon refs. A territory-free local quality matches all inputs. General and foreign-project qualities do not supply local matches, but their explicit territory patterns participate in audit dead-pattern detection.

The file classification used for counts is disjoint: exclusion match first, then local match, then uncovered. This count classification does not remove actual quality matches from the map. Dead patterns are checked against all input files only when the caller requests full-tree audit detail; review-time evaluation must never call a pattern dead merely because it misses the changeset.

Add `internal/report/coverage.go` for shared transport/persistence DTOs and conversion from the domain result. Both broker and monitor can import this package without an import cycle. One conversion defines the public coverage shape used by status, results, collect, and audit. CLI renderers consume this data; they may group or format it but never recalculate coverage. Keep schema tags in this adapter package, not in `internal/coverage`.

## Rubric parsing and matcher validation

Add `CoverageExclusions []string` to `canon.Rubric`, using the default snake-case `dd` key `coverage_exclusions`. Absent and empty lists mean no exceptions. Preserve strict rejection of unknown rubric fields; `coverage` remains unsupported. Validate each exclusion using the same validator as quality territories and report the rubric field index and offending pattern on failure. Preserve the declared pattern spelling in output so an agent can find the line to edit; normalize for matching only. Identical declared strings may be deduplicated in diagnostic results without rewriting the rubric.

`ValidateTerritory` currently validates by matching one dummy path. A mismatch in an early segment can hide invalid syntax in a later segment. Make validation visit every normalized segment: `**` is legal only as an entire segment; other segments must pass `path.Match` syntax checking, including later segments such as `internal/[`. Use this validator for both quality territories and exclusions. This is needed for reliable audit errors and does not change the accepted glob language. Do not add a second glob grammar or reinterpret leading/trailing-slash normalization in this work.

Validate before returning an assessment, even for an empty file set. New coverage code must propagate a matcher error rather than treating it as absence of a match. Existing composition and parsing should make invalid user patterns fail before review dispatch or audit evaluation.

## Wire contract

Use these member names on MCP and disk. New output collections are arrays, including empty arrays, never `null`. Optional fields are omitted as specified. Paths are normalized repository-relative slash paths. Quality refs use the existing extensionless, canon-relative `canon.Quality.Ref` representation, with IDs preserved from the quality header. Carry those refs unchanged into coverage, map, and dead-pattern output; do not append `.md` or change existing selected/excluded-quality fields.

### Coverage object

Every new review status, completed result, collect response, and audit result carries `coverage`. Historical status/result DTOs use a pointer with `json:"coverage,omitempty"` so absence survives reading and re-emission. Never fill missing historical coverage from the current canon. New ad-hoc reviews have an explicit object, not a nil pointer.

| Member | Type and semantics |
| --- | --- |
| `assessed` | Required boolean. True for a rubric assessment, including empty input and no-local-tier cases. |
| `reason` | String, omitted when assessed; `ad_hoc` when not assessed. |
| `file_counts` | Object, present only when assessed. Members: `total`, `excluded`, `covered`, `uncovered`, all nonnegative integers. |
| `local_qualities` | Array of `{id, ref}` for all local qualities in the composed rubric, sorted by ref. Empty for no local tier and for not-assessed output; inspect `assessed` before interpreting it. |
| `uncovered_files` | Array of all reportable uncovered paths, sorted. Empty for not-assessed output. |
| `exclusions` | Array of `{pattern, files}` for the rubric's distinct declared exclusion strings, sorted by pattern. Each `files` array contains all matching input paths, including files also reached by local qualities. Keep patterns with no matches with `files: []`. Empty for not-assessed output. |

For assessed output, `total = excluded + covered + uncovered`. `excluded` counts unique files matching any exclusion; overlapping exclusions count a file only once. `covered` counts matching files outside exclusions. `uncovered` equals the length of `uncovered_files`. The map retains actual quality matches even for files classified as excluded. These are file counts, not a coverage score.

An empty scope has all counts zero. All-files-excluded requires `total > 0` and `excluded == total`. An assessed gap-free scope has `uncovered == 0`, but consumers must retain the exclusion and empty-scope distinctions. A missing historical `coverage` object means unavailable, distinct from explicit `assessed: false` with `reason: ad_hoc`.

Example for three input files, one covered, one intentionally excluded, one uncovered:

```json
{
  "assessed": true,
  "file_counts": {"total": 3, "excluded": 1, "covered": 1, "uncovered": 1},
  "local_qualities": [{"id": "config-validation", "ref": "projects/example/config-validation"}],
  "uncovered_files": ["internal/gateway/handler.go"],
  "exclusions": [{"pattern": "docs/**", "files": ["docs/guide.md"]}]
}
```

Use a pointer for `file_counts`, omitted for not-assessed output; initialize all array fields to empty slices during conversion. Pair JSON omission tags with the corresponding `dd:",+omitempty"` flag where needed, such as `reason`; do not assume `dd` honors JSON tags. df v1.0.3 omits nil pointers on unbind. Retain the existing raw-reviewer-JSON converter in broker persistence. Test the actual `dd` and MCP serialization paths, including nil historical coverage and explicit false assessment, rather than relying on Go struct equality alone.

### Audit response

The MCP handler returns this object directly, with no review wrapper or session identifier:

| Member | Type and semantics |
| --- | --- |
| `project` | Resolved repo-basename project identity. |
| `rubric` | Requested rubric name, defaulting to `rubric`; normalize an accepted `.yaml` suffix consistently in the shared resolver. |
| `file_scope` | Literal `full`, identifying the existing full tracked scope. |
| `coverage` | The assessed coverage object above. |
| `dead_patterns` | Array of `{quality: {id, ref}, pattern}`, ordered by ref then declared pattern string. Include all rubric quality tiers; collapse duplicate identical ref/pattern pairs. |
| `coverage_map` | Optional array of directory groups, present only when `include_map` is true. A requested empty map is `[]`, not omitted. |

Each map group is `{directory, files}`. `directory` is the immediate parent, with `.` for repo-root files. Each file entry is `{file, qualities, exclusion_patterns}`: exact repo-relative path, an array of matching local `{id, ref}` objects, and an array of matching declared exclusions. Sort groups by directory, files by path, qualities by ref, and patterns lexically. Include every input file, whether covered, uncovered, or excluded. Do not emit a directory union in place of file detail. The first version needs no redundant directory-level quality summary.

Use a pointer to an initialized slice, or an equivalent explicit presence representation in the transport adapter, to preserve requested-empty versus omitted `coverage_map` through both JSON and `dd`. Map detail is optional for payload size; it does not affect counts, uncovered files, or dead-pattern results. Do not truncate lists silently or require agents to parse human-readable text to recover paths.

## Review integration

In `prepareReview`, after project/rubric and changeset resolution and successful `Compose`, calculate the assessment from the full composed set and `cs.Files`, before allocating artifacts or starting the reviewer. In the explicit-quality branch, create the not-assessed report without invoking rubric resolution or the calculator. Preserve `Narrow`, explicit-quality territory bypass, selected/excluded quality reporting, and prompt assembly behavior.

Store an owned coverage snapshot on `reviewJob`. Thread it through `job.status` for running, completed, and failed states; `execute`'s result construction; `writeResult`; `collectFromStored`; and `cloneCollectReviewResponse`. New status writes expose it from the first running status. Deep-copy every nested list when crossing mutable result boundaries, including exclusion file lists and local-quality lists, so modifying one collected response cannot alter the job or a later collect.

Extend the status, result-file, and collect DTOs with the shared pointer type. Disk collection must preserve missing historical data. Both live collection and restarted-broker collection use one guidance builder that combines existing findings guidance with coverage guidance. Gap guidance names the gap count, says `clean` concerns findings, and points to `audit_coverage` and its optional map for investigation. Not-assessed and unavailable states must be described honestly. Do not change triage order, finding dispositions, verdict calculation, or reviewer attribution.

In `printReviewResult`, render a distinct coverage line adjacent to the verdict even when there are no findings or no excluded qualities. Place the common summary formatting in `cmd/terminus/coverage.go` for reuse by the audit command. Render missing historical data as `coverage: unavailable`, ad-hoc as specified in the spec, empty scope as `coverage: no starting-point files`, all excluded as `coverage: all N files excluded from gap reporting`, ordinary gaps with their paths, and gap-free assessed input as `coverage: no uncovered files (N excluded from gap reporting)` when exclusions are nonzero. For no local tier, prefer the compact notice with the uncovered count; also identify empty/all-excluded scope when applicable so zero uncovered is not mistaken for local scrutiny.

The existing monitor CLI remains a lean status view; this stage adds coverage to `status.json`, not a new detailed monitor presentation. `_findings.md` and `_prompt.md` continue to describe the reviewer input and findings; coverage lives in status/result artifacts and collect output, as agreed.

## Audit service, MCP, and CLI

Add `Broker.AuditCoverage` in `internal/broker/audit.go`. It checks context and required repo path, resolves an absolute path, opens canon, loads the selected project rubric with `LoadProjectRubric`, composes it, obtains files through `changeset.Full`, invokes the shared calculation with dead-pattern analysis enabled, and converts the result once. It does not call `prepareReview`, check for a reviewer, create a job, touch `b.jobs`, allocate a review ID, or access the review log destination.

Extract common rubric request resolution into a broker helper used by both rubric review and audit. Preserve current basename project identity, empty-name default, optional `.yaml` suffix, containment checks, and `project.repo` validation. Keep the ad-hoc branch outside this resolver. The helper reports normalized rubric identity so review and audit do not disagree about `architecture` versus `architecture.yaml`.

Use existing `errs` categories and MCP error conversion. Invalid requests, bad rubric/quality/exclusion data, unreadable canon, and Git enumeration failures are `user_error` with useful path/rubric/pattern context. Unexpected internal failures retain `internal_error`; do not turn them into empty success data. Cancellation follows the existing service convention and stops evaluation promptly.

Register `audit_coverage` on the existing broker-backed MCP server. Input struct: `repo_path` required, `rubric` optional, `include_map` optional boolean. Use `jsonschema` field descriptions, particularly for `include_map`; describe defaults and map use in the tool description. Update `collect_review` and `start_review` descriptions to explain coverage reporting and direct agents to the audit. Set the audit tool annotations to `readOnlyHint: true` and `destructiveHint: false`; the pinned SDK supports both. Set `DestructiveHint` to a non-nil pointer to false so the explicit value is included in discovery. Exercise `tools/list`, not just a direct Go handler call, to verify discovery exposes the map option.

Assign the audit success schema explicitly to `mcp.Tool.OutputSchema`, deriving it from the audit response DTO with the pinned `jsonschema` package so the schema follows the wire types. Keep the handler's output type as `any`: return the concrete audit response on success, and return the existing `toolErrorResult` with a nil output on failure. In SDK v1.5.0, a concrete handler output type can cause a zero-value success object to overwrite handler-supplied structured error content. An `any` output alone avoids that overwrite but does not infer an output schema; the explicit schema supplies discovery and success validation while the nil error-path output preserves `{error: ...}`. Handle schema-construction failures during server setup.

Add `cmd/terminus/audit.go` and register it in `newRootCommand`. Accept no positional arguments. Defaults are `--repo .`, `--rubric rubric`, `--include-map=false`, with existing global config and logging flags. Load configuration through the normal cascade, then construct a broker with `CanonPath` only for this command. Do not call `EnsureLogDestination` or `wiring.NewBroker`; the audit needs neither log storage nor a constructed reviewer. Reuse normal config validation rather than introducing a second configuration dialect. An already-running MCP server keeps its existing startup lifecycle; invoking its audit tool adds no reviewer execution or review-log activity.

CLI output identifies project, rubric, and full scope; renders the coverage summary; groups uncovered paths by immediate parent directory; lists dead patterns with quality refs; and renders the optional map with per-file quality refs and exclusion markers. Grouping may use a directory heading followed by filenames, but must not lose exact paths. No-local-tier summaries remain compact unless a map is explicitly requested. A successful audit, even with gaps or dead patterns, returns nil from the command; ordinary errors reach the root's existing failure handling.

## Git scope verification

The pinned harness's `repo.Status` reads `git status --porcelain -b -uall`, maps a detected rename to its destination path, and exposes modified/added/deleted/untracked lists. `scope.Resolve(full)` reads `git ls-files`, not an on-disk existence walk. Therefore an unstaged deletion remains in the audit's tracked set until staged; an unstaged rename appears as old tracked path in full scope, while working-tree scope sees the old deletion and new untracked path. A staged rename is represented by its destination in both scopes. A staged deletion is in working-tree scope but absent from full scope. No coverage-specific filesystem filter should change these facts.

Planning verified these through disposable Git fixtures against the pinned dependency:

| Fixture change | Working-tree paths | Full tracked paths |
| --- | --- | --- |
| Untracked `new.go`, tracked `old.go` | `new.go` | `old.go` |
| Staged addition of `new.go` | `new.go` | `new.go`, `old.go` |
| Unstaged deletion of `old.go` | `old.go` | `old.go` |
| Staged deletion of `old.go` | `old.go` | Empty |
| Unstaged rename `old.go` to `new.go` | `new.go`, `old.go` | `old.go` |
| Staged rename `old.go` to `new.go` | `new.go` | `new.go` |

Carry the meaningful cases into integration tests. Existing `internal/changeset` tests cover modified/deleted/untracked and basic tracked path/full scopes, but not the complete staging matrix. This work preserves extraction behavior, including existing unusual-filename limitations; it does not replace the harness Git parser. Targeted existing tests for changeset, broker, and MCP passed during planning. The host enables signed Git commits globally, so these disposable fixtures needed a process-scoped `commit.gpgsign=false` override; do not change the operator's Git configuration to run tests.

## Stages

### stage 1 — shared assessment and wire model

Implement rubric exclusions and complete pattern validation, expose shared matcher/normalizer functions, add `internal/coverage` and `internal/report`, and pin domain-to-wire conversion. Keep the rest of the runtime compiling without requiring callers to use coverage yet.

Validate exact project-prefix membership and foreign/general exclusion, territory-free qualities, mixed covered/uncovered inputs, exclusions overlapping coverage and one another, empty and all-excluded scopes, no-local-tier scopes, malformed late pattern segments, and dead patterns across all rubric tiers including excluded-tree matches. Verify map false/true yields identical assessment and dead patterns, and map grouping preserves different matches within a directory. Test actual JSON/`dd` behavior for empty arrays, optional counts/map, and historical nil coverage. Run targeted package tests followed by `make test`, then Terminus review to `clean` before the operator's stage acceptance.

### stage 2 — review reporting and persistence

Integrate the shared assessment into dispatch, every status/result/collect path, guidance, and foreground CLI output. Update current behavior documentation and the `CHANGELOG.md` Unreleased entry for the behavior actually delivered by this stage.

Use a controllable reviewer that blocks until released: inspect the first running status, edit the canon, then let the reviewer finish and compare the original coverage across running/completed status, persisted result, in-memory collect, and collect through a new broker. Mutate returned nested coverage data and prove a later collect is unchanged. Exercise a failing reviewer and verify failed status retains coverage. For an ad-hoc review, omit the project rubric entirely and verify it still runs with `assessed: false`. Use a mixed covered/uncovered fixture to prove prompt quality selection and finding verdicts remain unchanged with reporting and exclusions. Load a pre-feature result fixture and verify missing coverage stays unavailable. Add a real MCP review/collect test and CLI checks for the distinct states. Run `make test`, targeted race checks for broker result ownership if new shared mutable state is involved, and Terminus review.

### stage 3 — agent audit and CLI

Add the audit service and shared resolver, MCP input/output schemas and discovery descriptions, CLI command/rendering, and final documentation. Keep every path on the shared calculator and report conversion. Finish the concrete Git staging matrix verification described above.

Use the same project/rubric fixture through direct service, MCP, and CLI. Compare full-review coverage with audit coverage over an unchanged fixture, and repeat the audit after a canon edit to verify fresh loading. Assert structured success for gaps/dead patterns and structured errors for malformed rubric, wrong project identity, malformed exclusion, and non-repository input. Assert `tools/list` exposes `audit_coverage`, the required repo path, optional map field, its explanatory descriptions, and the explicit annotations `readOnlyHint: true` and `destructiveHint: false`. Also assert that the advertised `outputSchema` describes the audit success object, including coverage, dead patterns, and the optional map, and validate a successful response against it. Through the real MCP transport, assert an audit failure has `IsError: true` and the exact `{error: {code, message, details}}` structured payload expected from the service fixture, with no replacement success-shaped fields. Check omitted-map versus requested-empty-map output through that same transport.

Prove the read-only contract with a reviewer stub that fails the test if called and a log destination that cannot be used as a directory, then compare project/canon file contents and broker job state before and after audit. CLI integration should point a valid configured reviewer at a nonexistent executable and the log destination at an unusable path; the audit must still succeed without executing the binary or touching that destination. This detects accidental use of review startup. Scope fixtures should include a staged add, untracked add, both forms of deletion and rename, and per-file map differences within one directory. Run `make test` and Terminus review of the final stage.

## Documentation and completion

Update `docs/current/overview.md` and relevant README usage as each surface lands: rubric field semantics, assessed/unavailable/ad-hoc distinctions, default reporting compatibility, the audit tool and flag defaults, exact response examples, and map discovery. Document that an unstaged deletion can keep a pattern live in a tracked-tree audit, that a territory-free local quality covers all inputs, and that a map is evidence for invariant review rather than a score.

Old binaries reject the new rubric key under strict parsing. Ship parser support before adding `coverage_exclusions` to real canon rubrics; no canon edits are part of this work order. Preserve older result readability and all existing review verdict semantics. No backfill is required. Keep the existing config cascade and dependency versions unless verification proves a concrete obstacle.

At final handoff, inspect relevant Terminus canon qualities against the implementation and report drift rather than editing canon. Synthesize built behavior into current docs before retiring this spec/work-order pair; preserve their deferred concerns and resolve roadmap links under the operator's normal close-out convention. Leave roadmap prioritization and commits to the operator.
