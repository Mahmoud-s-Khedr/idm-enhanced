# M5: Browser integration and alpha installable artifacts

Updated: 2026-11-18. Implementation is in progress; final checks and protected
merge are required before acceptance.

M4 PR #6 merged as `02e7fb3` after final-head Quality run 44288012297 passed
quality, Linux/Windows/macOS native jobs and the strict protected aggregate.

## Delivered scope

Explicit Firefox/Chrome link context menu and popup handoff, durable same-request
retry, safe destination naming, independent per-user native-host installation,
Linux/macOS Unix sockets, Windows SID-restricted named pipe, and CI-produced
Debian/Fedora/Windows installers and extension packages. See
[ADR 0006](decisions/0006-m5-browser-distribution.md) for policies, dependencies,
installation ownership, supported inputs and distribution limits.

## Validation status

The following passed locally on Fedora 45 amd64 with pinned tools:

- `go run ./tools/quality doctor`
- `go run ./tools/quality check`
- `go run ./tools/quality check:full` (including race, integration, secrets,
  host-target and Windows-target vulnerability scans)
- `go run ./tools/quality gate-probes` (20 rejection/integrity checks)
- `go run ./tools/quality task desktop-smoke` (real native WebView/Wails)
- `go run ./tools/quality task browser-smoke` (isolated production desktop,
  Firefox 160.0 and Chromium 157.0.8103.78, actual native-message handoff/retry,
  exactly two durable queued records)
- `go test -tags=integration -count=1 -run TestNativeHostDownloadHandoff ./integration`
- `git diff --check`

Statement coverage: engine 96.1%, manager 92.9%, storage 92.1%, desktop 93.2%,
protocol 96.6%, IPC 87.5%, registration 81.0%. All handwritten frontend modules
meet unchanged per-file thresholds. Desktop assets remain 258,792 raw / 83,314
gzip bytes; extension assets are 6,152 raw / 2,919 gzip bytes per browser, within
unchanged budgets. Gitleaks and host/Windows-target govulncheck found no findings.
The existing reviewed development-only picomatch applicability exception remains
visible and expires 2026-12-01; no thresholds or exclusions were weakened.

New deterministic fixtures cover atomic receipt rollback, schema-3 upgrade,
receipt retention after history removal, request conflicts, concurrent duplicate
admission, filename/collision bounds, closed/sleeping sessions, and native-host
process handoff through the real manager to exact verified output. Critical
M2/M3 recovery tests remain active. Migration fixtures remove newer tables when
reconstructing released schemas; future-version rejection advances to schema 5.

Initial hosted run 44296154157 exposed Git checkout trust for Go VCS stamping in
root-run distro containers. CI now explicitly trusts only its mounted workspace
and diagnoses Git status, keeping VCS stamping enabled. Native package/installer
results and final-head protected checks remain required before acceptance.

## Support boundary

CI targets Debian 13 amd64, Fedora 45 x86_64 and Windows amd64 installers.
macOS remains a native build/IPC target without an installer in this stage.
Live browser versions and platform checks must be distinguished from manifest/API
compatibility and native process tests. Firefox packages are unsigned and require
temporary loading until Mozilla signing; Chrome ZIP is loaded unpacked in developer
mode. Browser stores, code signing, older OS versions, Flatpak/Snap browsers,
physical power-event delivery and power-loss resilience are not established.
