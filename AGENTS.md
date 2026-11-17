# Repository instructions

## Current stage and references

M0–M2 have recorded platform evidence. Read `docs/M2_EVIDENCE.md`
for persistent download/resume acceptance and limitations. M3 parallel engine/manager
is next; `docs/M1_EVIDENCE.md` remains the historical sequential CLI evidence.
Read `PROJECT_PLAN.md` for scope and `docs/QUALITY_POLICY.md` for engineering standards.
Canonical tooling is implemented at `tools/quality`; run `doctor`, `check`, and `check:full`.
Read `docs/GATES.md` and `docs/M0_EVIDENCE.md` for activation status and actual platform evidence.
Begin M3 only after the protected merge of M2 PR #3; keep its scope within the
plan and preserve the M2 recovery invariants.

## Before changing code

- Inspect the relevant code, tests, dependencies, and existing patterns first. Use `rg` to search for existing implementations before adding a helper, type, component, or dependency.
- Identify the current milestone and its acceptance criteria. Keep changes focused on the task.
- Reuse the established owner of each behavior. Explain necessary duplication rather than silently creating a parallel implementation.
- Add dependencies only after checking the standard library and existing packages. Record purpose, runtime impact, maintenance, and license considerations.

## Architecture and duplication

- Keep the Go engine independent of Wails, frontend code, and browser APIs.
- Keep job lifecycle and queue policy in the manager, persistence in storage, and presentation in adapters. Do not reproduce those policies in React, the CLI, or the extension.
- The native host forwards validated commands; it must not become another engine.
- Share browser-independent extension logic and keep browser-specific adapters small.
- Define protocol contracts once. Generate frontend bindings where appropriate and never manually edit generated files.
- Share HTTP transfer facilities across file and media work without treating file byte ranges and media objects as identical jobs.
- Prefer small cohesive modules and interfaces at actual boundaries. Avoid generic frameworks, broad `utils` collections, and abstractions created only to satisfy a duplication metric.
- Keep side effects explicit and validate untrusted input at the receiving boundary.

## Correctness and testing

- Preserve the project's HTTP range, resource identity, checkpoint, credential-scoping, and finalization invariants.
- Bound concurrency, retries, buffers, and queues. Propagate cancellation; release response bodies, files, timers, and goroutines.
- Do not swallow errors or report completion before required verification/finalization succeeds.
- Add regression tests for behavioral bugs and meaningful tests for new behavior, especially failure and recovery paths. Do not add implementation-mirroring tests solely for coverage.
- Use deterministic HTTP/media fixtures. Do not depend on public services for mandatory tests.
- Use typed frontend code; avoid unexplained `any`, unsafe assertions, and broad lint suppressions.
- Never log credentials, signed URL queries, or sensitive request metadata.

## Validation and gate integrity

- Use `go run ./tools/quality check` and `go run ./tools/quality check:full`. Explicit bootstrap installs tools; hooks and ordinary checks must not download them.
- Run focused checks during work and the applicable mandatory checks before handing off implementation changes. Report exact commands, outcomes, and checks that could not run.
- Do not claim CI, coverage, browser compatibility, signing, or platform behavior was verified without evidence.
- Do not lower thresholds, broaden exclusions, disable checks, remove required jobs, or change tests merely to hide a failure. Legitimate policy changes need explicit rationale and visible review.
- Keep generated output, lockfiles, migrations, and fixtures consistent with their sources. Do not edit an already released migration; add a new one.
- Do not overwrite unrelated user changes. Keep documentation aligned with changed behavior.

## Completing a change

Summarize the behavior changed, why it changed, how it was verified, and remaining limitations.
For architecture or dependency changes, update the relevant decision record and support documentation.
Treat a green tool report as evidence, not a substitute for reviewing data integrity, concurrency, lifecycle, and architecture.
