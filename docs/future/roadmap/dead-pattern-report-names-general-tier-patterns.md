---
title: dead-pattern report names general-tier patterns
state: inbox
created: 2026-09-09
tags: [enhancement]
---

`audit_coverage` reports a dead pattern for every territory glob on every composed quality, general tier included. On a Go project that means `truthful-naming`'s `**/*.cpp`, `**/*.h`, `**/*.hpp`, and `**/*.mk` are listed as dead on every audit, and they will be on every Go project in the canon, because a general-tier quality's territory deliberately spans languages. Scope dead-pattern detection to project-local qualities, or label each dead pattern with its tier so a reader can skip the general ones at a glance.

## why

A dead pattern is meant to say "this territory names a package that was renamed or removed" — canon lagging one project. A general-tier pattern for another language is not that; it is the tier doing its job. Four permanent entries at the top of every Go project's audit teach the reader to skim the list, which is how the one real dead pattern gets missed.
