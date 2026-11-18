# Gate rollout

Current milestone: M7 HLS VOD; M6 PR #8 is merged. Evidence is tracked in M7_EVIDENCE.md; human beta acceptance remains deferred. M4 PR #6 is merged. M2 acceptance remains recorded in M2_EVIDENCE.md. `check` runs active offline tasks; `check:full` adds the
active race, native-host integration and network security checks. Missing tools
fail. `doctor` diagnoses installations; it is not proof that platform tests passed.

| State            | Gate / task                                                                                                     | Activation                                                    |
| ---------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| active           | versions, format, lint-go, lint-ts, types                                                                       | M0                                                            |
| active           | deadcode (Knip), architecture (dependency-cruiser/depguard), clones (jscpd/dupl), test-clones (separate report) | M0                                                            |
| active           | mod-drift, frozen pnpm bootstrap/CI installation                                                                | M0                                                            |
| active           | test-go (80% per internal package), test-tools, test-ts (80/80/80/75 per file)                                  | M0                                                            |
| active           | bindings (Go protocol generator and Wails generator), build, budgets                                            | M0                                                            |
| active           | race (Linux amd64), integration (native-host subprocess framing/deadlines)                                      | M0 full                                                       |
| active           | secrets, history-secrets, vuln (govulncheck and pnpm audit)                                                     | M0 full / CI                                                  |
| active           | staged formatting/lint/whitespace/secrets, index-preservation probe                                             | M0 hook                                                       |
| active           | aggregate rejects failed/cancelled/skipped/missing jobs                                                         | M0 CI                                                         |
| active           | gate-probes in a disposable copy, with a clean baseline for each gate                                           | M0 CI                                                         |
| active           | browser-smoke: real extension/native-host/IPC round trips using disposable Linux browser profiles               | M0 acceptance, requires running native app and local browsers |
| active           | Windows/macOS native CI build and framing tests (passed; see M0 evidence)                                       | M0 acceptance                                                 |
| active           | M1 engine/CLI 80% package coverage, deterministic HTTP fixtures and CLI process tests                           | M1                                                            |
| active           | engine, manager, storage 90% package coverage; range/recovery/process-kill fixtures                             | M2                                                            |
| partly evidenced | Windows/macOS native IPC and registration fixtures active; live browser/UI acceptance pending                   | M5/M6                                                         |
| active / pending | Debian/Fedora/Windows unsigned installers active; public signing and media pending                              | M5 packages; later release gates                              |

M3 range-worker, shared-limit, queue, retry and parallel process-kill fixtures are active.
SQLite migrations 001/002 and rollback/reopen/version/constraint tests are active in storage.
Critical coverage and engine-to-storage dependency rejection have controlled probes.

There are no broad production clone exceptions. Generated desktop bindings and
protocol types, dependencies, build outputs and fixture/test files are excluded
from production clone analysis. Tooling coverage is validated by focused tests
and controlled failure probes; framework entry points and generated code are
excluded from application coverage with rationale in ADR 0001.

Knip ignores only command-line dependencies invoked from the Go runner
(dependency-cruiser, jscpd, Prettier, Lefthook), which Knip cannot discover from Go.
There is no wildcard dependency exception; the unused-dependency probe remains
required. Gitleaks permits only the exact public development Chromium key in its
manifest, alongside installed/generated artifacts. No private key is tracked.

Linux interactive UI evidence is user-observed; automated browser evidence is
headless and is recorded separately. A local green check cannot establish native
Windows/macOS build results or hosted branch protection.

Application IPC coverage is active on Linux, macOS and Windows. Native fixtures
exercise Unix sockets or the Windows per-user pipe. The earlier non-Linux stub
exemption was removed in M5; framing and native-host process tests remain required.

The dependency gate retains high/critical enforcement and has one expiring,
version- and development-scope-specific applicability review for
[GHSA-fc5q-39xc-vjpf](security/GHSA-fc5q-39xc-vjpf.md). Raw audit findings remain
visible; expired, changed-scope and unrelated high/critical findings fail.

M4 desktop/manager/session, schedule/settings migration, frontend workflow and
redaction fixtures are active under the existing coverage thresholds.
`task desktop-smoke` is an explicit display-dependent native acceptance task;
ordinary headless checks do not silently claim it ran. The test driver is excluded
from production by the `desktop_smoke` build tag. Existing required CI jobs and
thresholds remain unchanged. See M4_EVIDENCE.md for exact native limitations.

## M5 activation

Browser download/accepted envelopes, schema-4 atomic receipts and process-level
native-host handoff are active. Linux/macOS live IPC and Windows user-only named
pipes have native fixture coverage; the old non-Linux IPC coverage exemption is
removed. Extension context menus, pending-request retry and safe filename modules
are included in existing per-file frontend thresholds. No threshold is reduced.

CI additionally builds/installs/uninstalls native Debian 13 and Fedora 45 packages,
and Windows setup using Inno Setup 6.5.4. The aggregate requires the linux-packages
matrix and native Windows installer checks. Artifacts include extension archives
and SHA256SUMS. Display-dependent browser/desktop smoke remains explicit; native
CI does not establish interactive Windows/macOS browser compatibility.

## M6 activation

Manager recovery hints, cancellation focus/Escape, accessible progress and native
200% text reflow checks extend existing suites/thresholds. Required packaging
jobs add controlled native version-upgrade and durable queue retention checks.
Schema 4/checkpoint v1 and all recovery gates stay unchanged. Signing, live
Windows/macOS browser/UI evidence and owner-deferred beta feedback remain
release gates; see SUPPORT.md. No mandatory job or threshold is relaxed.

## M7 activation

Media parsing/assembly/object recovery, schema-5 migration/checkpoint privacy and
lightweight summaries are active. Media joins engine/manager/storage at 90%
per-package coverage. Both transfer packages have adapter/persistence import
restrictions. `check:full` additionally runs `task media-smoke`, a nonempty suite
requiring real FFmpeg/ffprobe and deterministic synthetic playback fixtures.
Hosted Ubuntu bootstrap supplies tools explicitly; hooks/ordinary checks never
download them. Native browser smoke admits exactly four file/HLS jobs. Native
desktop smoke checks explicit HLS selection, MP4 output and temporary-cache cleanup.

The only new formatter/linter ignore is binary MPEG-TS fixture data under
`testdata/hls/**/*.ts`, which shares TypeScript's suffix; Git also marks it binary.
No handwritten source is excluded and budgets remain unchanged. Active-tab
discovery adds user-gesture scripting permissions, without broad host/cookie
access; live permission/site coverage and Windows/macOS playback remain explicit
limitations rather than green-check claims.

## M8 activation

Static DASH remains under the existing critical media coverage, architecture,
race/recovery and mandatory real-media gates. Media-smoke now decodes all four
synthetic DASH video/audio combinations after offline persisted assembly retry.
Native desktop/browser smoke includes explicit DASH analysis/selection/handoff.
Schema-6 upgrades and package version 0.8.0 use the existing required platform and
installer jobs. No thresholds, budgets, exclusions or required checks change.
