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

## Write-back Loop (dispositions and sharpening)

Terminus v1 records no dispositions: every review starts blank, so a finding the
operator already weighed re-fires on the next run. That is tolerable while review
is semi-manual and occasional — a human re-rejects and moves on — but it does not
scale to the automated, frequent review this is heading toward, where a verdict
dominated by known-rejected findings buries the signal. Convergence depends on the
system remembering what was already decided.

Two distinct things make a finding stop recurring, and only one needs new
machinery:

- **Settled by rule change.** The finding was a false positive because the *rule*
  was too broad; the fix is to sharpen the quality's `discrimination`/`boundary`,
  after which it genuinely should not fire anywhere. This already works — it is how
  the audio-thread "heavy vs cheap" refinement landed. No disposition store needed.
- **Settled by disposition.** The finding is legitimate under the rule, but the
  operator accepts, defers, or rejects it *for this project* (a deliberate boundary
  case). This is what needs a write-back store: a per-project record of
  "considered, decided X, don't re-raise."

### What a disposition pass would need

- **A stable finding identity across runs** — the hard problem. A finding's `id`
  and line numbers drift run-to-run, so dispositions must key on a fingerprint that
  survives edits (quality id + file + a normalized locus/claim, not the raw line
  range). Get this wrong and dispositions either leak (suppress the wrong finding)
  or never match (suppress nothing).
- **A durable, per-project store outside the subject repo**, written through
  `record.WriteNotes` (the hook reserved for this), alongside the review records.
  Dispositions are review metadata, not project code.
- **A suppression path at review time** — feed "these were considered and settled,
  do not re-raise" into the reviewer prompt, or filter settled findings during
  classification, or both: a prompt-level hint (so the reviewer spends no attention
  on them) with a post-classification safety net. The prompt-feed risks the
  reviewer rationalizing around a suppression; the post-filter is cleaner but wastes
  reviewer attention. A hybrid is likely.
- **Human-authored sharpening, not auto-maturation.** Accumulated rejections of a
  quality signal that its `discrimination` is wrong and can *propose* a sharpening,
  but a human authors it — consistent with the propose-don't-promote discipline.
  Dispositions never silently rewrite a quality or canonize a project-local one.

This is a larger effort than the v1 spine, gated on real review volume making the
recurrence painful — which the move toward automated review will bring. The
`changeset_kind` naming finding (deliberately deferred, with nowhere to record
that) is the standing example of the gap.

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
