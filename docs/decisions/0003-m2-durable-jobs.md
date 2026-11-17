# ADR 0003: Durable jobs and safe sequential resume

Date: 2026-11-17

Status: Implemented; M2 accepted with local and hosted evidence (M2_EVIDENCE.md)

## Ownership and storage

The manager owns all durable job commands, transitions, partial allocation,
checkpoint barriers, reconciliation, checksum verification and publication.
The former engine.Download implementation has been removed; its regression
fixtures now exercise the manager. The engine retains one HTTP loop for full GET,
probe and range GET. It has no manager, storage or presentation imports. localfs
owns native identity, process locks, access protection and directory sync.

SQLite uses database/sql with a single retained connection, verified WAL,
synchronous=FULL, foreign_keys=1 and busy_timeout=5000. Embedded 001.sql runs in a
transaction. SQL columns constrain states, generation and durable byte counts;
the versioned JSON record stores source identity and checkpoint intervals atomically
with those columns. Unknown database and checkpoint versions are refused. Status
opens mode=ro and performs neither migrations nor manager recovery. A process-held
flock or LockFileEx lock serializes modifying commands and releases on process death.
Unix state paths are owned by the current UID and use 0700/0600. Windows uses a
protected current-user DACL with inheritable access for newly created SQLite
sidecars. The token default owner is recognized for elevated Windows creation;
protection explicitly assigns ownership to the current user. Read-only checks
require a user-only DACL, including on newly created sidecars. Symlinks are rejected, except the known macOS /var and /tmp system aliases.
The destination's parent remains a trusted, existing directory as in M1.

Signed URLs are stored only in this protected database; no headers or cookies are
persisted. CLI output exposes only IDs, safe categories and local destinations.

## Dependency decision

The standard library has no SQLite driver. Existing Wails dependencies did not
supply a production persistence owner. Choose the approved pure-Go modernc SQLite
driver over adding a C compiler/runtime integration for SQLite. Pins:
modernc.org/sqlite v1.62.0, modernc.org/libc v1.80.1, x/sys v0.49.0.
The driver requires Go 1.26; this repository pins Go 1.27.4.

The pinned [upstream manifest](https://github.com/modernc-org/sqlite/blob/v1.62.0/go.mod)
and downloaded, checksum-verified module sources were inspected. Runtime transitive
additions/updates include libc, memory v1.12.1, mathutil v1.7.1, go-humanize v1.1.0,
uuid v1.6.0, go-isatty v0.0.24, go-strftime v1.1.0 and bigfft. go.sum also records
upstream test/tool modules, including pprof, fileutil and the modernc compiler
toolchain; these are not all linked into the downloader.

SQLite's translated driver, libc, memory, mathutil and x/sys use BSD-style licenses;
uuid and go-humanize are MIT, go-isatty is MIT, go-strftime is MIT, and bigfft is
BSD-style. Preserve upstream notices in distributions. SQLite itself is public
domain. The driver/libc version coupling is a maintenance constraint: upgrades
must follow the driver manifest and rerun migration, recovery and native tests.

On local Linux amd64 with Go 1.27.4, `go build -trimpath` produced a 9,122,303-byte
M1 downloader and a 17,326,943-byte initial M2 downloader (+8,204,640 bytes, about
90%). This includes the new application logic as well as SQLite. Frontend budgets
are unchanged. This measures unstripped executables, not installer size or RSS.
Cross-compilation is recorded separately from native runtime compatibility.

## Checkpoints and publication

A 32 KiB streaming write loop checkpoints at write boundaries after 4 MiB or one
second, on pause and at EOF. The manager syncs data, hashes the new half-open
interval with a bounded buffer, then commits the interval, generation and durable
prefix together. Failed barriers do not advance acknowledged progress. Cleanup
uses fresh five-second contexts after transfer cancellation. Recovery and full-file
verification use the command context, so large files are not limited by the pause
cleanup deadline.

Recovery verifies native identity and ordered interval digests, persists the
longest valid prefix, then truncates uncertain bytes. Missing/replaced partials
require restart. Restart abandons replaced files without deleting them. Unknown
allocations are retained and allocation persistence failures are reported.

Resume requires a strong ETag and a validated bytes=0-0 If-Range probe, then an
exact range from the proven prefix. Effective URL, validator, encoding, range and
length must agree. A full 200 is never appended; 416 never proves completeness.
Zero-byte resources use full GET validation. Unsafe cases enter needs_attention.

Verification rereads the whole file for SHA-256, honors cancellation, syncs and
closes the file, then commits identity, digest and length in the verified phase.
Exclusive hard-link publication preserves collisions. Recovery accepts a published
destination only with matching native identity, full digest and length. Directory
sync surrounds partial removal before completion. Verified cancellation remains
cancelled until an explicit resume; completed files are never removed.

Unix directory fsync is implemented. Windows directory flushing is explicitly
unsupported; M2 tests establish process-crash recovery, not power-loss resilience.
The test binary exposes deterministic persistence boundaries internally; production
commands have no environment-controlled fault injection.
