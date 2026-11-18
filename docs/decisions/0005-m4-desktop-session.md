# ADR 0005: Desktop sessions, schedules and native lifecycle

Date: 2026-11-18

Status: Implemented; acceptance requires M4's protected merge and recorded checks.

## Ownership and contracts

The Wails service projects redacted job snapshots and forwards commands to a
manager-owned Session. The frontend does not open SQLite, schedule jobs, implement
retries or own transfer workers. Generated Wails bindings remain the contract;
source URLs (including signed queries), allocation tokens, validators and partial
paths are excluded from presentation snapshots. Filename, destination, hostname,
state, committed bytes, known length and safe error categories are presented.

Session serializes asynchronous run admission, pause/join, settings and shutdown.
One explicit run or queue drain owns the session at a time; simultaneous downloads
within a drain still use M3's configured active-job limit. New enqueues await the
next drain. A reserved run blocks destructive mutations before Manager.Run has
entered, preventing an admission/removal race. Existing manager paths retain
ownership of retries, checkpoints, identity validation and exclusive publication.
Storage preserves the context cancellation cause alongside database/sql's
automatic-rollback ErrTxDone so interruption is still classified as a pause.
Queue cancellation/removal/reordering require Pause all and completion of the join.
Individual Pause cancels and joins a reserved individual run, including the gap
before manager admission, or cancels an active queue job / pauses an idle queued
job. Acknowledged pause cannot be overtaken by that reserved run.

Removing history never deletes a completed output. Unfinished retained partials
must be explicitly discarded through cancellation before removing their record.
Duplicate destinations among unfinished, non-cancelled jobs are rejected before
allocation; final publication still refuses existing destinations.

## Persistence and schedules

Migration 003 adds one settings record without modifying migrations 001/002.
Settings are bounded by Manager.Configure and saved in the protected database;
save failure restores the previous configuration. Schedules use a backward
compatible ScheduledAt field in each v1 job record. M2/M3 binaries reject the newer
schema; read-only access to their pre-upgrade schemas remains supported by M4.

The desktop accepts RFC3339 instants with timezone offsets. The UI converts the
user's local date/time to UTC; subsequent timezone or daylight-saving changes do
not reinterpret that instant. A one-second manager clock admits due scheduled
queued jobs while idle, in queue order. Scheduled runs are serialized; they wait
for an active explicit run/drain to finish. A manual queue drain excludes future
schedules. Unscheduled jobs never automatically start at application launch.

Overdue scheduled jobs become eligible after relaunch/wake. Sleep cancels and
joins active work; interrupted jobs remain paused for explicit resume, rather than
being automatically restarted. No system wake alarm, sleep inhibition, background
service or execution while the application is closed is implemented. Pause all
holds further scheduled admission for that session until an explicit run; this
hold is intentionally not persisted across relaunch. Clearing a schedule returns
it to manual queue control. Basic schedules are one-time starts, not recurrence.

## Presentation and operating-system boundary

The frontend polls serially at most four times per second, stopping on unmount.
Progress is durable committed bytes; parallel progress retains M3's window
increments. Unknown lengths remain indeterminate. Speed/ETA, batch import and
richer categories remain follow-up beta polish; no fabricated estimates appear.
Native destination selection always targets a new file; collisions are rejected,
with selection of another path available instead of overwrite. Open/reveal paths
are resolved from completed manager records, never accepted as arbitrary frontend
commands. Light/dark styles follow the OS; labels, keyboard focus, status text,
search, active/history filters and explicit partial retention are provided.

Window close cancels destruction and hides to the tray. On Linux the existing
D-Bus package checks for Wails' StatusNotifierWatcher before hiding; without a
host, close minimizes to the taskbar and explains the fallback. This check is
repeated at close. A tray disappearing after a successful check remains a desktop
environment race; a separate background service is not introduced. Explicit Quit,
OS shutdown and signals use Wails shutdown to checkpoint/join and release SQLite
ownership. System sleep/wake hooks forward to Session; actual OS power-event
coverage is separate from deterministic lifecycle tests.

Optional native notifications use Wails' existing notification service. Each
queue worker delivers its completion callback after verified finalization and
before returning to the drain; a stalled peer cannot defer completed-job notices.
Callbacks are joined with the run, and reopening a completed job does not repeat
a notice. OS
authorization is an explicit settings action; completion never opens a permission
prompt while shutdown is joining work. Failure
or denied authorization is visible in the app; completion/history remains
available. Notification text contains no filename, source URL or request metadata.
Unsigned macOS builds may lack native notification support; signing remains M6.

## Dependency and test strategy

No new independently selected transfer/storage/frontend runtime package is added.
The already pinned github.com/godbus/dbus/v5 v5.2.2 becomes a direct dependency
for Linux tray-host detection (D-Bus protocol is not implemented from scratch).
It is maintained upstream, BSD-2-Clause licensed, and was already in Wails' runtime
graph. Wails notifications activate its pinned transitive
git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3 on Windows: toast delivery, Unlicense OR
MIT (use MIT), depending only on the existing go-ole and x/sys packages. Platform
notification compatibility and updates remain coupled to the pinned Wails
release. This dependency is preferable to shell command interpolation, custom
native toast code or another notification library. Native executable size and
frontend bundle measurements are recorded in M4_EVIDENCE.md; no budgets change.

A desktop_smoke build tag embeds a DOM driver only in explicit test binaries.
Production binaries contain a no-op registration function and no driver, smoke
event handler or environment-controlled script execution. The driver operates
the real built React UI and generated Wails bindings against a local HTTP fixture;
it exercises the same native shell and close/shutdown hooks. Its single raw Wails
runtime event envelope reports test results, not application commands. This is a
test boundary, not an additional job implementation. On Wayland the harness checks
focus yield because GTK does not reliably report minimized state; independent
compositor observations are recorded separately. Headless checks do not claim
interactive platform support.

The controlled Go presentation-boundary probe now injects engine-to-frontend
assets instead of engine-to-desktop. Desktop now depends on manager/engine, so
the old sample failed at type checking with an import cycle before depguard could
run. The replacement remains forbidden and compiles independently; depguard must
still be the rejecting analyzer. No architecture rule or gate threshold changes.
