# Picomatch advisory applicability review

Reviewed: 2026-11-17. Reviewer: Codex, with the rationale and guard changes exposed
in the protected fix PR. Expires: **2026-12-01 00:00 UTC**, enforced by the quality
runner whenever this exception would be used. No audit severity threshold changes.

## Scope and decision

[GHSA-fc5q-39xc-vjpf / CVE-2026-112366](https://github.com/advisories/GHSA-fc5q-39xc-vjpf)
was added to the audit result after M2's successful checks. It caused
[main run 44152476015](https://github.com/Mahmoud-s-Khedr/idm-enhanced/actions/runs/44152476015)
to fail the dependency gate; the native jobs, tests, race checks and Go vulnerability
scan passed. On review, npm's latest picomatch release is still **4.0.7** and the
advisory lists **no patched version**. This change does not claim it is patched.

The issue requires attacker-controlled glob **patterns**, not simply input strings
matched against repository-defined patterns. The picomatch maintainer disputes the
advisory and recommends keeping patterns trusted in
[upstream PR #235](https://github.com/micromatch/picomatch/pull/235#issuecomment-6315749381).
That proposed patch was closed without merging, so it is not a supported release.
[Nx's maintainers document the same trust boundary](https://github.com/nrwl/nx/issues/37696#issuecomment-6655824029).

This repository uses picomatch only through development tooling: Vite, Vitest,
Knip, dependency-cruiser and their globbing dependencies. Patterns come from checked-in
build/lint/test configuration and source code. Download URLs, response bytes,
manager jobs, desktop messages and browser input never become Node glob patterns.
The native Go downloader does not load Node packages. A malicious contributor or
build plugin already able to change those patterns can execute arbitrary build
code; this exception does not establish a sandbox for untrusted repositories.

`pnpm -r list picomatch --prod --depth Infinity --json` returned no picomatch
instances in any workspace's production dependencies. `pnpm why picomatch` and
`pnpm audit --audit-level=high --json` identified only version 4.0.7 and development
findings. The lockfile remains unchanged; there is no speculative upgrade, fork,
package replacement, or broad development-dependency exclusion.

## Enforcement and revisit

The canonical `vuln` task still executes govulncheck and a full pnpm audit. It reads
the complete JSON result rather than configuring pnpm to hide the advisory. It
accepts this one **high** advisory only when the package is exactly `picomatch`,
every reported instance is version `4.0.7`, every finding explicitly has `dev: true`
and dependency paths, and the review has not expired. The runner prints a visible
**REVIEWED development-only applicability exception** message and the expiry.

Every other high/critical finding still fails. A production finding, unknown
scope, different affected version, changed severity, missing evidence, malformed
report, registry failure, or expired review fails closed. Regression fixtures test
these boundaries, including an unrelated high finding alongside the reviewed one.
Direct `pnpm audit` continues to report the advisory; this is a scoped applicability
decision in the project's canonical gate, not a clean raw audit result.
The JSON audit command can exit 1 for lower-severity findings too; the runner
validates counts and details at every severity before applying the existing
high/critical threshold. Low/moderate findings do not become scanner failures.

Remove or reassess this exception when a patched release appears, the advisory is
withdrawn, picomatch's version changes, or any new runtime/untrusted-pattern use is
introduced. Review is mandatory by the expiry even if upstream remains unchanged.
Any renewal must record new evidence and receive visible review; it is not automatic.
