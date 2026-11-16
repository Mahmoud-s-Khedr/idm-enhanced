# M0: Repository foundation and platform spike

## Summary

Establish a reproducible, agent-ready repository with enforced quality gates, a minimal Wails application, and working Firefox/Chromium communication.

**Duration:** 7–10 working days.

**Repository:** `github.com/Mahmoud-s-Khedr/idm-enhanced`

**License:** MIT

**Merge policy:** Pull requests with passing required checks; no mandatory second-person approval.

M0 ends with demonstrated infrastructure and recorded platform evidence. Downloading, queues, SQLite persistence, media processing, installers, and signing remain later milestones.

## 1. Repository and reproducible tooling

- Initialize Git with `main`, preserve existing planning documents, and add the MIT license, README, ignore rules, editor settings, contribution instructions, and PR template.
- Use one Go module named `github.com/Mahmoud-s-Khedr/idm-enhanced` and one pnpm workspace containing the desktop frontend and browser extensions.
- Pin Wails CLI and library to **v3.0.0**, which is a published non-prerelease. Keep their versions identical.
- Resolve supported stable Go, Node, pnpm, and analyzer versions during bootstrap; commit their exact versions and dependency lockfiles. Installation and CI must consume these pins, never resolve `latest`.
- Supply explicit bootstrap and diagnosis commands. Install tools into repository-local locations where practical; hooks must not download tools automatically.
- Install Linux prerequisites for the selected Wails stack: GTK4 and WebKitGTK 6 development packages. Install Firefox for the browser spike; Chromium is already available.
- Add architecture decision records covering engine independence, dependency direction, canonical tooling, native messaging, and platform support.
- Track every quality gate as **active**, **not applicable yet**, or **blocked**, with its activation milestone. Missing tools for active checks fail validation.

Create the public repository after checking tracked files and history for secrets. Configure hosting rules once workflow check names exist.

## 2. Canonical checks and enforcement

Implement a cross-platform Go runner at `tools/quality`. Provide these entry points:

| Command                              | Behavior                                                                                |
| ------------------------------------ | --------------------------------------------------------------------------------------- |
| `go run ./tools/quality doctor`      | Report pinned versions, native dependencies, browsers, and platform limitations         |
| `go run ./tools/quality check`       | Run all active offline checks, tests, coverage, builds, and bundle budgets              |
| `go run ./tools/quality check:full`  | Add race tests, integration tests, secret detection, and dependency vulnerability scans |
| `go run ./tools/quality task <name>` | Run one underlying task for hooks, CI jobs, and focused development                     |
| `go run ./tools/quality gate-probes` | Demonstrate that configured gates reject controlled failures in a disposable copy       |

Use the following tooling:

- **Go:** gofmt; golangci-lint with curated checks for errors, resource handling, static analysis, unused declarations, duplication, and imports; ordinary tests; supported race tests; govulncheck.
- **TypeScript:** Prettier, ESLint, strict TypeScript, Knip, dependency-cruiser, jscpd, Vitest, and Testing Library.
- **Repository/security:** Lefthook, Gitleaks, frozen pnpm installation, Go dependency-manifest drift checks, and pnpm audit.
- **Build integrity:** generated-binding drift checks and deterministic bundle measurements.

Avoid running equivalent static analyzers twice. Commands must report failures and explicit platform skips; missing executables and unexpectedly empty required suites cannot produce success.

**Architecture and duplication rules**

- Engine packages cannot import Wails, browser adapters, or frontend code.
- Application policy belongs to the manager; storage and presentation remain separate.
- Browser implementations share protocol and interaction logic, with small manifest/API adapters.
- Go import restrictions use depguard; frontend/extension boundaries and cycles use dependency-cruiser.
- Enable Go `dupl` with a 100-token starting threshold. Configure TS clone detection for qualifying blocks of at least 100 tokens and 10 lines.
- Reject qualifying production clones unless covered by a precise documented exception. Exclude generated output and deliberate fixtures narrowly; report test duplication separately.

**Coverage and budgets**

- Enforce 80% statement coverage per handwritten Go logic package present in M0.
- Enforce frontend/extension thresholds of 80% lines, statements, and functions, plus 75% branches.
- Exclude generated bindings and minimal framework registration explicitly. Missing engine packages remain not applicable; critical-engine thresholds activate when those packages arrive.
- Measure desktop and extension bundles separately. Set each initial raw/compressed asset budget to the clean scaffold baseline plus 20% and 8 KiB headroom; commit the resulting numeric values. Do not recalculate thresholds automatically during checks.

**Hooks and CI**

