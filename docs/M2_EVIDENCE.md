# M2: Persistent downloads and safe resume

Updated: 2026-11-17. M2 accepted with local and hosted platform evidence.
The successful hosted run below validates the implemented M2 code; subsequent
PR heads must independently pass all required checks before merge.

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

## Hosted acceptance

[Hosted run 44082279209](https://github.com/Mahmoud-s-Khedr/idm-enhanced/actions/runs/44082279209)
passed at code head `837225a784aad80854c53c5dfc94964b1b51a152`, including
quality, all three native jobs and the protected **Required checks** aggregate.
The quality job passed full checks, the history secret scan and all 20 gate probes.
Hosted critical coverage was engine 94.9%, manager 90.2%, storage 90.1%.
Go 1.27.4, Node 26.13.1, pnpm 12.21.0 and Wails 3.0.0 remained pinned.

| Native runner | Actual target | Native evidence                                                                                                            |
| ------------- | ------------- | -------------------------------------------------------------------------------------------------------------------------- |
| ubuntu-24.04  | linux/amd64   | Build, ordinary tests including process-kill recovery, CLI/native-host integration passed                                  |
| windows-2025  | windows/amd64 | Build, ordinary tests including process-kill recovery, native-host integration passed; CLI SIGINT tests explicitly skipped |
| macos-15      | darwin/arm64  | Build, ordinary tests including process-kill recovery, CLI/native-host integration passed                                  |

Windows cannot send os.Interrupt through Go. Its manager context cancellation,
retention and eight real process-kill/relaunch recovery boundaries passed; this is
not a skip of Windows recovery. The Windows native tests also verify current-user
ownership normalization for elevated tokens and reject public ACLs. macOS tests
verify its relative system temporary-directory aliases. The CLI interruption
fixture waits for client-observed metadata before signalling and gives each
independent command its own bounded deadline.

The macOS runner reported 15.8.2 (24H232). Linker deployment-target warnings remain;
this run establishes behavior on that runner, not compatibility with older macOS.
The protected branch requires an up-to-date **Required checks** result. Every later
PR head, including documentation-only updates, must pass independently.

## Remaining limits

Supported filesystems are local, with reliable locks and hard links. Windows
directory durability is unsupported; neither Unix directory fsync nor process-kill
tests establish power-loss resilience. Desktop jobs, queues, automatic retries,
replacement URLs, request credentials and legacy M1 partial import remain deferred.
