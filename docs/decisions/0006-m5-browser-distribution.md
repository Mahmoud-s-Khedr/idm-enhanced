# ADR 0006: Explicit browser handoff and alpha distribution

Date: 2026-11-18

Status: Implemented; final acceptance requires recorded checks and protected merge.

## Ownership and durable admission

The version-1 native envelope adds `download` (HTTP(S) URL and portable filename)
and `accepted` (job ID). Ping remains compatible. The Go protocol owns the DTO;
the existing contract generator produces TypeScript. Incoming message size remains
64 KiB, requests time out at five seconds and the desktop bridge admits at most
eight connections. Requests reject credentials, fragments, unsupported schemes,
unsafe filenames and arbitrary destination paths. The browser strips fragments
and replaces unsafe suggested-name characters; Go independently validates input.

Native hosts forward to the one running desktop process. They never instantiate
an engine or database owner. The desktop queues to the user's `Downloads` folder;
the manager owns bounded collision naming, queue admission and destination checks.
Acceptance means durably queued, not transfer completion or automatic start. The
user opens the desktop and runs the queue. No browser download is intercepted or
cancelled. Closed applications produce actionable open-and-retry feedback instead
of an uncontrolled auto-launch. Existing recovery and publication policies apply.

Migration 004 adds bounded durable receipts, committing the receipt and queued job
in the same SQLite transaction. Request IDs bind to the SHA-256 of the exact source
and filename. Same-ID/same-input retries return the original ID after restart or
metadata removal; changed input is rejected. Receipts contain no source URL. They
are retained without expiry to prevent late duplicate messages from recreating
removed downloads; this alpha admits at most 10,000 receipts per state directory.
Reaching that limit rejects new handoffs explicitly. Receipt lifecycle management
is a later product decision. Old migrations and v1 checkpoints are unchanged;
older binaries reject schema 4. Migration fixtures reconstruct the released schema
by removing tables introduced by later migrations before resetting user_version.

The extensions share all behavior. Browser-specific files are small MV3 manifests:
Firefox module event scripts and Chrome module service worker. Permissions are
nativeMessaging, contextMenus and storage, without cookies, request observation,
page injection or broad host access. Explicit link context menus and a popup both
queue direct URLs. The latest request is saved before sending and can be retried
from the popup with its original ID after a lost acknowledgement. A badge directs
context-menu users to the popup for the saved outcome. Browser storage holds the
latest URL, potentially including a signed query, until replaced; it is never
logged or sent to an external service. This is not a multi-request retry inbox. Shared Web Locks serialize persistence
across popup and background contexts. A reply can update only its own current
request, and a late failure cannot erase a known durable acknowledgement. Locks
are released before waiting for native messaging; no browser permission or new
library is added.
Authenticated browser sessions, POST, blob/data URLs, automatic interception and
media manifests remain unsupported; keep those downloads in the browser.

## Local transport and registration

Linux and macOS use a private, user-owned 0700 directory and 0600 Unix socket.
Windows uses a per-user SID named pipe with an explicit user-only DACL and remote
clients rejected by go-winio. Both share existing deadlines/framing/cancellation.
There is no network listener. Same-user local processes remain in the trust
boundary. Stale Unix sockets after a crash require verifying the app is stopped
before removing the socket; a second process never replaces a live socket.

Native host `--register`/`--unregister` works independently of the source checkout
for Firefox, Google Chrome and Chromium. Windows uses HKCU registry entries;
macOS/Linux use documented per-user paths. Canonical identities remain in
extensions/identities.json, embedded into the registration executable. A different
executable's registration is not overwritten/removed. Desktop startup attempts
registration for its sibling native host; explicit repair commands report failures.
Default browser profiles are the tested path; sandboxed Flatpak/Snap browser host
visibility and custom browser configuration locations are not established.

## Dependency decision

Add github.com/Microsoft/go-winio v0.6.2 for Windows-only named pipes. The standard
net package does not supply a Windows named-pipe listener with cancellation and
security descriptors; implementing overlapped IO ourselves would add substantial
platform code. Microsoft's library is MIT licensed, maintained upstream, and uses
the existing golang.org/x/sys dependency. It adds one module, no new independent
transitive modules, and no Linux/macOS runtime code. Its Windows footprint is
included in the native binary/installer artifacts. The Windows-target vulnerability
scan is required alongside the host-target scan. Add golang.org/x/text v0.44.0 for NFC normalization in manager destination
reservations, alongside Unicode case folding. The standard library has case
folding but no canonical Unicode normalization; hand-maintained mappings or new
platform-specific C APIs would be incomplete or add unnecessary platform code.
This Go-maintained BSD-3-Clause module adds about 123 KiB (0.5%) to the measured
Linux desktop executable: 24,609,704 to 24,732,688 bytes. Its graph advances the
existing test/tool-only x/sync dependency from v0.24.0 to v0.25.0; no additional
application runtime module is introduced beyond x/text. Frontend assets are
unaffected. Reservations intentionally treat canonical/case equivalents as
collisions on all filesystems, including case-sensitive Linux, to provide a
consistent portable policy. Direct CLI/desktop admission uses the same manager
comparison as browser naming. Existing files still use filesystem existence
checks and exclusive finalization. No existing dependency pin is otherwise changed.

## Installers and CI

The user's M5 scope explicitly brings initial Linux and Windows installation
forward from M6. Native Debian 13 amd64 and Fedora 45 x86_64 container builds create
.deb/.rpm packages, with GTK4/WebKitGTK6 runtime dependencies, application-menu
entry, desktop/CLI/native host binaries, and license. RPM retains automatic ELF
requirements. Windows amd64 creates a per-user Inno Setup 6.5.4 installer, shortcuts
and native registrations. Windows requires WebView2. Inno is a build-only GPL tool
with its documented installer-output exception; it is not a new application
runtime library. Packages are alpha, unsigned and not a claim of older OS support.
macOS native builds and IPC tests remain required; a macOS installer is not added.

The canonical package task invokes the standard-library Go assembler, system
package tools, and the explicitly installed pinned Windows compiler. Every PR and
main CI build uploads installers, Firefox unsigned XPI, Chrome ZIP and SHA256SUMS
as 30-day Actions artifacts. No store publication or release credentials are
introduced. Firefox Release requires Mozilla signing for permanent installation;
the unsigned XPI is for temporary development loading. Chrome ZIP is unpacked
through developer mode, preserving its checked-in public identity key. A public
store identity requires a deliberate manifest/registration update if it differs.

The Required checks aggregate now requires both Linux package jobs and the native
matrix (including Windows setup smoke), as well as quality. Controlled probes and
aggregate tests cover failures/skips of the added job. Installation smoke runs in
disposable CI systems and verifies registration, removal, executable dependencies
and user-data retention. It does not establish Windows interactive browser/UI
support, signing, OS reputation, or every distribution version.

## References

- [Chrome native messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging)
- [Firefox native manifests](https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_manifests)
- [Firefox background modules](https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Background_scripts)
- [go-winio source and license](https://github.com/microsoft/go-winio/tree/v0.6.2)
- [Inno Setup documentation](https://jrsoftware.org/ishelp/)

- [Web Locks across same-origin windows and workers](https://developer.mozilla.org/en-US/docs/Web/API/Web_Locks_API)

- [Go Unicode normalization](https://pkg.go.dev/golang.org/x/text/unicode/norm)
