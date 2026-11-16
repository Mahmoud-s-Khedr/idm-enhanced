# Codebase quality policy

Status: M0 executable foundation implemented; see GATES.md and M0_EVIDENCE.md for validation and remaining acceptance work

Updated: 2026-11-17

Applies to: Go engine/backend, React frontend, browser extensions, native host, tests, tooling, and CI

## 1. Purpose and source

Make this repository understandable and safe to change for human engineers and coding agents. Prevent common mistakes with clear ownership, narrow contracts, tests, and enforced checks. Automated tools reduce risk; they cannot guarantee mistake-free code or identify every semantic duplicate.

The [shared quality discussion](https://chatgpt.com/share/6afb7ff1-1a28-83ea-8250-5857cc336c47) explicitly emphasizes pre-commit checks, coverage, unused dependencies and bundles, linting, an agent-ready repository, and duplicated code. Some replies were absent from the retrieved page and appeared only as “Worked…” placeholders. This policy incorporates the readable requirements and project-specific engineering judgment, not a claimed reconstruction of those missing replies. The mentioned `cameri/nostream` repository has not been audited as part of this update.

`PROJECT_PLAN.md` owns product scope and milestones. This policy owns quality gates. `AGENTS.md` provides concise working instructions and points here rather than duplicating the full policy. The canonical Go runner, pinned tools, hook configuration, CI workflow, numeric bundle budgets and test suites now implement the M0 foundation. Hosting rules and platform runs require independent evidence in M0_EVIDENCE.md.

## 2. Structure and ownership

Start with one Go module and a pnpm workspace for the frontend and extensions. Pin toolchains and tools, commit lockfiles, use frozen dependency installation in CI, and document a reproducible bootstrap. Introduce additional packages only when their ownership and dependency direction are clear.

| Concern                                            | Single owner                                   | Boundary to preserve                                    |
| -------------------------------------------------- | ---------------------------------------------- | ------------------------------------------------------- |
| HTTP request/range behavior, retries, cancellation | Engine                                         | No Wails, browser, or UI imports                        |
| Job transitions, queues, schedules, user commands  | Manager                                        | Adapters invoke policy rather than reimplementing it    |
| Persistent state/checkpoints                       | Storage with explicit engine/manager contracts | React and extensions never write SQLite                 |
| UI state/presentation                              | Frontend                                       | Go remains authoritative for job state                  |
| Browser discovery and permissions                  | Extension shared logic plus browser adapters   | No second transfer engine                               |
| Message envelopes and DTOs                         | Versioned contracts at the boundary            | Generate or validate bindings rather than copying types |
| Media manifests and assembly plans                 | Media                                          | Reuse transport without duplicating HTTP policy         |

Keep dependency direction explicit in a short architecture record. Avoid cyclic runtime dependencies and large cross-layer interfaces. Use interfaces at the consumer's boundary, not one interface for every struct. Error categories should be stable across adapters while presentation text stays in the appropriate UI layer.

Enforce forbidden Go imports with targeted `depguard` rules and, where necessary, a package-graph check. Use dependency-cruiser for frontend/extension cycles and forbidden cross-package imports. Go rejects import cycles itself; its compiler does not enforce the project's layering rules.

## 3. Duplication prevention

Duplication is a first-class review concern, especially duplicated business rules that slowly diverge.

Before introducing code, search for existing equivalents and identify where the behavior belongs. Review should ask whether lifecycle, retries, resource validation, credential handling, or serialization is being implemented twice. Clone scanners are supplementary: semantically identical behavior can look different enough to escape them.

Use these automated checks:

- Go: enable `dupl` in golangci-lint for substantial copied fragments.
- TypeScript/TSX and shared extension code: use jscpd for copied blocks.
- Start clone calibration at approximately 100 tokens and 10 lines where supported by the selected tool; Go `dupl` uses its own threshold semantics. Pin exact versions and supported options during M0.
- Once calibrated, fail unapproved qualifying clones in handwritten production code. Start clean; a total-repository percentage alone can hide new copies as the repository grows.
- Generated bindings, build output, vendored code, and intentional fixtures are outside production clone gates. Analyze tests separately, initially as a report, because table-driven fixtures can be deliberately repetitive.
- Any exception identifies exact files/blocks, why reuse would be harmful, the reviewer, and a revisit trigger. Do not use blanket directory exclusions to hide product code.

Share code when it represents the same invariant or responsibility. Similar-looking code with different ownership or future behavior can remain separate with an explanation. Do not force premature abstractions, inheritance, or generic helper frameworks to reach a zero-duplication score.

## 4. Tools and failure conditions

The M0 configurations pin these tools. Gate activation and remaining platform limitations are tracked in GATES.md.

| Gate                      | Tool/approach                                      | Failure condition                                                                                                                |
| ------------------------- | -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Go format                 | `gofmt`                                            | Handwritten Go differs from canonical formatting                                                                                 |
| Go lint/static analysis   | golangci-lint                                      | Configured diagnostics, including unchecked errors, staticcheck, unused declarations, response-body leaks, and lost cancellation |
| Go compiler analysis      | `go vet` via one configured path                   | Findings; avoid running equivalent analyzers twice without a reason                                                              |
| Go tests                  | `go test`                                          | Behavioral or integration failures                                                                                               |
| Data races                | `go test -race` on supported runners               | Race reports or failures; skip unsupported targets explicitly                                                                    |
| TS format                 | Prettier                                           | Formatting differences                                                                                                           |
| TS lint                   | ESLint with TypeScript/React rules                 | Errors or warnings; avoid duplicate formatting rules                                                                             |
| TS type safety            | Strict TypeScript checks                           | Type errors in each workspace package                                                                                            |
| TS dead code/dependencies | Knip                                               | Unused files, exports, or dependencies after framework entry points are configured                                               |
| Go dependency hygiene     | `go mod tidy` plus clean-diff check                | Unexpected manifest/checksum drift                                                                                               |
| Dependency direction      | depguard/package graph, dependency-cruiser         | Forbidden imports or frontend/extension cycles                                                                                   |
| Duplication               | Go `dupl`, TS jscpd                                | Unapproved qualifying production clones                                                                                          |
| Frontend tests            | Vitest + Testing Library                           | Behavior tests or coverage thresholds fail                                                                                       |
| Browser/desktop contracts | Fixture tests and native smoke checks              | Invalid envelopes, broken acknowledgements, incompatible generated contracts                                                     |
| Secrets                   | Gitleaks or one equivalent scanner                 | Unapproved secret findings; scan PR changes and schedule history checks                                                          |
| Dependencies              | `govulncheck` and a pinned JS lockfile scanner     | Applicable high/critical findings; Go reachable vulnerabilities require triage irrespective of nominal severity                  |
| Build and generated files | Native builds and regeneration diff                | Broken build or committed generated output differs from its source                                                               |
| Bundle budgets            | Vite output manifest and pinned measurement script | Per-entry or aggregate budget exceeded                                                                                           |

Start Go lint with a curated set, including `errcheck`, `staticcheck`, `unused`, `bodyclose`, `ineffassign`, and `dupl`; validate names and defaults against the pinned version. Add targeted cancellation, context, error-wrapping, and import checks as they prove useful. Do not enable every linter or impose arbitrary function-length limits.

Do not use whole-program dead-code findings to delete code blindly: build tags, platform implementations, multiple executables, generated bindings, and framework registration affect reachability. Analyze the relevant entry points/build configurations and review uncertain findings.

## 5. One command contract

The cross-platform runner is `tools/quality`. Run `go run ./tools/quality check` or `go run ./tools/quality check:full`; use `task <name>` for a focused task. `bootstrap` installs exact pins and `doctor` reports the environment. Aliases must delegate to this runner rather than implementing another policy.

- `check`: formatting, lint/static analysis, type checks, ordinary Go/frontend tests, coverage, dependency hygiene, architecture, clones, dead code, generated drift, and build/bundle validation for the host's active packages.
- `check:full`: everything in `check`, plus the supported race/integration suites, secret/dependency scans, and applicable native/browser smoke checks. CI runs the native build/install matrix separately through the same underlying tasks.
- Tasks return nonzero on failure, print actionable findings, and report skipped platform-specific checks explicitly. Required tasks cannot silently pass because a binary, package, or test suite is missing.
- Hooks and CI invoke those underlying tasks rather than maintaining unrelated command lists.
- While a component does not yet exist, mark its gate as not applicable in a tracked rollout list. Activate its checks in the same change that introduces the component. An empty suite must not masquerade as passing coverage.

Avoid making local hooks depend on slow network installs. Bootstrap tools explicitly and provide a documented setup diagnosis command. CI installs pinned versions rather than following `latest`.

## 6. Layered enforcement

| Layer             | Required checks                                                                                                     | Timing goal                                       |
| ----------------- | ------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| Editor            | Formatter and lint/type diagnostics                                                                                 | Immediate feedback                                |
| Pre-commit        | Staged-file format, applicable fast lint, whitespace, staged-secret scan                                            | Aim below 10 seconds on ordinary changes; measure |
| Pre-push          | Optional focused tests/type checks                                                                                  | Convenience, not authority                        |
| PR CI             | Canonical checks, race tests, coverage, duplication, architecture, dependencies, secrets, active-platform builds    | Required before merge                             |
| Scheduled/release | Wider compatibility, history scans, benchmarks, fuzz campaigns, packaging/install tests, license inventory and SBOM | Required as appropriate for release               |

Lefthook runs the canonical `staged` task against a disposable index snapshot. It checks formatting, applicable lint, whitespace and secrets without rewriting the index or worktree. The gate-probe suite verifies partially staged rejection. CI independently reruns checks.

Protected-branch/ruleset settings and required status checks must be configured in the hosting service during repository setup; a workflow YAML does not enforce branch protection. Use a stable aggregate required-check job that fails on failed, unexpectedly skipped, or cancelled mandatory jobs. Carefully handle path filters and docs-only changes so a required check cannot disappear or silently omit affected packages.

Pin external actions to immutable revisions, minimize workflow permissions, and never expose release credentials to untrusted PR code. Keep signing and publication jobs separate from ordinary PR validation.

## 7. Coverage and meaningful tests

These project thresholds are enforced when the corresponding logic exists; they are not universal industry rules:

| Area                                                | Threshold when activated                     | Notes                                                                           |
| --------------------------------------------------- | -------------------------------------------- | ------------------------------------------------------------------------------- |
| Handwritten Go application packages                 | 80% statements, checked per package          | Active for M0 internal logic; explicit adapter exclusions require rationale     |
| Critical range/checkpoint/state-transition packages | 90% statements                               | Activate as those packages arrive in M2/M3; recovery scenarios remain mandatory |
| Handwritten frontend/extension logic                | 80% lines/statements/functions; 75% branches | Activate with each package's first behavior tests; use explicit inclusion globs |

Go's ordinary coverage reports statement coverage, not branch coverage. Coverage tooling must aggregate runs consistently and compare the intended package set, rather than hiding weak packages behind a single repository average. Generated code and platform glue may require narrow documented exclusions; do not exclude all error handling, adapters, or tests to improve results.

Require regression tests for behavioral bugs and fixture coverage of the plan's corruption/recovery cases. Focus on observable behavior and invariants, not private method call counts. A high percentage does not demonstrate correct assertions, safe concurrency, or power-loss resilience. Documentation-only changes do not need artificial tests.

Changed-line coverage reporting and mutation testing of critical decisions can follow once baseline enforcement is reliable; avoid adding noisy blocking tools before their reports are actionable.

## 8. Dependency and bundle budgets

Every new runtime dependency needs a short justification: existing/standard-library alternatives considered, purpose, maintained status, license, transitive footprint, and measured impact. This is part of the change description, not necessarily another standalone document.

Knip detects unused declared packages; a bundle measurement detects code that is used but unnecessarily expensive. These are separate gates. Configure Knip entries for Wails-generated bindings, React/Vite, extension manifests, content scripts, service workers, and dynamic entry points. Exclusions must be specific and reviewed.

M0 records separate desktop, Firefox and Chromium raw/gzip JS/CSS baselines. Blocking numeric limits are committed in `tools/budgets.json`, calculated once as baseline × 1.2 (rounded up) + 8192 bytes. Checks never recalculate limits. See M0_EVIDENCE.md for measurements.

Measure raw and compressed JS/CSS by initial entry and total emitted application assets. Track extension background/content-script costs separately. A desktop app loads local assets, so web network delivery numbers alone are not an adequate performance measure. Also record native installer size, startup time, memory, and idle resource use. Count FFmpeg/media packages separately when introduced so optional media assets cannot obscure core growth.

Require a reason for a budget increase and a visible policy/configuration diff. Do not move the threshold automatically when a dependency causes failure. Performance timing initially reports trends; use blocking limits only on stable, repeatable hardware or sufficiently controlled benchmarks.

## 9. Exceptions, changes, and agent workflow

- Search existing implementations before adding behavior; keep commits and diffs focused.
- Update an architecture decision record when changing dependency direction, contracts, checkpoint durability, or dependency strategy.
- Keep generated code reproducible; never hand-edit bindings. Version boundary envelopes and test compatibility between the extension, native host, and application.
- Add migration tests and maintain supported upgrade paths when persistence arrives.
- Never weaken a check, broaden an exclusion, or remove a test simply to obtain green CI.
- A legitimate exception records the precise scope, reason, reviewer, mitigation, and expiry/revisit condition. Security exceptions identify the advisory and applicability. Scanner outages are failures or explicit reviewed limitations, not clean scans.
- Report exact validation commands, results, unavailable environments, and unimplemented checks. Neither agents nor humans get a separate bypass policy.
- Repository instructions guide behavior; checks and review provide independent evidence. No claim that agents are incapable of introducing mistakes.

## 10. M0 acceptance criteria

- [x] Document and pin bootstrap toolchains, package manager, tools, and lockfiles.
- [x] Supply canonical check tasks and a clear list of gates not yet applicable.
- [x] Configure staged hooks without damaging partially staged work.
- [x] Configure Go checks immediately; activate TS checks when the shell/extension spike adds TS.
- [x] Complete passing native CI evidence. The workflow uses canonical tasks and hosting protection is configured; actual runner architectures and successful run links are recorded in M0_EVIDENCE.md.
- [x] Demonstrate failures with temporary controlled samples: formatting, lint/type errors, forbidden import, copied block, unused dependency, and dummy-secret fixture.
- [x] When test suites and bundle outputs exist, prove coverage and budget checks reject an intentional regression. Remove temporary probes after validation.
- [x] Configure clone exclusions and architecture entry points narrowly.
- [x] Record representative bundle measurements and commit initial numeric budgets.
- [x] Verify generated-file drift and dependency-manifest drift checks.
- [x] Document supported runner/platform limitations, including race-detector support.

Demonstrating rejection is part of implementing a gate; a check that only passes a clean scaffold has not yet shown that its wiring works. These are tooling acceptance checks, not a demand for permanent tests that merely mirror configurations.

## 11. Tool references

Verified during preparation on 2026-11-17:

- [golangci-lint linters](https://golangci-lint.run/docs/linters/): includes `dupl`, `depguard`, error/static-analysis, and resource-handling checks. Enable a curated configuration.
- [Knip getting started](https://knip.dev/overview/getting-started): reports unused dependencies, exports, and files. Its documented runtime requirements must match the pinned frontend toolchain.
- [jscpd](https://github.com/kucherenko/jscpd): clone-detection tool; pin its release and verify CLI/configuration semantics before wiring thresholds.

Other named tools are planned selections, not tested integrations. Verify their current official documentation and compatibility as M0 implements them.
