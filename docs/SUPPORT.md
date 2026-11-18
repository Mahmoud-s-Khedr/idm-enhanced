# Core testing support matrix and recovery guide

Updated: 2026-11-18. Version 0.8.0 is an unsigned internal testing candidate.
It is not a public R1 release. Beta-user acceptance is pending; the owner will
send the candidate to testers later. No automatic update channel is provided.

## Platform and source boundary

| Target           | Automated evidence                                                                                                                    | Interactive evidence / limitations                                                                                                             |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Debian 13 amd64  | Native DEB installation, controlled version upgrade, executable dependencies, registration/removal and state retention in required CI | Debian desktop/browser interaction has not been established                                                                                    |
| Fedora 45 x86_64 | Native RPM installation, controlled version upgrade and state retention in required CI; local core, recovery and native WebView smoke | Local Firefox 160.0 and Chromium 157.0.8103.78 explicit handoff; notifications depend on desktop permissions                                   |
| Windows amd64    | windows-2025 native tests, per-user Inno setup, controlled version upgrade, registration/uninstall and state retention in required CI | WebView2 required; interactive UI/browser and physical sleep/wake testing remain pending; unsigned setup may trigger reputation prompts        |
| macOS arm64      | macos-15 native builds, recovery and IPC fixtures in required CI                                                                      | Source build only; no packaged installer, signing/notarization or current interactive browser claim; unsigned notifications can be unavailable |

The CI workflow must pass on the exact PR head before merge. Historical platform
results do not establish newer heads. Older operating systems, other architectures,
network/removable filesystems and Flatpak/Snap browsers are unverified. Local
filesystems must support hard links, reliable locks and ownership metadata (Unix
extended attributes or Windows named streams). Windows directory durability and
power-loss resilience are not established.

Supported sources are direct HTTP(S) GET URLs, including directly usable MP4/WebM
file URLs. Strong ETags and validated byte ranges are required for safe resume;
initial downloads without them can stream sequentially. Credentials embedded in
URLs are rejected. Browser cookies, authorization replay, POST, blob/data URLs,
unsupported DASH profiles, encrypted media, DRM and automatic interception are unsupported. Keep those
workflows in the browser. Browser admission acknowledges a durable queued job;
choose Run queue in the running app to start it.

## HLS VOD

Choose HLS VOD in Add download, analyze the URL, select a quality, and save as
`.mp4`. The extension can explicitly queue an HLS URL at the default highest
quality. Find media candidates inspects the active tab only after a click; it
never queues automatically. Candidate detection does not establish compatibility.

The supported profile is explicit, unencrypted HLS VOD versions 1–3 with complete
MPEG-TS segments, H.264 video and/or AAC-LC audio. Live/event streams, alternate
audio, byte ranges, discontinuities, fragmented MP4 input and unknown extensions
are rejected. Install both `ffmpeg` and `ffprobe` on PATH for assembly; they are
optional external tools and are not bundled. See M7_EVIDENCE.md for limits.

Objects are verified and cached separately from the final MP4. Resume validates
cached objects before reuse. Once all objects are saved, missing-tool or assembly
failures can be retried offline after correcting the tool installation. Completed
caches remain until history removal; cancellation can keep or discard owned
progress. Unproven files are preserved for inspection. A successful download
requires verified assembly and safe destination publication.

Real local MP4 assembly/playback, desktop quality selection and both browsers'
HLS handoff have been exercised. The discovery permission dialog and real media
playback on Windows/macOS have not been exercised interactively.

## Static DASH VOD

Choose DASH VOD in Add download, analyze the MPD URL and select a video/audio
combination, then save as `.mp4`. CLI uses `media-info URL` and `-dash -quality ID`.
Browser popup/context-menu DASH handoff uses the highest combined bandwidth.

