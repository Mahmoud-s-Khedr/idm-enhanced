# Gate rollout

Current milestone: M2 implemented locally; hosted acceptance pending (M0 accepted). `check` runs active offline tasks; `check:full` adds the
active race, native-host integration and network security checks. Missing tools
fail. `doctor` diagnoses installations; it is not proof that platform tests passed.

| State              | Gate / task                                                                                                     | Activation                                                    |
| ------------------ | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| active             | versions, format, lint-go, lint-ts, types                                                                       | M0                                                            |
| active             | deadcode (Knip), architecture (dependency-cruiser/depguard), clones (jscpd/dupl), test-clones (separate report) | M0                                                            |
| active             | mod-drift, frozen pnpm bootstrap/CI installation                                                                | M0                                                            |
| active             | test-go (80% per internal package), test-tools, test-ts (80/80/80/75 per file)                                  | M0                                                            |
| active             | bindings (Go protocol generator and Wails generator), build, budgets                                            | M0                                                            |
| active             | race (Linux amd64), integration (native-host subprocess framing/deadlines)                                      | M0 full                                                       |
| active             | secrets, history-secrets, vuln (govulncheck and pnpm audit)                                                     | M0 full / CI                                                  |
| active             | staged formatting/lint/whitespace/secrets, index-preservation probe                                             | M0 hook                                                       |
| active             | aggregate rejects failed/cancelled/skipped/missing jobs                                                         | M0 CI                                                         |
| active             | gate-probes in a disposable copy, with a clean baseline for each gate                                           | M0 CI                                                         |
| active             | browser-smoke: real extension/native-host/IPC round trips using disposable Linux browser profiles               | M0 acceptance, requires running native app and local browsers |
| active             | Windows/macOS native CI build and framing tests (passed; see M0 evidence)                                       | M0 acceptance                                                 |
| active             | M1 engine/CLI 80% package coverage, deterministic HTTP fixtures and CLI process tests                           | M1                                                            |
| active             | engine, manager, storage 90% package coverage; range/recovery/process-kill fixtures                             | M2                                                            |
| not applicable yet | live Windows/macOS browser IPC and registration                                                                 | M5                                                            |
| not applicable yet | installers, signing, media                                                                                      | corresponding release milestones                              |

SQLite migration rollback/reopen/version/constraint tests are active in storage.
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

Application IPC coverage is active on Linux. Windows/macOS currently expose only
unsupported-platform IPC stubs, so their live-IPC coverage suite is explicitly not
applicable until M5. The runner reports this platform skip; it still requires
protocol coverage and the native-host subprocess framing/error tests. This follows
the approved platform scope, and does not exempt any Linux IPC code from 80%.
