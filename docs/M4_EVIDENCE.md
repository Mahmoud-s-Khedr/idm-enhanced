# M4: Desktop alpha

Updated: 2026-11-18. Implementation and acceptance evidence are tracked here.
M3 PR #5's protected merge (`daf89a7`) was verified before M4 work began.
Final M4 acceptance requires full quality checks, native CI and protected merge.

## Delivered behavior

The desktop now manages durable downloads without the CLI: add/paste or drop an
HTTP link, select a new destination, optionally verify SHA-256 or schedule a start,
run/reorder the queue, pause/resume/retry/restart, explicitly retain/discard partial
data, search/filter history, remove metadata, open completed files and reveal them.
Settings persist range workers, simultaneous jobs, per-host requests, bandwidth,
retry count and notification preference. Generated bindings project safe snapshots
at a maximum four refreshes per second. Unknown sizes are indeterminate.

Tray controls show, run, pause all and safely quit. Close keeps work running;
Linux without a tray host minimizes instead. Quit checkpoints/joins before database
closure. Sleep pauses active work, wake allows overdue queued schedules, and
interrupted jobs require explicit resume. Absolute UTC one-time schedules persist;
the UI names the local timezone. Application-closed execution and system wake
alarms are not implemented. See [ADR 0005](decisions/0005-m4-desktop-session.md).

## Deterministic evidence

Manager/session fixtures exercise settings validation/persistence/reopen, queued
and active commands, reservation-before-worker mutation rejection, cancellation,
retained-partial removal rejection, completed-file preservation, scheduled
not-before behavior, queue exclusion of future schedules, UTC schedule reopen,
sleep/wake, pause-all admission hold, failure presentation and worker joining
before shutdown. Existing sequential/parallel corruption and process-kill tests
remain active and unchanged.

Storage fixtures reconstruct released M2/M3 schemas, preserve their checkpoint
intervals, prove migration-003 rollback/version integrity, and verify settings
read/write/error handling. Old migrations are unchanged. Adapter fixtures check
input bounds, command/path validation, timezone offsets, redaction and the desktop
command workflow against deterministic HTTP responses. React interaction tests
cover add/schedule/checksum, chooser failure/cancellation, filters/search, queue
commands, disconnects, settings, unknown-size progress, retry/schedule information,
explicit retention, restart confirmation, completed-file actions and polling cleanup.
All new handwritten frontend modules are included in per-file coverage.

## Local validation

The following passed locally with pinned toolchains on Linux amd64:

- `go run ./tools/quality doctor`
- `go run ./tools/quality check`
- `go run ./tools/quality check:full` (race, integration, Gitleaks, govulncheck and pnpm audit)
- `go run ./tools/quality task desktop-smoke`
- `go test -race -count=1 ./internal/manager ./internal/storage ./internal/desktop`
- `go test -count=1 ./cmd/desktop ./internal/desktop ./internal/manager`
- `git diff --check`

`go run ./tools/quality gate-probes` also passed all 20 rejection/integrity checks.
Statement coverage: engine 95.7%, manager 92.8%, storage 91.3%, desktop 95.0%.
Existing critical 90% and other 80% thresholds remain unchanged. Every new
handwritten frontend module meets 80/80/80/75 per-file thresholds. Gitleaks and
local-target govulncheck reported no findings. pnpm audit retained the existing
reviewed development-only picomatch applicability exception until 2026-12-01;
raw findings remain visible.

Desktop JS/CSS: 258,792 raw bytes (budget 303,185), 83,314 gzip bytes (budget
103,429). Linux amd64 native desktop executable: 24,570,856 bytes with the normal
trimpath build, including embedded frontend assets. This is an executable-size
measurement, not installer size, startup/RSS measurement or a Windows toast cost
measurement. No budget, exclusion or required CI job was weakened.

## Native interactive evidence

Fedora Linux 45 amd64, GTK 4.24.1, WebKitGTK 2.54.2, GNOME Wayland:

- The production native app launched and rendered the real desktop UI. Screenshot
  capture and scoped AT-SPI extraction worked. Native computer-tool text entry
  could not reliably enter text under the configured multi-layout keyboard;
  keyboard settings were left unchanged.