| DASH feature             | Supported boundary                                                                                                                  |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| Presentation             | Explicit static MPD, standard namespace, one Period at zero, finite duration up to 24 hours                                         |
| Video/audio              | H.264 AVC / AAC-LC fragmented MP4, one selected representation per track; audio-only/video-only supported                           |
| Selection                | Explicit video/audio combinations, including alternate audio languages                                                              |
| Addressing               | SegmentTemplate with mandatory initialization; Number/Time/RepresentationID/Bandwidth substitutions                                 |
| Timing                   | Fixed duration or contiguous zero-based SegmentTimeline; finite bounded repeats including r=-1                                      |
| URL/template inheritance | Single BaseURL per level and inherited SegmentTemplate attributes/timeline                                                          |
| Unsupported              | Live/dynamic, DRM/encryption, multiple Periods, SegmentBase/SegmentList/ranges, nonzero offsets, gaps, subtitles, unknown semantics |

The same object validation and offline retry policies as HLS apply. Both `ffmpeg`
and `ffprobe` must be on PATH. DASH temporarily uses additional local space for
seekable track copies; ordinary cleanup removes staging. A process kill during
muxing can leave a private `.idm-mux-*` directory beside the output for inspection.
Cached resources remain until explicit discard or history removal. Existing
partial ownership and exclusive publication protections remain in force.

## Safe recovery

- Pause or Quit safely before maintenance. Closing the window leaves the app in
  the tray, or minimizes where no tray is available. Downloads stop when the app
  quits. Interrupted jobs require explicit resume; overdue one-time queued
  schedules can start when the app reopens. No system wake alarm exists.
- Use Resume / retry for ordinary interruptions. The engine verifies file and
  resource identity before reusing bytes. A changed source, invalid range response
  or checksum failure must never be forced into an append operation.
- Read the per-job recovery guidance. Restart discards owned saved progress and
  requests the original source again. A wrong expected checksum or expired URL
  may require a new download with corrected input. Authentication tied to the
  browser should stay in the browser.
- For missing or replaced partial files, preserve files whose ownership cannot
  be verified and create a download with a new destination. Do not manually edit
  SQLite checkpoints, ownership attributes or partial-file contents.
- Existing destinations are never overwritten. Remove history deletes metadata,
  not completed files. Cancellation explicitly keeps or discards owned partials.
- If browser acknowledgement is lost, use Retry last handoff with its original
  request ID. The latest URL, including any signed query, remains in local
  extension storage until replaced. Receipts are capped at 10,000 per state
  directory and remain after history removal; do not delete the database to
  work around that limit while relying on duplicate protection.

## Manual update and rollback

Obtain the three installer bundles from the successful Quality Actions run linked
by the PR. Check the run's commit and every SHA256SUMS entry before installing.
Checksums detect damage; they do not replace publisher signing. These unsigned
packages and developer-loaded extensions are intended for controlled testing.
No store publication or public installer promotion is authorized by a green CI run.

Quit safely, then back up the entire private state directory and any partial
files (preserving ownership metadata) before updating. State defaults to
`os.UserConfigDir()/idm-enhanced`; keep CLI and desktop pointed at the same state.
Install the newer package over the existing installation, open the desktop and
verify the queue and native-host registration. Linux uses the DEB/RPM package
manager; Windows uses the per-user setup. The workflow tests upgrade mechanics
using a controlled 0.5.0 predecessor package assembled from current sources and
then 0.8.0. It does not claim an archived M5-binary upgrade test. Released schema
migration compatibility is covered separately by storage fixtures. M8 advances the database to schema 6 as a DASH compatibility barrier; file checkpoint v1 and HLS media checkpoint v1 are retained. Back up before this migration.

Uninstall preserves downloads and application state. On Linux run the installed
native-host --unregister as the normal user before removing the package. Windows
uninstall removes its own native registrations. Older binaries refuse databases
with newer schemas; rollback requires restoring a compatible pre-upgrade backup
and matching owned partials. Do not downgrade only the database version number.

## Remaining R1 acceptance

Human beta workflows, Windows/macOS interactive checks, screen-reader and physical
power-event tests, public product naming/signing and release-channel decisions
remain open. Speed/ETA, batch import and richer category presentation are still
R1 feature follow-ups from M4. No fixed parallel speedup is claimed. These are
visible release limitations, not silently waived acceptance gates.
