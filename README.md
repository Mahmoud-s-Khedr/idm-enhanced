# IDM Enhanced

A local Go download manager in development. M0 establishes a Wails desktop shell,
repository quality gates and a browser-to-desktop ping. Downloading, queues,
persistence, media processing and installers are later milestones.

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
