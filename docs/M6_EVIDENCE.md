# M6: Core beta hardening

Updated: 2026-11-18. Engineering hardening is implemented; final acceptance requires the protected PR merge.
Human beta-user acceptance remains pending: the owner explicitly plans to send
it to beta users later. M6 must not be reported as a completed public R1 release.

## Prerequisite and delivered behavior

M5 PR #7 merged as b77ba5117ae2e95056488e3ad345d53d85dd3796. Its final head
3b38292b510221486cf5a0f3904a81168dc2d9f1 passed Quality run 44298050894,
including quality, native and package matrices and Required checks. This record
was read back from GitHub before M6 began.

The desktop cancellation panel now moves focus to Keep partial, supports Escape,
and restores focus to the invoking control. Destructive panel choices are
unavailable while a drain runs. A skip link reaches stable download/settings
content. The destination field has a separate explicit label; Browse is no longer
inside the field label. Progress exposes transferred/total or unknown-size text
and treats known zero-length files as determinate. Light-theme focus has a dark
outline; long names and narrow layouts can wrap. Native WebView smoke additionally
checks cancellation focus/Escape and 200% text reflow. This is a targeted
accessibility pass, not screen-reader certification.

Recovery advice is owned by the manager and projected through regenerated Wails
bindings. Advice distinguishes ordinary retry, unsafe source/checksum resume,
missing ownership, publication and disk problems without emitting source URLs,
partial paths or arbitrary error details. No transfer/checkpoint policy changes
or released migration edits are made. Existing eight-boundary process-kill
fixtures for sequential and parallel downloads remain mandatory.

Version 0.6.0 packaging uses one Go version value, passed to Inno Setup instead of
manually duplicating it in the installer. Required Debian/Fedora/Windows jobs now
install a controlled 0.5.0 predecessor, persist a queued job with the installed
CLI, upgrade to 0.6.0 and compare durable status before removal. The predecessor
uses current binaries with lower package metadata: this establishes native
upgrade mechanics, not an archived M5-binary replay. Existing released-schema
migration fixtures remain separate. Package artifacts contain only 0.6.0 output.
No new dependency, threshold increase, exclusion or protection bypass is added.

## Security and recovery review

Reviewed manager/session admission and pause/join ownership, resource/range
identity and publication paths, desktop projection, native protocol and IPC
boundaries. Existing request validators reject URL credentials and unsupported
schemes; transfer redirects remove Referer (including signed queries) and reject
HTTPS downgrade. Desktop snapshots omit source URLs and ownership data. Unix
uses private user-owned directories and 0600 sockets; Windows uses a user-SID
pipe/DACL. Requests retain 64 KiB framing, five-second deadlines and eight IPC
connection slots. Same-user processes remain inside the trust boundary. Browser
session credentials are unsupported rather than persisted or replayed.

Regression tests cover recovery hints (including unsafe protocol responses and
unknown categories without metadata leakage), focus restoration/Escape,
unknown-size accessible progress and zero-length determinate progress. Existing
core fixtures continue to exercise damaged files, failed checkpoints, retries,
identity changes and verified publication recovery.

## Validation

Exact final commands and hosted PR/check records are recorded with the PR.
The following passed locally on Fedora 45 amd64 with the pinned tools:

- `go run ./tools/quality doctor`
- `go run ./tools/quality check`
- `go run ./tools/quality check:full` (race, integration, Gitleaks, host/Windows vulnerability scans and reviewed dependency audit)
- `go run ./tools/quality task desktop-smoke` (real WebView, cancellation focus/Escape, 200% text reflow and verified output)
- `go run ./tools/quality task browser-smoke` (Firefox 160.0 and Chromium 157.0.8103.78, durable handoff/retry)
- `go test ./internal/manager ./internal/desktop ./tools/package` and the additional desktop checksum-failure regression
- `git diff --check` and `sh -n packaging/smoke-linux.sh`

Critical statement coverage: engine 96.1%, manager 93.0%, storage 92.1%.
Desktop assets: 259,834 raw / 83,670 gzip bytes; extension assets: 6,486 raw /
3,048 gzip per browser, within unchanged budgets. Scanners found no Go
vulnerabilities or secrets; the existing development-only picomatch applicability
exception remains visible and expires 2026-12-01. No gate is weakened.

Native package installation/upgrade results are established by CI, not local
source builds. Required final-head CI
includes the existing quality, native and package jobs with stricter upgrade
smoke; no new head inherits an earlier head's success.

## Native harness correction

A final-head rerun exposed a timeout in the new text-reflow check: waiting for
requestAnimationFrame can stall for an occluded native window. The check now
reads scrollWidth directly, forcing current layout without requiring a
compositor frame. The real native WebView smoke passed after this correction;
the earlier timeout is not counted as a passing run.

## Review correction

Automated review identified that remote_identity also classifies transient
resume-probe failures whose checkpoint remains valid. Advice now uses the saved
state: failed probes recommend Resume / retry, while needs_attention identity
changes retain Restart guidance. The existing network-error/HTTP-503 fixture
asserts the advice, preserves the two committed bytes and successfully resumes
without restarting. Focused manager/desktop race tests cover this correction.

## Acceptance boundary

A second review found that CLI status is written to stderr, while the initial
upgrade smoke captured stdout. Both package scripts now explicitly capture
stderr, extract the seeded job ID, require its queued/zero-progress record before
upgrade and request that exact ID afterward. Missing jobs fail the installed CLI
command; changed/nonempty snapshots fail comparison. Earlier green comparisons
are not treated as durable-retention evidence; corrected final-head package
checks must pass before protected merge.

Hosted Ubuntu native provisioning stalled on its regional EC2 apt mirror,
including a fresh-runner retry. The shared hosted-Ubuntu bootstrap now replaces
regional archive URIs with Ubuntu's HTTPS public archive in both source files
and the hosted image's indirect apt-mirrors.txt list. Logs exposed the indirect
list after the initial source-file-only correction. Apt network waits/retries
and the overall provisioning step are bounded. Signed repository verification, packages and required jobs are
unchanged. Other distro packaging jobs keep their native repositories. This is
provisioning resilience, not a platform/test exclusion.

See [support and recovery guide](SUPPORT.md) for the platform/source matrix,
manual checksum-verified update/rollback process and remaining release work.
Unsigned 0.6.0 artifacts stay in the internal test channel. Beta-user feedback is
explicitly deferred by the owner, and Windows/macOS interactive coverage,
screen-reader tests, physical power delivery and public signing/naming are not
invented. R1 feature follow-ups (speed/ETA, batch import/categories) remain open.
M7 media work is not part of this hardening change.

## Protected engineering acceptance

PR #8 merged as 0acec4abf9e8fee1b427c427ecffa261bcac4e9b after final head
2d7fb98a1175ab363a4d8130705746e122f2de2e passed run 44342722093, including
the corrected nonempty installed-CLI status comparisons on Debian/Fedora/Windows.
All required quality, native, packaging and aggregate jobs passed. Both review
threads were resolved. All final-head installer/extension checksums were verified.
The owner then authorized M7 work while deferring beta-user testing; this is
engineering acceptance, not public R1 release acceptance.
