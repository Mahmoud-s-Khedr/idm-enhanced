# Core testing support matrix and recovery guide

Updated: 2026-11-18. Version 0.6.0 is an unsigned internal testing candidate.
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
HLS/DASH manifests, DRM and automatic interception are unsupported. Keep those
workflows in the browser. Browser admission acknowledges a durable queued job;
choose Run queue in the running app to start it.

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
manager; Windows uses the per-user setup. The M6 workflow tests upgrade mechanics
using a controlled 0.5.0 predecessor package assembled from current sources and
then 0.6.0. It does not claim an archived M5-binary upgrade test. Released schema
migration compatibility is covered separately by storage fixtures; M6 keeps
schema 4 and checkpoint v1 unchanged.

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
