# Terminus loop-control: handoff from a real Sterling run

**From:** Sterling review work, 2026-07-15
**To:** terminus team
**TL;DR:** A single implementation agent took **7 review rounds** to land one stage. The *findings were almost all real and the fixes were sound* — the code did not spiral into damage. What spiraled was the **loop**: it has no principled stopping point, so it ran until the reviewer happened to sample a clean pass. Three changes fix it. None touch the canon; all are terminus/orchestration behavior.

## What we observed (the evidence)

One stage (Sterling stage 7), reviewed by `pi` / `openai-codex/gpt-5.6-sol` against the Sterling rubric. Seven consecutive rounds, verdicts:

```
R1 not_clean → R2 not_clean → R3 CLEAN → R4 not_clean → R5 CLEAN → R6 not_clean → R7 CLEAN
```

That is **oscillation, not convergence.** Blocking findings appeared *after* clean rounds (R4 after clean R3; R6 after clean R5). Findings logs: `~/.local/share/terminus/sterling/<session>/_findings.md`.

The blocking findings were all legitimate and got sound fixes (an attestor-distinctness trust hole, a fail-closed gap, a duplicated-invariant refactor, etc.). This is **not** a "reviewer is too noisy / findings are wrong" report. The reviewer was good. The loop around it was the problem.

## Root causes, in leverage order

### 1. The loop does not stop on a clean verdict (highest leverage, lowest cost)

R3, R5, and R7 came back `verdict: clean` — **zero blocking findings** — yet the loop continued each time. It continued because advisory (non-blocking) findings were still present and were treated as actionable work.

**Key point for cost:** terminus already emits everything needed to fix this. Each finding already carries `blocking: true|false`, and the round already computes `verdict: clean` when no blocking findings remain — *even while advisory findings are listed*. So the data model needs no change. The fix is loop policy:

- **Terminate the implementation loop on the first `clean` verdict.**
- **Advisory findings never re-trigger a round.** They accumulate into a human triage list, surfaced once at the end.

Had this been in place, the run stops at **R3** instead of R7. (~4 rounds saved.)

### 2. Blocking findings should reproduce before they block (removes oscillation)

The late blockers (R4 `spiffe://`-prefix edge, R6 missing-ledger) were **pre-existing latent issues the reviewer simply didn't surface until round 4/6** — not regressions the agent introduced. That is reviewer sampling variance, and it's what makes the verdict oscillate: the same tree yields different findings on different samples.

- **Two-sample rule:** run the reviewer independently ≥2× on the same tree; a finding **blocks only if it reproduces** across samples. Single-sample findings drop to advisory (→ triage list).

This is terminus-core (it changes how a round is sampled). It trades some latency for a stable verdict and kills phantom oscillation. Tune the sample count / quorum to taste.

### 3. Human checkpoint after N blocking-fix rounds (backstop)

Even with (1) and (2), a genuine spiral is possible — an agent whose fix for X introduces Y (we saw one mild instance: a dedup refactor that a later round flagged as a duplicated invariant). A hard backstop catches the case where the agent is making things *worse*, before it compounds.

- **After N blocking-fix rounds without convergence (≈3), pause and require human review** rather than looping indefinitely.

This is loop/orchestration policy — terminus if it hosts the loop, otherwise the agent-driver harness.

## What was handled canon-side (no terminus work needed)

One driver of the round count was a *conditional* rubric quality (`adversarial-member-skips`, batch resilience) with an effectively unbounded tail of ever-more-exotic instances — by R7 it was flagging an operator-supplied CLI `--attestation` FIFO, which isn't an adversarial input at all. That was fixed **in the canon** by adding a threat-model floor to the quality's own text (fire only on adversarial-reachable members; exclude operator-supplied local inputs). Noted here only so the pattern is on the team's radar: *conditional/heuristic qualities want an explicit scope floor, or they generate a diminishing-returns tail that an advisory-chasing loop will pursue forever.* This is as much a canon-authoring lesson as a terminus one.

## Net

- #1 (stop-on-clean, advisories→triage): small policy change, data model already supports it. **Do first.**
- #2 (reproduce-to-block): terminus-core, removes oscillation.
- #3 (human checkpoint at N): backstop against real spirals.
- Canon-side scope floors on conditional qualities: already applied for Sterling; worth a general authoring note.
