# Terminus Current Behavior

Terminus is a local MCP code-review broker. A caller points it at a project repo and a starting point — the dirty working tree, a set of paths, or the full tracked repo — and Terminus reviews the code against a central canon of qualities. The starting point is where the reviewer begins, not a fence: a finding may land on any file a change reaches. `clean` means the reviewer returned no blocking findings; advisory findings may still be present.

v1 is intentionally the review spine only. It has no sessions, no rounds, no disposition capture, no survey, no sharpening, no canon promotion, and no autofix loop. Iteration is external: fix or reject what the review surfaced, then start a new review.

## Repos And Data

The project repo under review supplies code only. The review criteria live in a separate canon repo, configured by `canon_path`, with project rubrics at `projects/<project>/rubric.yaml`. Project identity is the basename of `repo_path`, and the loaded rubric must have `project.repo` equal to that basename.

Review records are written outside the subject repo under `log_destination/<project>/<review_id>/`. The default log destination is `~/.local/share/terminus`. `canon_path` is required; `examples/terminus.yaml.example` shows a minimal local configuration. A review directory contains:

- `status.json` for monitor/collection state.
- `_prompt.md`, the exact prompt sent to the reviewer.
- `_findings.md`, the immutable durable record written through `theharnessbody/record`.
- `result.json`, the structured collection payload.

## Canon

A quality is a markdown file with strict YAML frontmatter:

```yaml
---
id: df-logging
applies_to:
  - go projects
territory:
  - "**/*.go"
---
```

The parser accepts only `id`, `applies_to`, and `territory`. Unknown keys fail loudly. The body is passed to the reviewer verbatim and is the self-contained, sole authority for the rule; the canon carries no back-reference to the grimoire. Quality bodies follow a section template (`statement`/`why`/`discrimination`/`example`/`boundary`); the review prompt directs the reviewer to each quality's `discrimination` and `boundary` as its flag/skip calibration, falling back to `statement` and `why` when those are absent.

A rubric lists quality refs and whether each one blocks:

```yaml
project:
  repo: terminus

qualities:
  - ref: go-conventions/df-logging
    blocking: true
```

Refs are canon-relative, must not be absolute, and must not escape with `..`. Duplicate quality ids in a composed rubric fail before review.

A project may carry multiple named rubrics, each a different subset of the canon. Rubric files live flat at `projects/<project>/<name>.yaml`, and the default is `rubric.yaml`. `terminus review --rubric architecture` (or the MCP `start_review` `rubric` field) selects a named rubric; an empty request resolves to the default. `terminus rubrics` lists the rubrics a project has. Blocking is per-rubric, so the same quality can block under one rubric and be advisory under another. Rubrics are independent, hand-authored lists; there is no include/compose mechanism yet.

## Selection

Selection has two stages. `Compose` loads every quality listed in the project rubric. `Narrow` intersects that set with the starting-point files using each quality's `territory`. Territory patterns are slash-path globs with recursive `**`; `**/*.go` matches both `main.go` and `internal/x.go`, and a trailing slash such as `cmd/` means `cmd/**`. A quality with no territory always applies.

## Changesets

`working-tree` uses the harness `repo` package to read git status and `git diff HEAD`. The file set includes modified, added, deleted, and untracked files. Untracked files are among the starting points but not in the diff.

The changeset files are *starting points*, not a boundary. The reviewer begins there, follows a starting-point change wherever it reaches, and may file findings on any file in the tree. The working-tree diff is the seed for that tracing — what changed in the starting-point files — not the limit of what gets reviewed.

`paths` and `full` use the harness `scope` package. They review tracked files only: explicit paths or the full tracked repo. Terminus passes paths and the working-tree diff, not file bodies. The reviewer runs with `WorkingDir` set to the project tree and reads files directly under the backend's read-only mode.

## Findings And Verdict

The reviewer returns:

```json
{
  "summary": "...",
  "findings": [
    {
      "id": "f1",
      "quality": "df-logging",
      "file": "main.go",
      "lines": "12",
      "claim": "...",
      "rationale": "...",
      "suggestion": null
    }
  ]
}
```

There is no reviewer severity field. After schema validation, Terminus rejects duplicate finding ids and unknown quality ids. It does not constrain which file a finding names — findings are not fenced to the changeset, so a finding may land on any file a starting-point change reaches. Blocking is computed only from the selected qualities captured at dispatch. Findings are returned blocking-first, then advisory, preserving reviewer order within each group.

## CLI Surface

Bare `terminus` prints command help and exits successfully. `terminus serve` is the explicit stdio MCP server entry point.

`terminus version` prints shared `push/build` metadata. Unstamped local builds report `v0.1.x [developer build]`; CI builds stamp version, commit, build time, branch, builder, Go runtime, and target through the `push` `ci/ldflags.sh` script.

`terminus review` runs a review in the foreground. It accepts the same starting point as `start_review`:

```bash
terminus review
terminus review --repo ../terminus --kind full
terminus review --kind paths internal/canon cmd/terminus
```

`--repo` defaults to `.`, `--kind` defaults to `working-tree`, `--rubric` defaults to `rubric` (the project's `rubric.yaml`), and path arguments are only valid with `--kind paths`. The command writes the same review artifacts as MCP review startup, waits for the reviewer to finish, then prints the verdict, finding counts, prompt path, findings log path, and a compact finding list. A `not_clean` result is still a successful command execution; process failure is reserved for operational errors.

`--quality <ref>` (repeatable) runs an ad-hoc review against the given canon quality refs directly, bypassing rubric resolution — useful for trying a quality, or reviewing a project that has no rubric yet. It needs no rubric file and does not check `project.repo`; the project is still the repo basename. Ad-hoc qualities are advisory unless `--blocking` is passed; the recorded rubric is `(ad-hoc)`. `--quality` and `--rubric` cannot be combined.

`terminus rubrics --repo <path>` lists the rubric names available for a project, derived from the `*.yaml` files under the canon's `projects/<project>/` directory; the default rubric is marked.

When `--kind` is left at its default and the working tree is clean, the command promotes the review to `full` and prints `working tree clean; reviewing full tracked repo`. This keeps a bare `terminus review` on a committed repo from selecting nothing and reporting a vacuous `clean`. An explicit `--kind working-tree` is always honored, even on a clean tree. The promotion is CLI-only; the MCP `start_review` tool reviews exactly the `changeset_kind` it is given.

## MCP Surface

`start_review` takes `repo_path`, `changeset_kind` (`working-tree`, `paths`, or `full`), optional `paths`, and an optional `rubric` name (defaulting to `rubric`). It resolves the named project rubric, narrows the qualities, writes `_prompt.md`, starts the reviewer in the background, and returns a `review_id` plus a monitor command. The selected rubric is recorded in `status.json` and `result.json`. An optional `qualities` list reviews against those canon quality refs directly, bypassing the rubric (an ad-hoc review); it takes precedence over `rubric`, and `qualities_blocking` makes them blocking (advisory by default).

`collect_review` returns a completed review when given `review_id`; if omitted, it lists known review runs. A running review returns `conflict`.

The CLI also exposes `terminus monitor --project <project> --wait <review_id>` for polling `status.json`.
