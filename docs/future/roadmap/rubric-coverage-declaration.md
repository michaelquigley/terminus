---
title: rubric coverage declaration
state: inbox
created: 2026-09-08
tags: [feature]
---

Let a rubric declare the tree its project-local qualities are expected to reach — a `coverage:` list of territory globs beside `qualities:` — and have every review report the starting-point files inside that tree that no project-local quality's territory matches. The report is a first-class field beside the verdict in `result.json`, the CLI output, and the MCP `collect_review` response, never only a line in the quality list. An absent `coverage:` means today's behavior.

## why

Territory narrowing is a precision device: it keeps a reviewer from being handed sixteen qualities when three apply. Its failure mode is that it silently drops scrutiny. A project-local quality names the packages its invariant lives in, so a new package matches nothing, the review runs on the general conventions alone, and the verdict says `clean` with a straight face. An external project hit this twice — `internal/config` and `cmd/` on 2026-08-07, `internal/runrecord` on 2026-09-01 — and `internal/gateway` is open now. Each was closed by a one-line territory patch, and the card that raised the third asked for the general answer instead.

The root is a cross-repo coupling: adding a package to a project implies an edit in the canon, and neither repo knows about the other. Terminus is the one process that holds both trees at once, so it is where the coupling can be checked.

The selected and excluded qualities are already in the result, and a standing instruction to read them rather than the verdict has been in force since August. It still took a journal note to catch the second instance. A field nobody is made to read is not a fix; a report that names the files, on the surface the verdict is read from, is.

## background

**Declared, not derived.** Nothing in the external project's existing patterns can tell terminus that `internal/gateway` should have been reached, and a heuristic over sibling directories would fire on `docs/` and `demo/` every review. The rubric author says which tree the local tier is responsible for; terminus checks it. That is also the deliberate decision `terminus-canon`'s `territory-audit-command-trees` card asks for: declaring `cmd/**` answers whether a command tree is meant to be covered.

**What a design pass settles.** Whether an uncovered file should also *widen* the review — select the whole project-local tier so the code is actually reviewed, with the report saying why — or only be reported; widening is the safe direction and costs a broad prompt on the rare review that adds a package. Whether the report touches the verdict at all, or stays a distinct field with the CLI line and the MCP guidance made to say it; the verdict is computed from findings and coverage is a property of the review's reach, which argues for distinct. Whether `coverage:` is per rubric, since narrowing is per rubric and a `code-issues` lens may legitimately cover less, or once per project. What "project-local" means to the check — a ref under `projects/<project>/` is the natural tier boundary and is what `terminus rubrics` already derives.

**Related.** `territory-coverage-audit` is the same check over the full tracked tree, run by hand rather than per review.
