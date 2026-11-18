# ADR 0004: Bounded parallel transfers and manager scheduling

Date: 2026-11-18

Status: Implemented; acceptance requires protected M3 merge and recorded checks.

## Ownership and checkpoint compatibility

The engine extends its existing HTTP loop with exclusive range ends; probes,
sequential downloads and workers share redirect, encoding, identity, length and
response-body validation. No runtime dependency is added. The manager owns job
admission, retry policy, queue ordering, checkpoints and finalization.

Four range workers are the default (configurable 1–16). Known files larger than
4 MiB with strong ETags are candidates. A full GET supplies initial metadata;
its body is closed before an actual 0–0 range probe. Unknown lengths, weak/missing
validators and ignored initial probes use sequential streaming. A failed resume
identity check still requires explicit restart; workers never append a 200 body.

Workers write disjoint 4 MiB ranges through WriteAt. Each window joins every
worker before the manager syncs the file, digests its contiguous successful
prefix, and commits the existing v1 interval checkpoint. No sparse-checkpoint
format is introduced. A failed/abandoned speculative tail is not durable and is
truncated during reconciliation. This deliberately redownloads successful ranges
beyond the first hole to preserve M2's proven-prefix recovery invariant. Progress
for parallel transfers advances per committed window (up to 64 MiB at 16 workers).
Verification, allocation ownership, exclusive publication and recovery stay in
the existing manager paths. Process-kill tests exercise sequential and parallel
transfers at all eight existing crash boundaries; these do not establish
power-loss resilience.

## Shared resource bounds

Manager settings are immutable while jobs or a queue drain are active. Defaults:
two active jobs (1–16), four requests per hostname (1–64), 1,000 unfinished jobs
(1–10,000) and unlimited bandwidth. Completed/cancelled records are history.
Admission is serialized in the manager; one job cannot run twice. Close refuses
active work; Pause cancels it, and callers join Run/RunQueue before destructive
cancellation or Close. Cross-process ownership remains the existing SQLite
state-directory lock.

One engine transport wrapper counts active response bodies per normalized
hostname, across jobs and redirected requests. Closing a body releases its slot;
unused hostname entries are removed. A global optional body-byte limiter uses
serialized reads of at most 32 KiB, with at most one chunk of burst. Cancellation
interrupts admission and rate waits. This controls application response bytes,
not TLS overhead or bytes already buffered in the OS. Host aliases are separate;
ports for one hostname share a limit. There is no throughput acceleration claim.

## Queue and retry persistence

Create persists queued jobs without starting them. Reorder atomically updates
the complete queued set using one storage transaction. New jobs follow existing
orders; legacy v1 records use creation time then ID to resolve their zero order.
RunQueue drains a bounded snapshot in order with the configured active-job limit.
New enqueues wait for the next invocation. Failures do not stop independent jobs;
caller cancellation stops admission and joins active jobs. Pause leaves a job
paused; explicit resume operates on it independently. Queue mutation/cancellation
of snapshot jobs is rejected during draining, avoiding stale command races.

Three automatic retries are default (0–8). Transient network/truncated-body errors
and HTTP 408/429/500/502/503/504 use exponential backoff, initially one second.
Retry-After seconds and dates are respected; waits exceeding five minutes return
failure for explicit retry rather than shortening the server's delay. No jitter
is currently applied. Disk, authentication, verification and unsafe-protocol
errors do not authorize automatic retry. Joined errors must all be retryable.
Strong-validator progress is resumed; an interrupted stream without a strong
validator restarts its owned partial during an automatic transient retry.

Retry count and next attempt time are persisted. A server Retry-After deadline
is saved even when automatic retries are disabled/exhausted or its wait exceeds
the automatic five-minute ceiling; explicit resume after relaunch still waits.
Startup pauses interrupted
retry waits; an explicit resume respects their remaining delay. Retry budget is
per Run invocation; a new explicit resume starts a fresh bounded budget. No
automatic application startup or background daemon is introduced.

Migration 002 adds retry_wait to SQLite's state constraint without editing the
released 001 migration. Additional JSON metadata is backward compatible with
v1 checkpoints; read-only M2 status is supported before upgrade. Migration
rollback, preserved checkpoints, reopen/idempotence and atomic reorder rollback
have deterministic fixtures. M2 binaries cannot open the upgraded schema.
