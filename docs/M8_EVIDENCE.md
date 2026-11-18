# M8: Bounded static DASH expansion

Updated: 2026-11-19. Implementation and local validation are complete; [PR #10](https://github.com/Mahmoud-s-Khedr/idm-enhanced/pull/10) records final-head hosted checks, artifact verification and protected merge for engineering acceptance. Human beta workflows are deferred
by the owner until after M8 and are not claimed by automated evidence.

M7 protected acceptance was read back from PR #9: merged as
5c14ec3489c61dadd7b697361731148f3c7f097a after corrected head
2c24de51223a9ef31a947f3a3de69b34f0265e06 passed Quality run 44407355708.

## Delivered behavior

Static, single-Period DASH analysis and explicit quality/audio combination choices;
mandatory initialization objects; SegmentTemplate Number/Time addressing with
fixed duration or bounded contiguous SegmentTimeline, including finite r=-1;
base/template inheritance and safe URL resolution. Desktop, CLI -dash/-quality
and explicit Firefox/Chromium popup/context-menu handoff use the existing manager.
Opt-in current-tab candidates recognize .mpd, without cookies or interception.

DASH checkpoint v2 preserves ordered track partitions independently of file
checkpoint v1 and HLS media checkpoint v1. Schema 6 prevents older binaries from
misinterpreting new jobs. Verified caches resume safely and mux offline after tool
or assembly failures. FFmpeg receives only private local staged tracks, with
file-only MOV demuxing and explicit stream-copy maps; verified publication remains
the existing manager finalizer. See ADR 0008 and SUPPORT.md for the full matrix.

## Evidence collected during implementation

- Deterministic MPD fixtures cover addressing, inherited templates/base URLs,
  audio-only/video-only, selection, negative repeats, malformed XML, unsupported
  encryption/live/multi-period/ranges, unsafe URLs and bounded expansion.
- Four checked-in synthetic video/audio combinations download only selected
  initialization resources, retain six complete objects with tools absent,
  reopen persisted state with the origin offline, mux with real FFmpeg/ffprobe,
  produce the selected resolution plus AAC, fully decode and preserve output
  after history/cache removal.
- DASH subprocesses are killed and relaunched at all nine existing media/object/
  assembly/publication boundaries, producing exact expected bytes. The existing
  HLS and sequential/parallel file crash suites remain active.
- Manager fixtures cover failed fragments, retained initialization, resume,
  offline retry after reopen, invalid track partitions, cleanup and browser
  receipt deduplication/conflicts. Storage fixtures cover version-5 upgrade,
  transactional migration rollback and preserved HLS/file/receipt data.
- Frontend/extension fixtures exercise DASH analysis/edit locks/selected admission,
  explicit handoff and same-request retry, alongside existing HLS workflows.

## Local validation

The following passed on Fedora 45 amd64 with pinned tools:

- `go run ./tools/quality doctor`
- `go run ./tools/quality check`
- `go run ./tools/quality gate-probes` (all 20 recorded rejection/integrity checks, including staged-index preservation)
- `go run ./tools/quality check:full` (ordinary checks, race, integration, real
  FFmpeg/ffprobe 9.0.2 media decode, secrets, host/Windows vulnerability scans and
  JavaScript dependency audit)
- `go run ./tools/quality task desktop-smoke` (actual native WebView file/HLS/DASH
  workflows, selected quality/audio, MP4 output, cache cleanup and lifecycle)
- `go run ./tools/quality task browser-smoke` (Firefox 160.0 and Chromium
  157.0.8103.78 explicit file/HLS/DASH handoff/retry, exactly six durable jobs)
- `go test ./internal/manager -run DASH -count=1`
- `go test ./internal/media -run '^$' -fuzz '^FuzzDASHManifest$' -fuzztime=10s -parallel=2`
  (109,127 executions, no failures)
- `git diff --check`

Critical Go coverage: engine 96.1%, manager 91.3%, storage 92.3%, media 92.7%.
The 48 TS tests pass; statements 98.23%, branches 92.39%, functions 99.05%,
lines 99.18%; unchanged per-file gates also pass. Desktop assets: raw 262,742 /
303,185 bytes, gzip 84,550 / 103,429. Each extension: raw 9,271 / 11,195,
gzip 4,260 / 9,642. Scans report no secrets, Go vulnerabilities or high/critical
JavaScript findings. Existing reviewed dependency applicability policy remains.

The first expanded desktop smoke timed out while checking for the new queued
DASH job because its previous HLS workflow left the UI on History. The harness
now explicitly returns to All downloads for each format. The actual native
workflow passed after that correction; no assertion or production behavior was
weakened. Initial extension coverage required exercising the new DASH handoff
branch; explicit same-request retry coverage was added and all gates passed.

Final protected checks, artifact verification and merge are tracked with [PR #10](https://github.com/Mahmoud-s-Khedr/idm-enhanced/pull/10). Existing
thresholds, budgets, required jobs and dependencies are unchanged.

## Acceptance limits

This is a defined static DASH subset, not general DASH or site extraction.
Subtitles, live/dynamic, DRM/encryption, multiple Periods, ranges and nonzero
presentation offsets remain unsupported. Tools remain separately installed;
packages remain unsigned internal candidates. Local Linux real playback does
not establish Windows/macOS interactive playback. Beta-user feedback, physical
power events, signing/naming and the public R1 checklist remain pending.

## Review correction

Automated review identified a fractional-duration boundary error: binary floating
point could turn PT1.1S at timescale 100 into 111 ticks, requesting an extra
fragment or rejecting a complete finite timeline. DASH durations now use bounded
exact rational arithmetic through the integer tick ceiling. Regressions cover
fixed-duration, finite and r=-1 timelines, exact decimal boundaries, nanosecond
precision and exact upward tick calculation. Ends requiring clipping are now explicitly rejected by the profile, as explained in the presentation-boundary correction below.
The float summary no longer determines addressing. Corrected-head local `check:full` passed again. Hosted required checks must pass before merge; an earlier green head is not inherited as evidence.

## Hosted retry-fixture correction

Run 44420698683 passed native Linux/macOS and Debian/Fedora installer checks, but
Windows exposed an existing M3 timing assumption in TestPersistentQueueAndRetryWait.
The fixture set a deadline 80 ms ahead before SQLite sync/recovery, then demanded
at least 50 ms of delay measured only after that setup. Slow native persistence
could correctly consume the deadline before Run, causing a false failure.
The fixture now requires the exact RetryAt to survive recovery and records the
actual HTTP request timestamp, rejecting any request before the saved deadline.
It also requires the request to reach the fixture. Transfer/retry policy and
coverage thresholds are unchanged. Corrected-head validation remains mandatory.

## Presentation-boundary review correction

A second automated review identified that complete terminal fragments could extend
past the MPD Period, while the assembly boundary did not receive that duration.
The initial profile now requires exact terminal alignment in every track and
rejects overlaps/non-integral Period ticks that require clipping. Declared timing
and actual encoded track durations are independently checked. The manager carries
Period duration into muxing; bounded ffprobe validation checks each staged input
and the final output to one microsecond of reported duration precision.

Real validation also exposed AAC priming being shifted into fragmented pipe output,
lengthening a two-second fixture by about 23 ms. DASH now stream-copies to a private
seekable MP4 with edit lists and microsecond movie timescale, preserving the exact
two-second presentation. Go copies the bounded temporary output into the existing
owned finalization path. HLS keeps its existing pipe assembly. Cancellation and
size checks cover the extra copy; temporary disk costs are documented in SUPPORT.md.
Regressions reject both a 1.5-second declared terminal overlap and apparently aligned
one-second timing backed by two-second encoded fragments, with no published output.
All four real selected-track combinations must still mux offline and fully decode.
