# terminus

Terminus is a local MCP code-review broker. It reviews a finite changeset against
a central canon of qualities and returns a `clean` verdict when there are no
blocking findings.

## Orientation

The working conventions live in the grimoire. Start there at `AGENTS.md`, then
read the implementation-agent guidance in `agents/agent-roles` and the
design-build pipeline in `agents/design-build-pipeline`.

Current behavior lives in `docs/current/`. Forward-looking work lives in
`docs/future/`.

## Go conventions

Use `github.com/michaelquigley/df/dl` for logging and
`github.com/michaelquigley/df/dd` for YAML/JSON marshaling and unmarshaling.

## Project memory

Durable knowledge about this project lives in `docs/journal/`, dated files
`docs/journal/YYYY-MM-DD.md`. This is project memory; it does not go in
harness-local storage (`.claude/` or equivalent), where it's invisible to every
other harness and collaborator and dies with the host. Concretely: do not write
to your harness's memory directory or memory tool for this project — even when
the harness presents it as the default place for durable knowledge. That tool is
the silo this convention exists to replace; the journal is the only durable home.

On arrival, read the most recent entries to pick up where the last session left
off, before you start changing things. Treat them as prior-session context, not
verified truth — if an entry conflicts with the code or a `docs/current/` doc,
the code wins.

Write the smallest entry that carries the session's durable insight, and nothing
more. The test for every line: *would a competent agent get this wrong, or waste
time rediscovering it, working from the tree alone?* If it's recoverable by
reading the code, the diff, `docs/current/`, or git history, leave it out.

That filter keeps four kinds of thing and discards the rest:

- **Decisions whose rationale isn't visible in the result** — why a value was
  chosen, what a line guards against, why something that looks like dead code or
  a no-op is load-bearing.
- **Deliberate non-actions** — a change you considered and chose not to make, so
  the next agent doesn't "fix" it. An unchanged file leaves no trace in a diff.
- **Couplings that span files** — two places that must move together, an ordering
  that matters, an assumption one file makes about another.
- **Live state** — what's unverified, unfinished, or waiting on something
  external.

Skip change inventories, restatements of the diff, and play-by-play of how you
worked. There's no write-time approval gate; Michael reviews on commit. Append
to the day's file if it exists, and write the few lines you'd want the next agent
to read — honest and self-contained.