- The test-only native harness exercised settings, URL/destination/checksum
  entry, queue start, pause/resume, verified completion, history and metadata-only
  removal through the actual WebView and Wails bindings, against a local fixture.
- No StatusNotifierWatcher was present. The close hook selected taskbar minimize;
  GNOME compositor inspection independently reported the app `hidden: true` and
  `focused: false`. GTK's minimized flag remained false on this Wayland session.
  The automated harness therefore checks focus yield there, with this independent
  compositor observation as the actual minimize evidence.

`task desktop-smoke` passed against the native app and checked the fixture's
exact SHA-256 after history removal. The production build omits the tagged driver.
Hosted native evidence is recorded on the M4 PR's final checks. Physical
sleep/wake delivery, Windows/macOS interactive tray/dialog/notification behavior,
installers, signing and older OS compatibility are not established by Linux tests
or native CI builds alone.

## Limits

M2/M3 local-filesystem ownership, Windows directory-durability and power-loss
limitations remain. Progress reports committed windows; scheduled jobs wait while
a run/drain is active. New queue additions require another drain. Pause all holds
schedule admission until an explicit run or relaunch. Failed/paused jobs never
silently auto-resume. M4 upgrades the database to version 3; older binaries cannot
open it. Native notification availability depends on OS permission/environment;
unsigned macOS builds can be unavailable and show the history fallback. Speed/ETA,
batch import and richer category polish remain beta follow-ups. Browser download
handoff is M5; media remains M7/M8. No performance speedup or power-loss claim.

## Hosted regression correction

Initial run 44217616398 built successfully on Windows, then exposed an M3
`TestRetries/cancel_wait` fixture race: its 30 ms deadline could expire before
the first HTTP request, yielding zero requests instead of testing cancellation
during backoff. The fixture now observes persisted `retry_wait` before cancelling,
uses a long server Retry-After within the supported automatic window, and retains
exactly-one-request and paused-state assertions while additionally requiring
`context.Canceled`. A bounded five-second setup deadline fails if the intended
state is never reached. This changes test synchronization, not transfer behavior,
retry policy, thresholds or required jobs.
`go test -race -count=25 -run '^TestRetries$' ./internal/manager` passed, followed
by another full canonical check. Final-head hosted checks remain required.

## Review correction

Automated review found that persistent “Command accepted” feedback took priority
over later manager failures and lifecycle updates. Command feedback and
authoritative session status now render separately. A React regression admits a
queue command, then delivers a later manager failure, running state and sleeping
state, requiring each to remain visible alongside the admission acknowledgement.

A later full integration rerun exposed a SQLite cancellation race: database/sql
can return `sql.ErrTxDone` after its automatic rollback instead of the cancelled
context cause. Storage now preserves both errors so manager/CLI interruption
classification remains correct. A controlled SQL driver deterministically models
that error, and 20 repeated race-enabled regression runs plus 20 real CLI process
interruption runs passed. The transaction failure is retained; successful commits
are never reclassified solely because cancellation happened afterward.

After both corrections, `check:full` and the real native WebView smoke passed
again locally. Final hosted checks independently validate the updated head.

Two additional review regressions cover asynchronous admission and notification
timing. Individual Pause now cancels and joins a reserved individual run before
acknowledgement, including when its worker has not entered the manager. Queue
completion callbacks now run after each job's verified finalization, rather than
after the entire drain. A deterministic admission barrier verifies no HTTP
request follows an acknowledged pause; a stalled-peer fixture requires the small
file's notification while the drain remains active and rejects interrupted-job
and duplicate completed-job notices. Both passed 20 race-enabled repetitions.

After these lifecycle corrections, `doctor`, `check`, `check:full` and
`task desktop-smoke` passed again locally. Final-head hosted checks are required
before protected merge.

## Protected acceptance

PR #6 merged as `02e7fb3133aa04b52dbee244662a6a38efa75c0d` on 2026-11-18.
Final code head `12cb5bcec99e631a49301c914cbad853a42e153d` passed
[Quality run 44288012297](https://github.com/Mahmoud-s-Khedr/idm-enhanced/actions/runs/44288012297),
including full quality, gate probes, history secrets, all three native jobs and
Required checks. M4 is accepted; its interactive platform limitations remain.
