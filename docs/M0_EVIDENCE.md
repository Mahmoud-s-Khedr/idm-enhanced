# M0 evidence

Updated: 2026-11-17 (Africa/Cairo). M0 implementation is under final validation.
Do not interpret pending hosted results as passing checks.

## Local environment

Linux amd64, Fedora 45. Go 1.27.4 (Fedora build reports nodwarf5 experiment),
Node 26.13.1, pnpm 12.21.0, Wails CLI/library/runtime 3.0.0.
GTK4 4.24.1 and WebKitGTK 6 API implementation 2.54.2.
Firefox 160.0 and Chromium 157.0.8103.78.

The user installed Linux prerequisites with dnf. The native build succeeds with
upstream GTK deprecation warnings. The desktop process served its embedded JS/CSS
and created a mode-0600 socket in a mode-0700 directory. The user confirmed that
the window rendered and the backend button worked (“it works”). This is
user-observed interactive evidence, not an automated screenshot assertion.

## Browser acknowledgement evidence

`go run ./tools/quality task browser-smoke` uses temporary browser profiles and
real installed extension popups, forwards through the built native host into the
running native Wails app, clicks Test desktop connection, and reads the response.
The script records actual browser versions and matching request IDs in
`build/browser-smoke.json`. Successful run at 2026-11-16T22:45:24Z:

| Browser                | Response                                                  |
| ---------------------- | --------------------------------------------------------- |
| Firefox 160.0          | Desktop acknowledged 057be27f-6a5a-470e-9886-9dac8a839bd7 |
| Chromium 157.0.8103.78 | Desktop acknowledged 7381218f-a1d1-41d6-a2b1-dabd140920dc |

This browser test is headless. It does not claim manual extension installation
or interactive popup review in the user's normal browser profile. Chromium
custom-profile tests require the same native-host manifest copied into that
profile's NativeMessagingHosts directory. Default-profile registration remains
in the documented user configuration locations.

The optional Linux smoke harness needs `zip` and geckodriver 0.37.1 (Mozilla
release) at `.tools/bin/geckodriver`. It uses Node's built-in WebSocket/fetch for
Chromium, with no additional application dependency. Firefox's test process
explicitly enables remote system access to obtain the temporary add-on URL; this
setting is scoped to that disposable driver process. It never changes normal
browser preferences.

## Bundle baselines and committed limits

Each limit is ceil(baseline × 1.2) + 8192 bytes; gzip uses Go's standard gzip writer.
All emitted JS/CSS, including lazily emitted chunks, is counted. Each current
bundle has a single JS entry (desktop also has CSS); additional entries require
separate reviewed budgets when introduced.

| Bundle   | Baseline raw | Baseline gzip | Raw limit | Gzip limit |
| -------- | -----------: | ------------: | --------: | ---------: |
| Desktop  |       245827 |         79364 |    303185 |     103429 |
| Firefox  |         2502 |          1208 |     11195 |       9642 |
| Chromium |         2502 |          1208 |     11195 |       9642 |

## Validation record

- `go run ./tools/quality doctor`: passed, exact tools and native dependencies diagnosed.
- `go run ./tools/quality check`: passed before final documentation reconciliation; final rerun pending.
- `go run ./tools/quality check:full`: passed locally, including build, coverage, race, subprocess integration and both vulnerability scanners.
- `go run ./tools/quality gate-probes`: passed all 16 controlled rejection checks in a disposable copy; results in build/gate-probes.txt.
- `go run ./tools/quality task integration`: passed, including the actual five-second stdin deadline.
- `go run ./tools/quality task vuln`: passed, no known/reachable Go or high/critical JS vulnerabilities reported at execution time.
- Coverage: desktop state 100%, IPC 87.4%, protocol 98.3% statements. TS behavior suite passed its per-file thresholds.

## Hosted and deferred results

Public repository: https://github.com/Mahmoud-s-Khedr/idm-enhanced . Main protection
was configured and read back: PRs required, Required checks required with strict
up-to-date branches, admins included, zero mandatory approving reviewers, force
pushes/deletion blocked. Native CI run links will be recorded after successful runs.
The first hosted run exposed a missing module-cache bootstrap in native matrix
jobs; the follow-up adds explicit dependency bootstrap before offline builds.
Repeated local builds remove stale extension assets and produce byte-identical
asset fingerprints. Current extension output is 2537 raw / 1209 gzip bytes, within
the unchanged initial budgets. The first calibration (2502 / 1208) remains recorded above. Windows/macOS interactive launch remains
outstanding even after native builds pass. Windows/macOS live browser IPC belongs
to M5. No signing, installer, power-loss, engine, download, media or additional
architecture compatibility is claimed.
