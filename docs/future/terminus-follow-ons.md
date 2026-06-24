# Terminus Follow-ons

The v1 review spine is built. These are the forward-looking pieces deliberately
left out so the broker could run before the canon became large.

## Canon Growth

The `terminus-canon` repo currently carries only the seed qualities needed for
the first walkthrough. Broader adaptation from `otis-bok` remains future work.
Project-local qualities should start under `projects/<project>/`; promotion to
the general tier happens only after the same quality recurs across projects.

## Rubric Composition

Named rubrics ship as independent, hand-authored lists — `--rubric architecture`,
`--rubric code-issues`, each a flat `projects/<project>/<name>.yaml`. Deferred is
any composition mechanism: a rubric that `includes` a base rubric, or tag/group
selection where qualities are pulled by a label rather than enumerated. Build it
only when real duplication across rubrics makes re-listing painful; tag-based
selection is the likely shape, and it overlaps with the Survey work below
(applicability matching already wants to assemble a rubric from qualities). The
flat layout is also provisional — revisit a `projects/<project>/rubrics/` subdir
if projects accrete other files.

## Survey

Applicability matching and stub harvesting are still separate future actions.
Matching should refresh a hand-reviewed `rubric.yaml` from existing qualities.
Harvesting should only propose stubs for human authoring and must never promote
or mature a quality automatically.

## Write-back Loop

Terminus v1 records no dispositions. A later write-back pass can capture human
decisions, sharpen qualities from accumulated dispositions, and canonize
project-local qualities. That is the moment Terminus would start using
`record.WriteNotes`.

## Targeted vs. Full Review (unsettled)

Reviews currently treat the changeset (working-tree changes, given paths, or the
full tree) as *starting points* rather than a fenced scope — the reviewer ranges
across the tree and files findings wherever a problem lives. The open question is
whether the targeted/full distinction earns its keep at all, or whether every
review should effectively be a full review with changed files as a spotlight. The
working hypothesis leans toward the latter, but it is deliberately unsettled: with
only two broad-territory qualities the distinction barely has teeth, so the shape
should be decided from real-world review experience on a richer canon, not from
first principles. Two coupled questions ride along: whether selection should narrow
to the changeset or to the whole repo, and whether a stronger always-full-audit
posture replaces the current blast-radius-from-starting-points one.

## Intent Overlay

v1 reviews code against qualities only. The loop-closing mode that compares code
against a converged spec and work order remains future work. Revisit whether that
belongs in Terminus or in Mercurius before building it, since Mercurius already
owns the converged intent artifacts.

## Reviewer Hardening

The reviewer runs in the actual project tree. Pi suppresses project context
files; codex and claude currently rely on the prompt's precedence instruction.
If Terminus is pointed at untrusted repos, add backend-specific context-file
suppression and consider snapshotting/hashing reviewed bytes.

## Deferred Extensions

Continuous drift watching, execute-and-autofix, multi-round sessions, loading
grimoire convention text into prompts, path-keyed project identity, branch/base
changesets, and harness extraction of shared `errs`, `config`, or `cli` packages
all remain out of scope until there is a concrete second consumer or a real
operational need.
