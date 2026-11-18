# M7: Bounded HLS VOD media

Updated: 2026-11-19. Supported-profile implementation is complete; protected
acceptance is tracked in [PR #9](https://github.com/Mahmoud-s-Khedr/idm-enhanced/pull/9),
which records the final head, required checks, artifact verification and merge.

M6 PR #8 merged as 0acec4abf9e8fee1b427c427ecffa261bcac4e9b after final head
2d7fb98a1175ab363a4d8130705746e122f2de2e passed
[Quality run 44342722093](https://github.com/Mahmoud-s-Khedr/idm-enhanced/actions/runs/44342722093).
Both review findings were corrected and resolved before protected merge. The
owner explicitly deferred human beta testing and directed work to M7 afterward.

## Delivered profile

Master/media analysis, explicit variant selection, bounded complete-object
transfers, persistent object checkpoints, pause/restart recovery and offline
assembly retry. FFmpeg stream-copies H.264/AAC MPEG-TS to fragmented MP4; ffprobe
and the existing manager finalizer verify output before completion. The desktop
analyzes/queues/selects quality and reports object progress. CLI media-info and
-hls/-quality options use the manager. Browser link menus and popup explicitly
queue HLS; current-tab discovery is opt-in and bounded, with no cookie access or
automatic interception. See [ADR 0007](decisions/0007-m7-hls-vod.md).

Only explicit VOD+ENDLIST playlists, versions 1–3, are supported. Encryption/DRM,
live/event/untyped content, fMP4 input, ranges, discontinuities, alternate audio
and unknown extended semantics are rejected. This is a defined initial subset,
not general HLS/site extraction. DASH remains M8.

## Deterministic and local evidence

- Parser fixtures cover relative URLs, redirects, quoted attributes, mixed
  master/media rejection, unsafe URLs, malformed/bounded input, unsupported
  profiles, immutable source changes and non-leaking choice metadata.
- Manager fixtures cover retained strong/weak objects, exact ordering, damaged,
  replaced/missing caches, foreign output collisions, explicit discard/restart,
  unconsumed input, offline assembly retry and bounded analysis cancellation.
- A parallel transient HTTP failure cancels peers without converting cleanup into
  a user pause/retry veto; joined disk errors remain failures.
- Real subprocesses are killed at nine media/object/assembly/publication
  boundaries and relaunched to exact verified output. Existing sequential and
  parallel eight-boundary suites remain active. These establish process-crash
  behavior, not power-loss resilience.
- Cancellation before/after the verified commit can resume locally; a committed
  verified file also retries publication without contacting its source.
- Storage fixtures prove released-schema upgrades/rollback, preserved browser
  receipts/file checkpoints, atomic summary/checkpoint writes, bounded payloads,
  lightweight history reads and checkpoint cascade on metadata deletion.
- Checked-in synthetic 128×72 and 256×144 H.264/AAC fixtures assemble with real
  FFmpeg/ffprobe, select the expected resolution and fully decode successfully.
  Tools are initially absent in the fixture run: after complete downloads, the
  server is closed, tools restored, and assembly succeeds without network access.
- Frontend tests cover pending-analysis edit locks, explicit quality selection,
  unsupported-source feedback, segment progress, media mode retry, bounded
  discovery, candidate selection and main-frame source privacy.
- Real Firefox 160.0 and Chromium 157.0.8103.78 native handoff/retry passed for
  files and HLS, with exactly four durable jobs and manager-owned MP4 names.
- Real native WebView smoke passed file workflows plus HLS analysis, lower-quality
  selection, object progress, playable MP4 and cache cleanup on history removal.
  The driver now distinguishes settings/add-form checkboxes and waits for HLS
  analysis enablement after entering its URL; assertions were retained.

An initial real mux failure exposed the required AAC ADTS-to-ASC filter. FFmpeg's
nonzero exit correctly prevented publication; the corrected stream-copy pipeline
passes actual playback. A native test selector initially matched the new HLS
checkbox while waiting for settings; scoped selectors corrected the harness.

## Validation status

Final local validation on Fedora 45 amd64:

- `go run ./tools/quality doctor` — passed environment diagnostics.
- `go run ./tools/quality check` — passed formatting, lint, types, architecture,
  clones, dependency drift, tests/coverage, generated drift, build and budgets.
- `go run ./tools/quality task desktop-smoke` — passed the native workflows above.
- `go run ./tools/quality task browser-smoke` — passed both real browsers above.

Critical Go statement coverage: engine 96.1%, manager 91.0%, storage 92.3%,
media 97.7%. The 46 TS tests passed; aggregate statements 98.16%, branches
92.26%, functions 99.04%, lines 99.15%; per-file thresholds also passed.
Desktop emitted assets: raw 262,406 / 303,185 bytes; gzip 84,440 / 103,429.
Each extension: raw 8,959 / 11,195; gzip 4,152 / 9,642.
`go run ./tools/quality check:full` also passed: all ordinary gates, race
checks, process/IPC integration, real media smoke with FFmpeg/ffprobe 9.0.2,
secrets and Go (host/Windows)/JS vulnerability scans. Hosted platform evidence
requires the exact PR head's required checks before protected merge.
No Go/JS dependency, threshold reduction, budget increase or required-job bypass
is introduced. FFmpeg is separately installed and not bundled; the full gate
requires real media tools rather than silently skipping playback tests.

## Protected validation and review

Initial head 2d9c9901155bebf0b68cde132808c19e9d45f3cd passed
[Quality run 44359948011](https://github.com/Mahmoud-s-Khedr/idm-enhanced/actions/runs/44359948011),
including all three native platforms and Debian/Fedora/Windows controlled
installer upgrades. All nine artifact hashes and six extension archives passed
local checksum/version/background-entry verification. All 19 local gate probes
passed, including staged-index preservation. The staged runner excludes only
binary MPEG-TS fixtures from TypeScript lint, matching the formatter/config scope.

A late review found output-allocation errors were incorrectly classified as
ownership failures. The fix preserves retryable assembly errors for allocation
failures, while existing partial ownership failures remain explicit attention
cases. A regression removes the output directory, completes segment downloads,
then restores the directory and successfully retries assembly with the origin
offline and no further requests. Final-head validation and protected merge are
recorded in PR #9; the initial run is not evidence for the corrected head.

## Limits and pending acceptance

The owner's human beta workflows remain deferred. Discovery's DOM/resource logic
has deterministic tests; live browser tests establish native handoff, not every
activeTab permission/UI flow or site. Windows/macOS real FFmpeg playback and
interactive media UI remain unverified by fake-process/native builds. Packages
remain unsigned internal testing artifacts. Source/session compatibility, cache
retention, update/rollback and platform limits are documented in SUPPORT.md.
