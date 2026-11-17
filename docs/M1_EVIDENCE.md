# M1: Sequential CLI

Updated: 2026-11-17. Implementation complete; local validation recorded below.

## Delivered behavior

`cmd/downloader` downloads one HTTP(S) URL into an explicitly chosen, new file.
The engine remains independent of desktop/browser code; `internal/cli` only parses
arguments, presents progress/errors, and maps exit codes. No dependencies added.

- GET without requiring HEAD; known and unknown sizes, bounded redirects.
  Generated referrers are omitted, and every HTTPS-to-HTTP redirect hop is
  rejected, including after an initial HTTP-to-HTTPS upgrade.
- A fixed 32 KiB transfer buffer; no full-response buffering or parallel workers.
- Identity encoding, HTTP status and body-length checks, context cancellation,
  whole-transfer timeout (30 minutes by default), and throttled progress.
- Optional expected SHA-256, checked before finalization.
- Private randomized partial files beside the destination, synced and closed before
  exclusive hard-link publication. Existing files and destination symlinks are
  rejected; a concurrent destination creation cannot be overwritten.
- Failure/cancellation removes the owned partial, reporting cleanup failures.
  Cancellation exits 130, usage errors 2, other failures 1, success/help 0.
- Ctrl+C and SIGTERM cancel safely. Transport diagnostics preserve error identity
  while hiding URLs, query strings, and request metadata.

The output filesystem must support hard links. Failure to publish is an error;
there is no fallback that could overwrite another file. Parent directories must
already exist and be trusted. Directory durability, power-loss recovery, resume,
retries, persistent jobs, and authenticated request handoff are outside M1.
Cancellation before publication prevents publication; cancellation after the
exclusive link succeeds may race with successful completion. A cleanup failure
after publication reports an error even though the verified destination exists.

## Acceptance evidence

Fixtures run locally; mandatory tests never contact public services.

- `go test -cover ./internal/engine ./internal/cli`: passed; engine 92.2%, CLI 100%.
- `go test -tags=integration ./integration -run TestDownloaderProcess -count=1`:
  passed; real executable downloads/checksums a fixture and responds to SIGINT
  with exit 130 and no partial/output left behind.
- `go test ./internal/engine -run '^$' -bench BenchmarkTransferStreaming -benchmem -benchtime=3x`:
  passed; generated 64 MiB per operation used 34,658 B/op and 18 allocations/op on
  Linux amd64. This measures transfer-loop allocation, not network throughput or
  whole-process RSS. A separate test rejects writes larger than 32 KiB.
- Known/unknown-size and redirect fixtures match expected SHA-256. Tests cover
  interrupted bodies, rejected HEAD reliance, bad status/encoding, cancellation
  before and during transfer, stalled-body timeout, short/disk writes, response
  close failures, invalid input, checksum mismatch, existing/symlink destinations,
  and destination creation during transfer.

- `go run ./tools/quality doctor`: passed.
- `go run ./tools/quality check`: passed.
- `go run ./tools/quality check:full`: passed, including the final 92.2% engine
  coverage, 100% CLI coverage, race suite, both CLI/native-host process suites,
  Gitleaks, govulncheck and pnpm audit. Both vulnerability scanners reported no
  known vulnerabilities at execution time.
- `env GOOS=windows GOARCH=amd64 go build -o /tmp/idm-m1-windows.exe ./cmd/downloader`:
  passed (cross-compilation only).
- `env GOOS=darwin GOARCH=arm64 go build -o /tmp/idm-m1-macos ./cmd/downloader`:
  passed (cross-compilation only).
- `go run ./tools/quality gate-probes`: passed all 18 controlled rejection checks.
  Go probe cleanup removes only its injected file, preserving the real engine
  package between probes.
- `git diff --check`: passed.

The [M1 hosted run](https://github.com/Mahmoud-s-Khedr/idm-enhanced/actions/runs/44073354322)
passed all quality checks/probes, the required aggregate, and native build,
ordinary tests and integration jobs on Linux amd64, Windows amd64 and macOS arm64.
Subsequent redirect-scoping fixes passed all canonical checks/probes locally and
add regression tests for signed-URL referrer disclosure and HTTP-to-HTTPS-to-HTTP
downgrades. [PR #2](https://github.com/Mahmoud-s-Khedr/idm-enhanced/pull/2) requires
a fully successful check of its updated head before merge.

Windows explicitly skips process SIGINT because Go cannot send os.Interrupt
there; context cancellation is tested normally. Interactive Windows/macOS use and
power-loss resilience remain unverified.

The M2 checkpoint protocol is specified in [ADR 0002](decisions/0002-m1-transfer-and-m2-checkpoints.md).
