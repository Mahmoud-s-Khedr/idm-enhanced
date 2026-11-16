# ADR 0001: M0 boundaries and reproducible validation

Accepted: 2026-11-17 (approved M0 plan).

One Go module owns backend contracts. The engine remains independent of Wails,
presentation and browser transports. No engine, manager or storage framework is
created before those milestones need it. Future queue/lifecycle policy belongs to
the manager; adapters must not reproduce it. Depguard and dependency-cruiser enforce
import boundaries; Go also rejects import cycles.

The desktop adapter owns only presentation of build and ping state. The native
host forwards one validated request to the existing app; it cannot launch an app
or engine. `internal/protocol` owns framing and the versioned envelope. A generator
emits extension constants/types from Go; both languages exercise common fixtures.
Wails generates desktop service bindings. Generated files are compared byte for
byte and never manually maintained.

Linux uses a user-owned 0700 directory and 0600 Unix socket, eight concurrent
connections, a 64 KiB frame cap, and a five-second round-trip deadline. Cancellation
closes the listener and connections and waits for workers. An existing socket is
never silently removed: a second instance fails, and stale-socket cleanup requires
verifying the previous app has stopped. Native stdout contains only framed JSON.

The canonical Go runner owns task composition. Hooks validate an index snapshot
and never rewrite the index/worktree or install tools. Full checks and failure
probes belong in CI. Missing executables, empty required suites and skipped
mandatory jobs fail validation. Pin changes are visible review changes; checks do
not recompute budgets. `tools/versions.json`, package manifests and lockfiles own
exact versions; bootstrap never resolves latest.

Windows and macOS require native builds and framing tests. Their live IPC and
registration are deferred to M5. Linux is the M0 interactive target. Installers,
code signing and additional architectures require later independent evidence.

## Dependencies

Go's standard library handles IPC, framing, validation, tests, coverage parsing,
processes and budgets. Wails v3.0.0 is the sole direct Go runtime dependency (MIT),
selected by the approved plan for native WebViews and generated bindings. Its CLI
is pinned to the same version, installed locally, and not shipped with the app.
The Wails module brings upstream transitive desktop/platform dependencies; `go.sum`
records these. No database or transfer dependency is introduced in M0.

React/React DOM (MIT) provide the approved frontend model. `@wailsio/runtime`
3.0.0 (MIT) supplies the actual generated binding transport. Vite, TypeScript,
ESLint/typescript-eslint, Prettier, Knip, dependency-cruiser, jscpd, Vitest,
Testing Library, jsdom and Lefthook are development-only tools. These established
upstream projects implement the plan's distinct build, static analysis and test
requirements; the standard library cannot replace their language/framework
analysis. TypeScript 6.0.3 is chosen because typescript-eslint 8.73.0 declares
support below 6.1; TypeScript 7 is currently incompatible. The lockfile records
transitive packages and integrity hashes. Go analyzers and scanners are installed
locally at exact versions; golangci-lint alone runs govet/staticcheck.

Runtime impact is measured by committed JS/CSS raw and gzip budgets in
`tools/budgets.json`; no Node runtime ships in the desktop app. Extension output
has no third-party runtime dependency. Tool licenses are upstream permissive
licenses (primarily MIT/Apache-2.0; golangci-lint GPL-3.0 is a development executable,
not linked into the application). Revisit license inventory/SBOM before release.

## Coverage scope

All present handwritten application logic under `internal/` is subject to 80%
per-package statements on its supported platforms. Application IPC is Linux-only
in M0, so live-IPC coverage is explicitly not applicable on Windows/macOS until M5.
Those targets still require shared protocol coverage and native-host framing/error
process tests; a reported skip must not imply a working live IPC bridge. Frontend App and extension connection/popup behavior use
80% statements/lines/functions and 75% branches per file. Generated bindings,
generated protocol types, the root render call, asset embedding and minimal
framework/native command registration are excluded. Tooling is validated through
controlled gate probes and focused runner tests rather than application coverage.
Future logic must enter the measured package/file set in the introducing change.
