# terminus ask: project-scoped changeset exclusions

archive needs a way to keep its frozen `v2/` reference tree out of review, structurally. this describes the incident that proved prose can't do it, and the mechanism that would.

## the incident

archive carries `v2/` as a frozen legacy reference: consulted for proven patterns, never edited, explicitly outside v3 conventions. every full-changeset review until `d4915acfdffd` (2026-07-31) left it alone. that round went `not_clean` with three findings -- a blocking `df-logging` hit and two `lowercase-output` advisories -- all inside `v2/`, none actionable: the freeze forbids exactly the edits the findings suggest.

## the diagnosis

- the only "do not flag `v2/`" language the reviewer ever saw came from one project-local quality body (`projects/archive/id-not-id.md`), because a project-local body may name project directories. the general-tier qualities that fired (`df-logging`, `lowercase-output`, `mermaid-diagrams`) carry no such carve-out and cannot: general bodies must stay project-agnostic.
- rubric `characteristics` do not ride into the review prompt. a characteristics line stating the freeze ("v2/ is outside review territory -- never flag its contents") was added before the failing round and never appeared in `_prompt.md`; it is inert.
- therefore every prior round's restraint was model judgment -- the reviewer extrapolating one quality's carve-out to the whole tree. that judgment is nondeterministic, and it lapsed.

## the ask

a project-scoped path exclusion applied at changeset assembly, so excluded trees never enter the manifest at all -- for every changeset kind (`full`, `working-tree`, `paths`). something like:

```yaml
# projects/archive/rubric.yaml
project:
  name: archive
  excluded_paths:
    - v2/**
```

removing the files from the changeset beats instructing the reviewer to ignore them: the reviewer cannot flag what it never sees, the exclusion is deterministic, and general-tier quality bodies stay project-agnostic.

## alternatives considered

- **characteristics into the prompt**: would make the existing prose line live, but it remains an instruction to restrain rather than a boundary -- still subject to reviewer judgment, still nondeterministic.
- **territory negation on quality frontmatter** (`!v2/**`): granular, but every general quality would need the negation per project, which puts project knowledge back into general bodies or demands per-project territory overrides in the rubric -- more machinery for the same outcome.

## acceptance shape

- a full review of archive with the exclusion configured lists no `v2/` files in the changeset manifest and can never emit a finding with a `v2/` path.
- the exclusion is visible in the review record (the manifest or prompt noting what was excluded and why), so a clean verdict is honest about its scope.
