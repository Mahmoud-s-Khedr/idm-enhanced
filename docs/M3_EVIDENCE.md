# M3: Parallel engine and manager

Updated: 2026-11-18. Local validation passed; hosted acceptance is pending.
Acceptance requires the protected merge of this milestone's PR.

## Delivered behavior

The CLI supports enqueue, run-queue and reorder, alongside M2's download, resume,
restart, status and explicit-retention cancellation. Workers, simultaneous jobs,
per-host requests, global body-byte rate and retries are configurable. Defaults
are four range workers, two jobs, four requests per hostname and three retries.

The manager retains ownership of lifecycle and checkpoints. Parallel workers
join before a verified contiguous prefix is synced and committed. Uncommitted
speculative ranges are discarded on recovery. See
[ADR 0004](decisions/0004-m3-parallel-manager.md) for exact policy and tradeoffs.

## Deterministic evidence

Fixtures cover exact segmented output, ignored-probe sequential fallback,
weak/unknown representations, changed worker identity, short/overlong bodies,
worker cancellation, failed checkpoints, request admission, active-job limits,
global bandwidth pacing and cancellation, queue ordering/reopen, atomic reorder,
transient retry success/exhaustion, Retry-After, retry-wait recovery, and permanent
failure rejection. Migration fixtures upgrade the released M2 schema, retain its
checkpoints, and verify rollback and read-only access.

Real subprocesses are killed and relaunched at eight sync/checkpoint/verification/
publication boundaries for both sequential and parallel transfers. Each must
recover the fixture's exact bytes. A separate real process-kill test interrupts
actively writing range workers and recovers the exact file after relaunch.
The existing M2 corruption, ownership,
credential-redaction and finalization suites remain mandatory.

## Validation

All commands below passed locally on Linux amd64 with the pinned toolchains:

- `go run ./tools/quality doctor`
- `go run ./tools/quality check`
- `go run ./tools/quality check:full` (final revision, including race, subprocess
  integration, Gitleaks, govulncheck and pnpm audit)
- `go run ./tools/quality gate-probes` (all 20 rejection/integrity checks)
- `go test -race -count=1 ./internal/engine ./internal/manager ./internal/storage ./internal/cli`
- `git diff --check`

Final full-check statement coverage: engine 95.7%, manager 91.6%, storage 90.4%,
CLI 95.9%. Critical thresholds remain 90% per package. Gitleaks and govulncheck
reported no findings. pnpm audit retained the existing reviewed development-only
picomatch applicability exception, expiring 2026-12-01; raw findings remain visible.

Native Windows/macOS results and the required aggregate must pass on the hosted
PR head before merge; local results do not establish those platform behaviors.
Thresholds, exclusions, required jobs and dependency pins remain unchanged.

## Limits

Parallel progress updates occur at window commits. Speculative ranges beyond a
hole may be downloaded again. Queue draining uses a snapshot; new enqueues wait
for the next invocation, and paused jobs require explicit resume. Retry budgets
reset on a new explicit invocation; startup never automatically starts jobs.
Bandwidth measures response-body bytes with a 32 KiB burst, excluding protocol
and kernel buffering. No benchmark speedup, fairness or whole-process RSS claim
is made. M2 filesystem/ownership, Windows directory-durability and power-loss
limitations remain. Desktop management, browser handoff and media remain later
milestones.
