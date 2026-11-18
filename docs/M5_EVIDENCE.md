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

Final local and hosted evidence will be recorded here before protected merge.
Required commands are doctor, check, check:full, gate-probes, real desktop smoke,
real Linux browser smoke and native/package CI. New deterministic fixtures cover
atomic receipt rollback, schema-3 upgrade, receipt retention after history removal,
request conflicts, concurrent duplicate admission, filename/collision bounds,
closed/sleeping sessions, and native-host process handoff through the real manager
and final output. Critical M2/M3 recovery tests remain active.

## Support boundary

CI targets Debian 13 amd64, Fedora 45 x86_64 and Windows amd64 installers.
macOS remains a native build/IPC target without an installer in this stage.
Live browser versions and platform checks must be distinguished from manifest/API
compatibility and native process tests. Firefox packages are unsigned and require
temporary loading until Mozilla signing; Chrome ZIP is loaded unpacked in developer
mode. Browser stores, code signing, older OS versions, Flatpak/Snap browsers,
physical power-event delivery and power-loss resilience are not established.
