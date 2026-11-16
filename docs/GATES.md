# Gate rollout

Current milestone: M0. `check` runs active offline tasks; `check:full` adds the
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
| blocked            | Windows/macOS native CI evidence until hosted runs complete                                                     | M0 acceptance                                                 |
| not applicable yet | engine 90% critical coverage, HTTP/range/recovery fixtures                                                      | M1–M3 as logic arrives                                        |
| not applicable yet | live Windows/macOS browser IPC and registration                                                                 | M5                                                            |
| not applicable yet | installers, signing, media, SQLite migration checks                                                             | corresponding release milestones                              |

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
