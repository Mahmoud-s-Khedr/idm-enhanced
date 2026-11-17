# M2: Persistent downloads and safe resume

Updated: 2026-11-17. Implementation and local validation complete.
Hosted M2 acceptance is pending. M0/M1 run links do not establish M2 compatibility.

The CLI persists all new downloads, prints job IDs, supports status, resume,
restart and explicit cancellation retention, and pauses on interruption/timeout.
See [CLI usage](../README.md) and [ADR 0003](decisions/0003-m2-durable-jobs.md).

## Deterministic evidence

Engine fixtures cover GET without HEAD, unknown lengths, safe redirects and
redaction, strong/weak/missing validators, range probes, exact resume, ignored
ranges, changed identity/size, invalid Content-Range, short/long bodies and 416.
Manager fixtures cover damaged, truncated, missing and replaced partials, unknown
length becoming known, committed-prefix rewind, uncommitted tails, checksum
mismatch, cancellation retention, explicit restart, destination collisions and
publication recovery. Failed sync and checkpoint transactions preserve durable
progress. Storage tests cover reopen, exclusive ownership, read-only status,
migration rollback/idempotence, newer versions, constraints and unsafe paths.

Real test subprocesses are killed before/after file sync, after checkpoint commit,
before/after verified-record commit, after publication, after partial removal and
after completion commit. A newly launched process recovers and produces the exact
fixture bytes. This is process-crash evidence on the executing OS. Linux CLI
SIGINT tests assert exit 130 and a retained partial. Windows cannot send
os.Interrupt through Go; ordinary context cancellation remains tested there.

Critical package coverage is enforced at 90% for engine, manager and storage;
other package thresholds remain unchanged. Controlled rejection probes cover the
critical threshold and engine-to-storage dependency boundary.

## Local validation

All commands below passed on Linux amd64 with the pinned toolchain:

- `go run ./tools/quality doctor`
- `go run ./tools/quality check`
- `go run ./tools/quality check:full` (including race, CLI/native-host integration,
  Gitleaks, govulncheck and pnpm audit; no known vulnerabilities reported)
- `go run ./tools/quality gate-probes` (20 rejection/integrity checks)
- `go test -race ./internal/engine ./internal/manager ./internal/storage ./internal/cli`
- `go test -tags=integration ./integration -run TestDownloader -count=1`
- `git diff --check`

Final local statement coverage: engine 94.9%, manager 90.2%, storage 90.1%,
CLI 95.1%, localfs 86.5%. Existing thresholds elsewhere remain unchanged.
The ordinary suite runs the real process-crash tests; the race suite also passes.

`go test ./internal/manager -run '^$' -bench BenchmarkChecksum -benchmem -benchtime=3x`
reported 33,066 B/op and six allocations for both 1 MiB and 64 MiB inputs.
The engine retains its 32 KiB streaming benchmark and bounded-write fixture;
checksum fixtures also reject reads larger than 32 KiB. Repeated pauses prove all
fixture request handlers exit after cancellation. These measure bounded buffers
and request lifetimes, not whole-process RSS or every possible OS resource leak.

`env GOOS=windows GOARCH=amd64 go build -o /tmp/idm-m2-windows.exe ./cmd/downloader`
and `env GOOS=darwin GOARCH=arm64 go build -o /tmp/idm-m2-macos ./cmd/downloader`
passed. These are cross-compilation results only. Windows database URI paths
include the required leading slash before the drive letter.

## Acceptance limits

Hosted Linux/Windows/macOS native build, test and recovery results and the protected
aggregate for this change are required before M2 is accepted. No such M2 hosted run
has been executed in this workspace. Existing CI executes ordinary tests (including
process-kill recovery) on all three native platforms and CLI integration tests.
Cross-compilation alone is not native runtime evidence.

Supported filesystems are local, with reliable locks and hard links. Windows
directory durability is unsupported; neither Unix directory fsync nor process-kill
tests establish power-loss resilience. Desktop jobs, queues, automatic retries,
replacement URLs, request credentials and legacy M1 partial import remain deferred.
