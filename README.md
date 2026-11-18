# IDM Enhanced

A local Go download manager in development. M0 supplies the desktop shell, quality
gates and browser ping. M2 adds durable CLI downloads and safe resume. M3 adds
parallel ranges, queues, shared limits and bounded retries; acceptance is tracked in
[M3 evidence](docs/M3_EVIDENCE.md). M4 adds desktop download management, history, persisted settings,
one-time schedules and native lifecycle controls; see [M4 evidence](docs/M4_EVIDENCE.md).
M5 adds explicit browser handoff and alpha installable packages; see [M5 evidence](docs/M5_EVIDENCE.md). M6 hardening adds accessible cancellation/progress, recovery guidance and upgrade checks; see [M6 evidence](docs/M6_EVIDENCE.md) and the [support/update guide](docs/SUPPORT.md). Human beta feedback remains pending. M7 adds a bounded HLS VOD profile; see [M7 evidence](docs/M7_EVIDENCE.md). M8 adds bounded static DASH with video/audio selection and offline mux recovery; see [M8 evidence](docs/M8_EVIDENCE.md). [M2 evidence](docs/M2_EVIDENCE.md) records its acceptance.

MIT licensed. See [project scope](PROJECT_PLAN.md), [quality policy](docs/QUALITY_POLICY.md),
[contribution guide](CONTRIBUTING.md) and [platform evidence](docs/M0_EVIDENCE.md).

## Setup

Exact pins: Go 1.27.4, Node 26.13.1, pnpm 12.21.0, Wails library/CLI/runtime 3.0.0.
Install the pinned Go/Node distributions from their official downloads and pnpm with
`npm install --prefix .tools/pnpm pnpm@12.21.0`. Put its `node_modules/.bin` on PATH.
The runner also discovers a local Node distribution under
`.tools/node-v26.13.1-linux-x64/bin` on Linux amd64. Bootstrap does not resolve latest.

Linux Fedora prerequisites:

```sh
sudo dnf install -y gtk4-devel webkitgtk6.0-devel firefox gcc-c++
```

Ubuntu 24.04: `sudo apt-get install libgtk-4-dev libwebkitgtk-6.0-dev build-essential firefox`.
macOS requires Xcode command-line tools; Windows requires the Go native toolchain
and WebView2. Native CI provides evidence separately from interactive support.

```sh
go run ./tools/quality bootstrap
go run ./tools/quality doctor
go run ./tools/quality check
go run ./tools/quality check:full
go run ./tools/quality gate-probes
```

## Durable CLI downloads

```sh
go build -o build/downloader ./cmd/downloader
./build/downloader -o /path/to/new-file -sha256 EXPECTED_HEX https://example.com/file
./build/downloader status
./build/downloader resume -timeout 30m JOB_ID
./build/downloader restart JOB_ID
./build/downloader cancel --keep-partial JOB_ID
./build/downloader cancel --discard-partial JOB_ID
```

Every download persists a job and prints its ID before transferring. State defaults
to `os.UserConfigDir()/idm-enhanced`; override it with `--state-dir DIR` before the
command. Status is read-only and remains available during a transfer. Only one
modifying command can own a state directory; use Ctrl+C to control a live transfer.

Omit `-sha256` when no trusted checksum is available. Options precede the URL or job
ID. `-timeout 30m` bounds a transfer; `-quiet` hides progress. Ctrl+C and SIGTERM
checkpoint and pause, retaining the private partial. Ctrl+C exits 130; timeout
exits 1. Resume requires the same effective URL, strong ETag and valid byte ranges.
An unsafe resume preserves progress and enters `needs_attention`; `restart`
explicitly discards owned progress and requests the original source again.
Cancellation requires an explicit retention flag. Retained cancellations can be
resumed; discarded cancellations require restart. Completed files are never deleted.

Destinations must be new files in an existing trusted directory on a local
filesystem supporting hard links, reliable locks and persistent ownership metadata
(extended attributes on Unix; named streams on Windows). Source URLs remain only in
the protected database; status and diagnostics omit them. Command-line arguments
may be visible to local processes. There is no cookie/header persistence, legacy partial import, or power-loss
resilience claim. Windows directory
flush is currently unsupported. See [M2 evidence](docs/M2_EVIDENCE.md).

## Parallel downloads and queues (M3)

```sh
./build/downloader enqueue -o /path/to/first https://example.com/first
./build/downloader enqueue -o /path/to/second https://example.com/second
./build/downloader reorder SECOND_JOB_ID FIRST_JOB_ID
./build/downloader run-queue -jobs 2 -workers 4 -per-host 4 -rate 1048576 -retries 3
```

Download/resume/restart also accept `-workers`, `-per-host`, `-rate` and `-retries`.
`-rate` is a shared response-body bytes/second cap; zero is unlimited, with at most
32 KiB of burst. Defaults are four workers, two jobs, four requests per hostname
and three retries. Workers range from 1–16, jobs 1–16, host requests 1–64, retries
0–8. Files over 4 MiB use parallel ranges when a strong validator and real range
probe permit them. Other initial downloads stream sequentially. Parallel progress
advances at committed windows; unsafe resume still requires explicit restart.

