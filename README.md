# IDM Enhanced

A local Go download manager in development. M0 supplies the desktop shell, quality
gates and browser ping. M2 adds durable sequential CLI downloads and safe resume. Queues, full desktop
workflows, media and installers remain later milestones. M2 hosted acceptance
is pending; see [M2 evidence](docs/M2_EVIDENCE.md).

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

## Durable CLI downloads (M2)

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
filesystem supporting hard links and reliable locks. Source URLs remain only in
the protected database; status and diagnostics omit them. Command-line arguments
may be visible to local processes. There is no cookie/header persistence, automatic
retry, legacy partial import, or power-loss resilience claim. Windows directory
flush is currently unsupported. See [M2 evidence](docs/M2_EVIDENCE.md).

## Run the platform spike

```sh
go run ./tools/quality task build
./build/desktop
```

Leave the app running; select **Test backend connection** to call Go. For Linux
browser connections, in another terminal:

```sh
go run ./tools/quality task register
```

Load `extensions/chromium/dist` as an unpacked extension from `chrome://extensions`
with developer mode enabled. Firefox: `about:debugging` → This Firefox → Load
Temporary Add-on → `extensions/firefox/dist/manifest.json`. Open the extension's
popup and choose **Test desktop connection**. The app shows the matching request ID.
Development identities are fixed in `extensions/identities.json`; Chromium's checked
in key is public only. No browser profile or signed extension is installed by setup.

Registration writes only user manifests for Firefox, Chromium and Google Chrome,
with an absolute native-host path and explicit allowed extension identity. Cleanup:

```sh
go run ./tools/quality task unregister
```

The app must already be running. Errors instruct you to open it and retry. Messages
are capped at 64 KiB; waits at five seconds. The Linux IPC directory must be private
0700, socket 0600. A crash can leave a stale socket: verify no app is running before
removing `$XDG_RUNTIME_DIR/idm-enhanced/desktop.sock` (or the user-cache equivalent).
Windows/macOS live IPC and registration are deferred to M5; desktop/framing builds
are required in native CI. Signing and installers are not part of this spike.

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