- Pre-commit runs check-only staged formatting, fast applicable lint, whitespace checks, and staged-secret detection. It must preserve partially staged work.
- Full validation belongs in CI. Optional pre-push checks invoke existing tasks.
- Use Linux, Windows, and macOS native-build jobs with pinned runner labels and action revisions.
- Configure one always-running aggregate required check that rejects failed, cancelled, or unexpectedly skipped mandatory jobs.
- Protect `main`: require PRs and the aggregate check; block force pushes and deletion. Do not require another reviewer.
- Keep workflow permissions minimal and release secrets absent from PR execution.

## 3. Wails shell and browser spike

**Desktop shell**

Build a minimal React/TypeScript application inside Wails containing:

- Application/build information and backend connection status.
- A button that invokes a typed Go service and displays its response.
- A browser-connection panel showing the last acknowledged ping.
- Explicit error and disconnected states.

Generate Wails bindings rather than maintaining duplicate frontend service signatures. Add no download UI, persistence, component library, or state-management dependency.

**Browser interfaces**

Add minimal Firefox and Chromium extensions with a popup action: **Test desktop connection**.

The complete Linux path is:

`extension → native host → local IPC → running Wails application → acknowledgement`

- Use a versioned ping/request-acknowledgement contract with request IDs.
- Share Go protocol definitions between the application and native host; share extension-side protocol handling across both browsers.
- Validate the contract with common fixtures exercised by Go and TypeScript tests.
- Use native-messaging framing over stdin/stdout; write diagnostics only to stderr.
- Use a user-private Unix-domain socket on Linux with restricted permissions.
- Bound messages to 64 KiB and round-trip waits to five seconds.
- Require the desktop app to be running. Do not implement automatic application launch in M0.
- Provide development registration and cleanup commands for both browser host manifests, using stable development extension identities and explicit allowed origins.
- When the app is absent, return an actionable error; never create another application or engine process.

Windows/macOS desktop shells and native-host framing must build and test. Their live browser-to-app IPC and registration remain explicitly deferred platform work rather than simulated support.

## 4. Verification and acceptance

Run controlled failure probes in a disposable repository copy. Do not alter the working tree or introduce real credentials.

| Area                  | Required evidence                                                                                  |
| --------------------- | -------------------------------------------------------------------------------------------------- |
| Formatting/lint/types | Intentional violations are rejected                                                                |
| Architecture          | Forbidden Go and TS imports fail                                                                   |
| Duplication           | Qualifying copied Go and TS blocks fail                                                            |
| Dependency hygiene    | Unused declared dependency and manifest drift fail                                                 |
| Security              | A harmless scanner-test secret is detected                                                         |
| Coverage              | An intentional uncovered behavior lowers coverage and fails                                        |
| Bundles               | An intentional budget overrun fails                                                                |
| Generated bindings    | Changed service contract without regeneration fails                                                |
| Hooks                 | A partially staged change remains unchanged after rejection                                        |
| CI aggregation        | Failed and unexpectedly skipped jobs cannot yield a green required check                           |
| Native messaging      | Partial frames, malformed JSON, unsupported versions, oversized messages, and timeouts are handled |
| Browser round trips   | Firefox and Chromium independently receive matching acknowledgements from the running Linux app    |

Record platform results separately:

- **Linux amd64:** native build, interactive shell launch, backend call, both browser round trips.
- **Windows:** native CI build and noninteractive tests; interactive launch remains outstanding.
- **macOS:** native CI build and noninteractive tests; interactive launch remains outstanding.
- Record actual runner architecture and dependency versions. Do not imply additional architectures were verified.

M0 is complete when active gates demonstrably work, `main` has enforced required checks, Linux interactive evidence exists, and all remaining platform gaps have explicit follow-up milestones.

## 5. Delivery sequence and review artifacts

| Days | Deliverable                                                                                                 |
| ---- | ----------------------------------------------------------------------------------------------------------- |
| 1–2  | Git/module/workspace setup, exact tooling pins, Linux prerequisites, bootstrap/doctor, architecture records |
| 3–4  | Canonical checks, hooks, boundary rules, duplication checks, coverage, and failure probes                   |
| 5–6  | Wails shell, generated bindings, native build matrix, measured bundle budgets                               |
| 7–8  | Shared extension logic, native host, Linux IPC, Firefox and Chromium round trips                            |
| 9–10 | CI enforcement, clean-machine validation, evidence report, documentation reconciliation                     |

At handoff, update the project plan, quality policy, and agent instructions to distinguish implemented commands from planned ones. Include setup instructions, supported-platform evidence, gate-probe results, dependency justifications, and tracked limitations.

Assumptions: Linux is the only interactive test environment; GitHub-hosted Windows/macOS runners are available; repository rules can be configured with the signed-in account. Missing signing credentials do not block this unsigned development spike, but remain recorded release requirements.