Enqueue persists a job without transferring. Reorder requires every queued ID
exactly once and commits the order atomically. Run-queue drains a snapshot of queued
jobs; new enqueues wait for the next invocation. Paused/failed jobs require explicit
resume/restart. The CLI's timeout applies to the whole drain; Ctrl+C stops admission
and checkpoints active transfers. One failed job does not stop the others, but the
drain returns failure. At most 1,000 unfinished jobs are admitted by default.

Transient failures retry with exponential backoff and respect Retry-After. Server
waits longer than five minutes require explicit retry. Interrupted no-validator
streams restart during automatic retry; strong-validator progress resumes.
Retry waits persist and are respected by explicit resume after relaunch. A new
explicit invocation starts a fresh bounded retry budget. Set `-retries 0` to disable
automatic retry. There is no automatic startup drain. See
[ADR 0004](docs/decisions/0004-m3-parallel-manager.md) for recovery and limit details.

## Desktop alpha (M4)

```sh
go run ./tools/quality task build
./build/desktop
# Optional isolated state (same layout as the CLI):
./build/desktop --state-dir /path/to/private/state
```

Add a URL and a new destination, then choose **Run queue**. Use **Pause all**
before changing limits, reordering, cancelling or removing jobs during a drain.
Cancellation explicitly chooses whether to retain partial data. Restart discards
owned progress; removing completed history keeps the output. Native Browse,
open-file and reveal-in-folder controls are available. Existing destination files
are rejected; choose another name. Drop a link into the URL field or paste it.

Settings persist in SQLite. One-time schedules use the displayed local timezone
and store UTC instants; overdue schedules run when the app next opens or wakes,
after any active run/drain finishes. Future schedules are excluded from manual
queue draining. Interrupted jobs stay paused for explicit resume. **Pause all**
holds scheduled admission until an explicit run or relaunch. No download runs
while the app is closed, and no system wake alarm is installed.

Closing the window leaves work in the tray. Linux without a tray host minimizes
to the taskbar instead; the app explains which behavior applies. **Quit safely**
checkpoints and joins active work. Native notifications are optional and depend on
OS authorization; unavailable delivery is shown in the app and completion remains
in History. Unsigned macOS builds may lack notification support. M4 migrates the
state database to version 3; older binaries cannot open the upgraded database.

A real native WebView smoke uses a temporary state directory, local HTTP fixture
and a test-only build tag (requires a display; close other app instances first):

```sh
go run ./tools/quality task desktop-smoke
```

## Browser integration and installable alpha (M5)

CI's **Quality** workflow uploads 30-day artifacts for every PR and main build:

- `installable-linux-deb`: Debian 13 amd64 `.deb`.
- `installable-linux-rpm`: Fedora 45 x86_64 `.rpm`.
- `installable-windows-amd64`: per-user Windows setup `.exe` (WebView2 required).

Each includes Firefox unsigned `.xpi`, Chrome `.zip` and `SHA256SUMS`. Download
from the successful Actions run's Artifacts section and verify checksums. These
are unsigned alpha packages, without store publication or signing. Install Linux
with `sudo apt install ./idm-enhanced_0.8.0_amd64.deb` or
`sudo dnf install ./idm-enhanced-0.8.0-1.x86_64.rpm`; launch IDM Enhanced from the
application menu. Windows setup installs for the current user without elevation.

Open the desktop once to register its sibling native host. For Firefox, temporarily
load the unsigned XPI using `about:debugging` → This Firefox → Load Temporary
Add-on (permanent installation in Firefox Release needs Mozilla signing). For
Chrome, extract the ZIP, open `chrome://extensions`, enable Developer mode and
Load unpacked. Chromium also uses that package. Keep the manifest's public `key`
so its ID matches native registration. Default browser installations/profiles are
the current support boundary; sandboxed Flatpak/Snap browsers are unverified.

