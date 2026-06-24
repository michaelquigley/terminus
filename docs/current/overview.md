# Terminus Current Behavior

Terminus is a local MCP code-review broker. A caller points it at a project repo and one finite review target — the dirty working tree, a set of paths, or the full tracked repo — and Terminus reviews that target against a central canon of qualities. The verdict is scoped to that target: `clean` means the reviewer returned no blocking findings. Advisory findings may still be present.

v1 is intentionally the review spine only. It has no sessions, no rounds, no disposition capture, no survey, no sharpening, no canon promotion, and no autofix loop. Iteration is external: fix or reject what the review surfaced, then start a new review.

## Repos And Data

The project repo under review supplies code only. The review criteria live in a separate canon repo, configured by `canon_path`, with project rubrics at `projects/<project>/rubric.yaml`. Project identity is the basename of `repo_path`, and the loaded rubric must have `project.repo` equal to that basename.

Review records are written outside the subject repo under `log_destination/<project>/<review_id>/`. The default log destination is `~/.local/share/terminus`. `canon_path` is required; `terminus.yaml.example` shows a minimal local configuration. A review directory contains:

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
convention: software/conventions/changelog-convention
---
```

The parser accepts only `id`, `applies_to`, `territory`, and `convention`. Unknown keys fail loudly. The body is passed to the reviewer verbatim and is expected to be self-contained; `convention` is provenance only in v1.

A rubric lists quality refs and whether each one blocks:

```yaml
project:
  repo: terminus

qualities:
  - ref: go-conventions/df-logging
    blocking: true
```

Refs are canon-relative, must not be absolute, and must not escape with `..`. Duplicate quality ids in a composed rubric fail before review.

## Selection

Selection has two stages. `Compose` loads every quality listed in the project rubric. `Narrow` intersects that set with the files under review using each quality's `territory`. Territory patterns are slash-path globs with recursive `**`; `**/*.go` matches both `main.go` and `internal/x.go`, and a trailing slash such as `cmd/` means `cmd/**`. A quality with no territory always applies.

## Changesets

`working-tree` uses the harness `repo` package to read git status and `git diff HEAD`. The file set includes modified, added, deleted, and untracked files. Untracked files are in the review scope but not in the diff.

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

There is no reviewer severity field. After schema validation, Terminus rejects duplicate finding ids, unknown quality ids, and findings on files outside the review scope. Blocking is computed only from the selected qualities captured at dispatch. Findings are returned blocking-first, then advisory, preserving reviewer order within each group.

## CLI Surface

Bare `terminus` prints command help and exits successfully. `terminus serve` is the explicit stdio MCP server entry point.

`terminus version` prints shared `push/build` metadata. Unstamped local builds report `v0.1.x [developer build]`; CI builds stamp version, commit, build time, branch, builder, Go runtime, and target through the `push` `ci/ldflags.sh` script.

`terminus review` runs a review in the foreground. It accepts the same review target as `start_review`:

```bash
terminus review
terminus review --repo ../terminus --kind full
terminus review --kind paths internal/canon cmd/terminus
```

`--repo` defaults to `.`, `--kind` defaults to `working-tree`, and path arguments are only valid with `--kind paths`. The command writes the same review artifacts as MCP review startup, waits for the reviewer to finish, then prints the verdict, finding counts, prompt path, findings log path, and a compact finding list. A `not_clean` result is still a successful command execution; process failure is reserved for operational errors.

When `--kind` is left at its default and the working tree is clean, the command promotes the review to `full` and prints `working tree clean; reviewing full tracked repo`. This keeps a bare `terminus review` on a committed repo from selecting nothing and reporting a vacuous `clean`. An explicit `--kind working-tree` is always honored, even on a clean tree. The promotion is CLI-only; the MCP `start_review` tool reviews exactly the `changeset_kind` it is given.

## MCP Surface

`start_review` takes `repo_path`, `changeset_kind` (`working-tree`, `paths`, or `full`), and optional `paths`. It resolves the project rubric, narrows the qualities, writes `_prompt.md`, starts the reviewer in the background, and returns a `review_id` plus a monitor command.

`collect_review` returns a completed review when given `review_id`; if omitted, it lists known review runs. A running review returns `conflict`.

The CLI also exposes `terminus monitor --project <project> --wait <review_id>` for polling `status.json`.