Right-click an HTTP(S) link and choose **Download with IDM Enhanced**, or enter a
URL in the extension popup. Acknowledgement means the job is durably queued in
`~/Downloads` (Windows: the user profile's Downloads directory). Open the desktop
and choose Run queue. Files are never overwritten; queued case-equivalent and Unicode-equivalent names
are reserved consistently across platforms, with numbered browser collision names.
The extension saves the latest request and shows context-menu results through its
badge/popup. **Retry last handoff** reuses its request ID, including after a lost
acknowledgement or app restart. Do not submit a new request to retry the old one.
Only the latest request has a retry slot. That slot retains its URL (including any
signed query) in local browser extension storage until replaced.

The app must be running. Missing/closed apps explain how to open and retry; the
browser link remains usable. Cookies/session credentials, POST, blob/data URLs,
automatic interception and media manifests are unsupported. Use the browser for
those cases. Receipts persist after history removal to prevent late duplicates;
this alpha caps them at 10,000 per state directory and then rejects new handoffs.

Repair or remove user registration using the installed host:

```sh
# Linux (run as your normal user):
/usr/lib/idm-enhanced/native-host --register
/usr/lib/idm-enhanced/native-host --unregister
# Development checkout, after task build:
go run ./tools/quality task register
go run ./tools/quality task unregister
```

Windows: run `native-host.exe --register`/`--unregister` in the installation folder;
the uninstaller unregisters automatically. On Linux, run `--unregister` before
removing the package; system package removal cannot safely alter every user's
home. Uninstall retains downloads and application state. macOS supports native
builds and user registration, but no installer is supplied in M5.

Build packages after `task build` with `PACKAGE_FORMAT=deb` or `rpm` followed by
`go run ./tools/quality task package`. Windows uses `PACKAGE_FORMAT=windows` and
Inno Setup 6.5.4. Packaging explicitly requires the corresponding system tools;
ordinary checks do not install them. Package assembly also emits both extension
archives. Linux browser smoke launches an isolated real desktop and disposable
Firefox/Chromium profiles, queues deterministic links and verifies durable IDs:

```sh
go run ./tools/quality task browser-smoke
```

Live IPC keeps the existing five-second/64-KiB bounds. Unix uses a private 0700
directory and 0600 socket; Windows uses a user-SID-restricted pipe. A crash may
leave a stale Unix socket: verify no app is running before removing
`$XDG_RUNTIME_DIR/idm-enhanced/desktop.sock` (or the user-cache equivalent).

## Agent desktop tools on Linux

This workstation also has `@agent-sh/computer-use-linux` 0.7.55 installed outside
the repository under `~/.local/share/codex-desktop-control`, with the
`computer-use-linux` stdio MCP registered in the personal Codex profile. It exposes
19 desktop tools, including screenshot, click, keyboard, accessibility and window
operations. AT-SPI is enabled; GNOME must load the installed window-control
extension after logout/login, and Codex must reload the MCP configuration. Screen
capture/input remain subject to the system's portal consent. Capture and input
are not claimed verified merely because the server registered successfully.

This is development workstation tooling, not an application dependency or required
CI gate. Its binaries were installed at the exact npm version and checked against
upstream SHA-256 release files. Source/license: [computer-use-linux (MIT)](https://github.com/agent-sh/computer-use-linux).

## HLS VOD media (M7)

Install FFmpeg and ffprobe separately (Ubuntu: `sudo apt-get install ffmpeg`).
They are not bundled; ordinary file downloads work without them. On the desktop,
select **HLS VOD media (MP4)**, enter a playlist URL and new `.mp4` destination,
choose **Analyze HLS**, select quality and add/run the queue. Empty selection
uses highest declared bandwidth. Progress counts complete segments separately
from output bytes. Complete objects remain for offline assembly retry until
history removal; the completed MP4 is kept.

```sh
./build/downloader media-info https://example.com/master.m3u8
./build/downloader -hls -quality VARIANT_ID -o /path/to/new.mp4 https://example.com/master.m3u8
./build/downloader enqueue -hls -o /path/to/new.mp4 https://example.com/master.m3u8
go run ./tools/quality task media-smoke
```

Omit `-quality` for the highest bandwidth or a direct media playlist; copy a
variant ID exactly from media-info to select it. Standard resume/restart/cancel
commands keep manager ownership. FFmpeg installation or assembly failure retains
complete objects: **Resume / retry** can assemble without the original server.

In the extension use **Find media in this tab**, choose a candidate, then queue
it explicitly, or choose HLS mode for a pasted URL/link context menu. Discovery
uses temporary activeTab/scripting access to the main frame and recent resource
URLs; it does not read cookies or intercept transfers. Candidate detection does
not prove source support. Browser HLS uses highest declared bandwidth and the
manager chooses an MP4 name; use desktop analysis for explicit quality.

The initial profile requires HLS versions 1–3, explicit VOD plus ENDLIST and
MPEG-TS objects with H.264/AAC. Live/event/untyped, encrypted/DRM, fMP4 input,
byte-range, discontinuity and alternate-audio playlists are rejected. Keep
unsupported or session-authenticated sources in the browser. See the exact
[support and recovery matrix](docs/SUPPORT.md). Full checks require the real media
tools; ordinary checks do not install them or silently substitute fake playback.

## Static DASH VOD (M8)

Select DASH VOD in the desktop, analyze a static MPD, choose the video/audio
combination and save to a new MP4. The extension offers explicit DASH handoff and
opt-in `.mpd` candidates; handoff queues the highest-bandwidth combination.

```sh
go run ./cmd/downloader media-info https://example.com/manifest.mpd
go run ./cmd/downloader -dash -quality CHOICE_ID -o video.mp4 https://example.com/manifest.mpd
```

The supported profile is single-Period, unencrypted H.264/AAC fragmented MP4 with
SegmentTemplate initialization and finite duration/timeline addressing. Requires
separately installed FFmpeg and ffprobe. Completed objects can assemble offline;
failed or interrupted jobs use the existing Resume / retry workflow. Database
schema 6 prevents older binaries from opening new DASH state; back up before
upgrading. See [support matrix](docs/SUPPORT.md), [M8 evidence](docs/M8_EVIDENCE.md)
and [ADR 0008](docs/decisions/0008-m8-dash-vod.md). Human beta testing remains pending.
