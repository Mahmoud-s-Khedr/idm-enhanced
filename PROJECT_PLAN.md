# Cross-platform download manager — project plan

Status: M0–M4 accepted; M5 implementation complete, acceptance follows PR #7 protected merge; M6 core beta hardening is next

Date: 2026-11-16

Updated: 2026-11-18 — M5 implementation and validation tracked in docs/M5_EVIDENCE.md

Working name: IDM Enhanced; choose a distinct public product name before release

Target: Windows, macOS, and Linux desktop

Repository standards: [Quality policy](docs/QUALITY_POLICY.md) and [agent instructions](AGENTS.md). M0 now supplies canonical tooling, hooks, a desktop shell and browser ping spike. See [M0 evidence](docs/M0_EVIDENCE.md) and [gate rollout](docs/GATES.md) for acceptance status; [M1 evidence](docs/M1_EVIDENCE.md) records the sequential CLI. [M2 evidence](docs/M2_EVIDENCE.md) covers persistence/recovery and [M3 evidence](docs/M3_EVIDENCE.md) covers parallel transfers, queues and shared limits; M4 desktop workflows are implemented with acceptance tracked in [M4 evidence](docs/M4_EVIDENCE.md); M5 browser handoff and alpha installers are tracked in [M5 evidence](docs/M5_EVIDENCE.md); media remains planned.

## 1. Project direction

Build a modern download manager with our own Go transfer engine, a Wails desktop application, browser integration, and staged support for downloading media. The engine must work without a desktop window or browser extension.

The central promise is trustworthy downloading: understandable failures, safe resume, predictable resource use, and recovery after interruption. Multiple connections and a modern interface are baseline features rather than proven differentiation. Validate the product with people downloading large files over unreliable connections.

This plan supersedes the Rust/Tauri/aria2 recommendation in the earlier architecture report. We will use networking and media libraries, but own download orchestration, scheduling, checkpoints, and recovery. We will not implement TCP, TLS, HTTP, or a media muxer from scratch.

### Accepted decisions

- Go for the custom engine and application backend.
- Wails with React and TypeScript for the desktop interface.
- SQLite for persistent application and download metadata.
- Browser extensions communicating through a Go native-messaging host.
- HTTP/HTTPS first, followed by direct media, HLS video on demand, and DASH.
- FFmpeg for compatible media assembly and muxing.
- Local operation without mandatory accounts or a cloud backend.

### Proposed defaults requiring review

- Develop primarily on Linux; validate Windows and macOS from the first milestone.
- Start extension support with Firefox, then Chromium browsers before the core public beta.
- Use Wails v3.0.0 is verified and pinned for the M0 platform spike. Packaging remains later work.
- Make FFmpeg an optional dependency until the media release.
- Keep downloads running when the window closes by leaving the application in the tray; explicit Quit stops downloads safely.
- Start with a local application process, not a separately installed background service.

## 2. Release scope

| Release               | User outcome                                                      | Included                                                                                                              | Deferred                                         |
| --------------------- | ----------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ |
| R0: Engine alpha      | Download and safely resume files from the CLI                     | HTTP(S), single stream, ranges, bounded parallelism, persistence, retries, checksums                                  | Desktop, extensions, media manifests             |
| R1: Core desktop beta | Manage downloads from a desktop app and supported browser actions | Queues, bandwidth caps, history, tray, basic scheduling, Firefox and Chromium explicit handoff, direct MP4/WebM links | Universal interception, HLS/DASH, cloud features |
| R2: Media beta        | Discover and save supported HLS on-demand media                   | Browser media candidates, quality/audio selection, supported HLS profiles, FFmpeg assembly                            | Live recording, broad site-specific extraction   |
| R3: Expanded media    | Save supported DASH on-demand media                               | Explicit DASH profile support, track selection, audio/video muxing, selected subtitle support                         | Full DASH coverage and DRM workflows             |

Do not make R1 depend on completing R2 or R3. Unsupported sources must produce clear messages and preserve browser fallback where available.

### R1 features

- Add a URL, drag a link, or paste/import a batch of URLs.
- Pause, resume, cancel, retry, reorder, and remove jobs.
- Configurable simultaneous jobs and per-host transfer limits.
- Global bandwidth cap; configurable range-worker count with conservative defaults.
- Destination selection, safe filenames, duplicate awareness, and collision choices.
- Progress, transferred bytes, total size when known, speed, and estimated time when meaningful.
- Persistent jobs and history, search, filters, and basic file categories.
- Immediate or scheduled start; define timezone and sleep/wake behavior.
- Tray controls, completion notifications, open file, and reveal in folder.
- Expected SHA-256 verification when the user supplies a trusted checksum.
- Explicit browser handoff with acknowledgement and clear failure feedback.
- Safe handling of a deliberately supported subset of authenticated requests.

### Later features

Folder rules, per-job bandwidth limits, validated replacement URLs, proxy settings, broader authenticated-download support, selective automatic browser capture, diagnostics export, and richer schedules follow evidence from beta users.

Torrents, FTP/FTPS, SFTP, HTTP/3, live recording, mobile apps, cloud sync, remote access, plugin execution, and universal site extraction are outside the initial releases. Preserve architectural flexibility without implementing empty frameworks for all of them.

## 3. Technology choices

| Layer          | Choice                                               | Notes                                                                             |
| -------------- | ---------------------------------------------------- | --------------------------------------------------------------------------------- |
| Engine/runtime | Go                                                   | Pin a supported toolchain during setup                                            |
| HTTP transport | Standard `net/http`                                  | Reusable transports, TLS verification, explicit timeout and redirect policies     |
| Concurrency    | Goroutines, `context`, bounded channels/semaphores   | Job cancellation and bounded resource consumption                                 |
| Desktop shell  | Wails v3.0.0, M0 validation                          | Verify tray, events, bindings, installers, and shutdown on each OS                |
| Frontend       | React, TypeScript, Vite                              | Generated bindings or typed adapters around Wails                                 |
| Styling        | Tailwind CSS and accessible UI primitives            | Consistent keyboard, focus, and status behavior                                   |
| Storage        | SQLite via `database/sql`                            | Proposed driver: `modernc.org/sqlite`; validate toolchain/platform compatibility  |
| Migrations     | Numbered SQL migrations                              | Keep schema changes explicit and test upgrades                                    |
| Logging        | Go `log/slog`                                        | Structured local logs with credential and URL redaction                           |
| Extension      | TypeScript WebExtensions                             | Shared logic with browser-specific manifests and adapters                         |
| Native bridge  | Small Go executable                                  | Length-prefixed JSON over stdin/stdout; diagnostics only on stderr                |
| Media          | Go manifest parsers plus FFmpeg                      | Evaluate maintained parsers against fixtures before selecting libraries           |
| Verification   | Go tests, `httptest`, race detector; frontend checks | Protocol failures and restart behavior are primary test targets                   |
| CI/releases    | GitHub Actions on Linux, Windows, macOS              | Native builds and install checks; signing credentials introduced for distribution |

No Node.js server, Redis, PostgreSQL, or hosted download proxy is required. Frontend build tools are development dependencies. Download bytes travel directly from the source to the user's machine.

## 4. Architecture and module boundaries

```text
Desktop UI ── Wails bindings/events ──┐
CLI ────────────────────────────────┤
Browser extension                  │
  └─ native host ── local IPC ──────┤
                                    ▼
                          Application manager
                      jobs / queues / schedules
                         │                │
                         ▼                ▼
                    Go engine          SQLite
               HTTP transfer workers   metadata/checkpoints
                         │
                         ▼
                   Partial files → completed files

Media analyzer → selected track/segment plan → shared transfer facilities
                                              └→ FFmpeg → final media file
```

- **Engine:** HTTP behavior, resource identity, byte ranges, retries, disk writes, and transfer progress. No Wails dependency.
- **Manager:** durable jobs, user commands, scheduling, limits, state transitions, and event snapshots.
- **Storage:** migrations, transactional updates, checkpoints, and recovery reconciliation.
- **Desktop adapter:** narrow validated commands and throttled progress events. The UI cannot directly mutate engine state or database rows.
- **Native host:** validates browser messages and forwards them to the existing application. It never starts a second engine for each message.
- **Media layer:** discovery analysis, manifest parsing, track selection, resource plans, and assembly. File ranges and media segments share transport facilities but retain different scheduling/checkpoint semantics.

Use a single Go module initially. Keep engine packages independent of desktop dependencies; an exported library can be extracted when there is an actual external consumer.

### Proposed repository layout

```text
cmd/
  downloader/               # CLI
  desktop/                  # Wails entry point
  native-host/              # Browser native-messaging adapter
internal/
  engine/                   # HTTP transfers, range scheduling, retries
  manager/                  # Jobs, queue, schedules, limits
  storage/                  # SQLite repositories and migrations
  ipc/                      # Local application bridge
  platform/                 # Tray/lifecycle/filesystem OS adapters
  media/                    # Added during media milestones
frontend/
extensions/
  shared/
  firefox/
  chromium/
testdata/                   # HTTP and media fixtures
integration/                # Restart, crash, browser, packaging scenarios
docs/                       # Design decisions and support matrix
```

Keep this as a target layout; create modules when their milestone needs them.

## 5. Engine correctness requirements

### HTTP and resource inspection

1. Support streaming downloads when size is unknown; do not require a successful HEAD request.
2. Probe range behavior with an actual request and validate responses. `Accept-Ranges` alone is insufficient.
3. For segmented transfers, validate `206`, exact `Content-Range`, expected body length, and consistent representation identity.
4. Never append a full `200` response into a partial range. Stop conflicting workers and restart safely as a sequential transfer when appropriate.
5. Handle `416` by reconciling resource length and state; do not assume it proves completion.
6. Use suitable validators such as strong ETags and `If-Range`. A matching filename or size does not prove identical content. With insufficient identity evidence, prefer restart to unsafe cross-session resume.
7. Keep representation encoding consistent across range requests. Avoid transparent decompression changing byte offsets.
8. Bound redirects, retries, response sizes for metadata, concurrent requests, and queued work. Respect `Retry-After` when applicable.
9. Distinguish temporary network failures from authentication failures, missing files, disk errors, and permanent protocol errors.
10. Treat parallel requests as concurrency; HTTP/2 requests may share a connection. Benchmark before promising acceleration.

### Files and checkpoints

- Stream into a job-owned partial file, preferably beside the destination on the same filesystem.
- Write disjoint byte ranges using explicit offsets. Prevent overlapping worker ownership.
- Record committed byte intervals or per-segment offsets, not only an aggregate byte counter.
- Establish an ordered checkpoint protocol: coordinate writes, flush the corresponding file data, then commit progress metadata. Database state must not claim bytes that have not been durably written.
- On restart, reconcile partial-file state with checkpoints and resource identity; safely redownload uncertain intervals.
- Version checkpoint formats and migrations.
- Finalize only after required ranges are complete and requested checksum verification succeeds.
- Use platform-appropriate same-filesystem finalization, collision handling, and directory durability steps. Recover if a crash occurs between file finalization and marking the database job complete.
- Test forced process termination separately from power-loss resilience; do not claim the latter based only on process-kill tests.
- Pause preserves partial data. Cancel stops work and asks whether to retain partial data; removing history never silently deletes a completed file.

### Job state and persisted data

Core states: `queued`, `inspecting`, `downloading`, `pausing`, `paused`, `retry_wait`, `needs_attention`, `verifying`, `completed`, `failed`, and `cancelled`. Media adds `analyzing` and `assembling`.

Explicitly define legal transitions and make repeated commands safe. On startup, active jobs become recoverable queued/paused jobs after reconciliation; they do not remain stuck as running.

Persist jobs, destination and partial paths, source identity metadata, segment checkpoints, retry state, schedule, timestamps, and structured error categories. Store sensitive authentication references separately from ordinary metadata. Keep high-frequency progress in memory and persist through controlled checkpoints.

## 6. Browser integration

### Initial workflow

1. User selects “Download with [app]” from a link context menu or extension popup.
2. Extension collects URL, suggested filename, source page, and permitted request metadata.
3. Native host validates the versioned message and forwards it over local IPC.
4. Manager validates the request, persists the job, and returns an acknowledgement with a job ID.
5. Extension reports acceptance or an actionable failure.

Use a request ID for deduplication: retries after a lost acknowledgement must not create duplicate jobs. If the application is closed, support a controlled launch or explain how to open it.

### Automatic capture and authenticated requests

- Explicit handoff comes first. Automatic capture is a separate, opt-in feature with a documented compatibility matrix.
- Chrome MV3 permits request observation with permissions but restricts blocking interception for ordinary extensions. Do not promise capture of every download.
- Browser download events occur after creation. Do not cancel a browser transfer merely because a message was sent; require accepted durable handoff and a recoverable failure path. Race conditions and one-use URLs need dedicated tests.
- Cookies, authorization headers, and referrers are handled only when required and with appropriate browser permissions. Scope them to their source and prevent leakage on cross-origin redirects or manifest child requests.
- Treat POST downloads, browser-generated Blobs, expiring URLs, and session/device-bound requests as explicit compatibility cases. Unsupported requests stay in the browser where possible.
- Native-host installation, registration, update, and removal must work independently for Firefox and Chromium on each OS.

## 7. Media support

### Direct media

Discover permitted MP4/WebM/audio resources through media elements and network metadata. Pass usable HTTP URLs to the normal engine. A `blob:` URL is not automatically a remotely downloadable source; inspect underlying resources where accessible.

### HLS video on demand

Analyze master/media playlists, resolve relative URLs, expose available qualities and audio tracks, and download resources in bounded parallelism while preserving playback order. Track media-object completion independently from HTTP file ranges.

Start with a documented, tested subset of unencrypted VOD. Add initialization segments, byte-range resources, alternate audio, and discontinuities deliberately. Unsupported playlist features must be rejected clearly rather than producing a corrupted file. Expand encryption support only as a separately specified capability; do not conflate ordinary segment encryption with DRM.

### DASH video on demand

Start with a documented static-MPD profile and explicit support for its addressing modes, initialization data, timeline, and audio/video representations. Broader MPDs and multi-period content require additional work. Avoid claiming general DASH support from one fixture.

### Assembly and recovery

Use FFmpeg for compatible stream-copy muxing. Choose a compatible container or explain the limitation; avoid automatic transcoding in the initial media release. Validate exit status and output before cleanup, retain downloaded resources if assembly fails, and allow retrying assembly without downloading everything again.

Track selection, authentication context, manifest identity, resources, and final assembly state belong to a persistent media job. Detect unsupported DRM-protected workflows and report the limitation. Site-specific extractors and live recording are separate future projects.

## 8. Desktop experience

- Main layout: category/filter sidebar, searchable download list, toolbar, and optional details panel.
- Add-download dialog: source, destination, filename, queue/start choice, and advanced request options when supported.
- Details: progress, errors, retry state, source host, destination, and connection/segment information.
- Media dialog: available quality, audio, subtitle, and output-container choices.
- Settings: folders, connection limits, bandwidth, schedules, browser setup, notifications, and lifecycle behavior.
- Show indeterminate progress for unknown sizes; avoid fabricated ETAs.
- Support keyboard navigation, visible focus, accessible status text, dark/light themes, and non-color-only status indicators.
- Throttle UI progress snapshots, initially around 4 updates/second per visible job; measure under load rather than emitting an event for every network chunk.

## 9. Security and distribution requirements

- Validate URLs, commands, paths, filenames, message sizes, and engine limits in Go.
- Accept HTTP(S) for remote downloads initially. Block arbitrary shell commands and unsafe path traversal; avoid overwriting unrelated files or following unsafe destination symlinks.
- Keep remote pages and scripts outside the privileged desktop UI.
- Native messaging permits only registered extension identities; local IPC uses OS access controls and an authenticated handshake appropriate to the transport.
- Prefer Unix-domain sockets on Unix and named pipes on Windows. A loopback transport is acceptable only with explicit authentication and restrictive exposure.
- Redact cookies, tokens, authorization headers, signed URL queries, and sensitive referrers from logs. Treat saved source URLs as potentially secret data.
- Use OS credential storage when persistent secrets are necessary; define expiry and deletion behavior.
- Invoke FFmpeg using argument arrays, never shell interpolation; handle process cancellation and output paths safely.
- Build and smoke-test on each OS. Go compilation alone does not establish that a Wails application can be packaged or installed there.
- Define minimum OS versions, Linux distro/WebView requirements, architecture support, installer formats, and signing/notarization requirements during the platform spike.
- Initial beta may use manual updates; public installers require integrity checks and a documented update/recovery process. Signed automatic updates are later work if not ready.
- Review licenses for libraries and the actual distributed FFmpeg build. Product name and visual identity should be distinct before public distribution.

## 10. Milestones and acceptance gates

Estimates assume one developer working roughly full-time. They are planning ranges, not delivery commitments; part-time work and platform access increase calendar time. Re-estimate after M2 using observed progress.

| Milestone                     | Estimate  | Deliverables                                                                                                                                     | Acceptance gate                                                                                                                                                          |
| ----------------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| M0: Foundation/platform spike | 1–2 weeks | Go module, pinned tools, hooks, canonical checks, required CI gates, duplication/architecture rules, design records, tiny Wails shell, OS matrix | Active quality checks demonstrably reject controlled failures; inactive gates tracked; shell launches on each available target; missing hardware/signing access recorded |
| M1: Sequential CLI            | 1–2 weeks | Stream-to-disk transfer, cancellation, progress, errors                                                                                          | Correct checksum for known/unknown-size files; bounded memory; clean cancellation                                                                                        |
| M2: Persistent safe resume    | 2–3 weeks | SQLite, checkpoints, resource validators, recovery                                                                                               | Relaunch resumes safely; changed resources and ignored ranges never silently corrupt output                                                                              |
| M3: Parallel engine/manager   | 2–3 weeks | Segmented transfers, queues, concurrency/bandwidth limits, backoff                                                                               | Failure matrix passes; limits hold across jobs; race detector passes supported suites                                                                                    |
| M4: Desktop alpha             | 2–3 weeks | UI workflows, history, tray, schedules, notifications                                                                                            | Core workflows work without CLI; close/quit/sleep behavior is explicit and tested                                                                                        |
| M5: Browser integration       | 2–3 weeks | Native host, Firefox then Chromium explicit handoff; Debian/Fedora/Windows alpha installers (user-approved M5 scope expansion)                   | Registration works on target OS/browser matrix; unavailable app and duplicate messages are handled                                                                       |
| M6: Core beta hardening       | 2–3 weeks | Installers, recovery fixes, accessibility pass, user testing                                                                                     | R1 release checklist passes; core fixtures and platform smoke tests are green                                                                                            |
| M7: HLS media beta            | 3–5 weeks | Detection, supported VOD profiles, selection, resumable resource jobs, FFmpeg                                                                    | Fixture outputs play correctly; interruptions and assembly failures recover                                                                                              |
| M8: DASH expansion            | 3–5 weeks | Supported static MPD profiles, track selection, muxing                                                                                           | Published support matrix and playback/recovery tests pass                                                                                                                |

**Planning envelope:** approximately 12–19 focused weeks for R1 and another 6–10 weeks for the planned media stages, excluding major scope expansion and external review delays. The M0 range includes implementing and validating the repository quality foundation. M0 should include a small native-host feasibility spike even though full integration comes in M5.

Do not advance past a correctness gate by compensating with more UI features. Desktop and browser prototypes can inform design early, but shipping gates depend on engine reliability.

## 11. Test and benchmark plan

Use deterministic local HTTP fixtures rather than relying primarily on public download sites.

| Area            | Required scenarios                                                                                              |
| --------------- | --------------------------------------------------------------------------------------------------------------- |
| Basic HTTP      | Known/unknown length, HEAD rejected, redirects, chunked body, TLS verification                                  |
| Ranges          | Correct 206, ignored Range/200, malformed Content-Range, short/long body, 416, inconsistent validators          |
| Source changes  | Changed ETag, same-size replacement, expired or single-use URL, missing validators                              |
| Network/retries | Connection dropped mid-body, stalls, 429/503, Retry-After, cancellation during backoff                          |
| Disk/restart    | Disk full, permission failure, missing partial file, forced termination at checkpoint/finalization boundaries   |
| Scheduling      | Global/per-host limits, pause during active work, queue reordering, schedule after sleep or clock change        |
| Authentication  | Scoped credentials, redirect boundaries, expired sessions, logs without secrets                                 |
| Browser         | App absent/closed, malformed messages, duplicate requests, host registration, browser fallback                  |
| Media           | Relative URLs, alternate tracks, init segments, supported range/timeline features, missing segment, mux failure |

Use unit tests for parsing/state transitions, integration tests for network/filesystem behavior, subprocess tests for recovery, and manual OS/browser smoke tests for installation. Fuzz untrusted parsers and range/message validation where useful.

Benchmarks compare sequential and segmented modes under controlled latency, per-request limits, throttling, and disk speeds. Record throughput, CPU, memory, open connections, recovery overhead, and correctness. Memory should scale with bounded workers/buffers, not file size. Set numeric budgets after the first measured baseline; do not advertise a fixed speed multiplier.

## 12. Core beta release checklist

- [ ] Active quality gates in `docs/QUALITY_POLICY.md` pass; required CI/hosting checks are enforced and remaining exceptions are explicit.
- [ ] No known silent-corruption failure in the supported fixture suite.
- [ ] Pause, resume, process restart, and finalization recovery behave as documented.
- [ ] Unsupported resume/authentication cases explain how to proceed safely.
- [ ] Queue, bandwidth, and per-host limits are enforced outside the UI.
- [ ] Firefox and Chromium explicit handoff pass supported OS checks.
- [ ] Install, upgrade, uninstall, and native-host registration are verified.
- [ ] Credential redaction and local IPC access controls are reviewed.
- [ ] Supported OS/browser/source matrix and known limitations are published.
- [ ] Signing/distribution requirements are satisfied for the intended release channel.
- [ ] Beta users complete representative workflows; failures feed the next iteration.

## 13. Risks and decisions for review

| Decision/risk                               | Proposed handling                                                                  |
| ------------------------------------------- | ---------------------------------------------------------------------------------- |
| Custom engine grows beyond schedule         | Keep R1 HTTP-only and correctness-focused; review estimates after M2               |
| Cross-platform UI and native host setup     | Spike early, test natively, keep OS-specific adapters narrow                       |
| No access to macOS/Windows test machines    | Arrange access before claiming support; CI does not replace all interactive checks |
| Competitors already cover baseline features | Validate recovery/diagnostics with actual users; avoid assuming demand             |
| Authenticated downloads are hard to replay  | Support explicit cases and retain browser fallback                                 |
| Media compatibility expands indefinitely    | Publish supported profiles and add fixture-backed capabilities incrementally       |
| Background downloads after UI exit          | Tray-only initially; separate daemon only when justified                           |
| Licensing and business model                | Choose before public distribution; no billing or account infrastructure in R1      |
| FFmpeg installation/bundling                | Decide at M7; document build provenance and update ownership                       |
| Product name                                | Treat current name as internal; select distinct branding for publication           |

Review priorities: confirm the R1 feature boundary, initial OS/browser order, tray behavior, timeline expectations, and license/distribution direction. These are review items, not prerequisites for writing this plan.

## 14. First implementation sprint

After plan review, start M0/M1 with these concrete tasks:

1. Record architecture decisions for engine independence, job lifecycle, checkpoint ownership, and Wails selection.
2. Initialize the Go module and pinned tooling; implement and validate the quality policy's active hooks, canonical checks, CI enforcement, duplication/import rules, and test fixture server. Activate coverage, dependency, and bundle gates as their components arrive.
3. Build a minimal Wails shell and native-host proof of communication on the primary OS; document remaining platform checks.
4. Implement a CLI that downloads one HTTP(S) URL into a partial file with cancellation and progress.
5. Add fixtures for unknown size, ignored HEAD, redirects, interrupted bodies, and cancellation.
6. Verify output against expected checksums and define the M2 checkpoint protocol before implementing resume.

Sprint demo: download a large local fixture without buffering the full file in memory, cancel it cleanly, inspect the result/error, and show the desktop shell and browser bridge feasibility results. Persistent resume is the following milestone, not an implied capability of the first CLI.

## 15. References and verification notes

- [User's shared quality discussion](https://chatgpt.com/share/6afb7ff1-1a28-83ea-8250-5857cc336c47): source for repository automation, agent readiness, and duplication concerns; readable requirements are captured in `docs/QUALITY_POLICY.md`, with retrieval limitations noted there.
- [User's shared project conversation](https://chatgpt.com/share/6afb728e-d750-83ea-9031-78c94b62416f): source for the Go/custom-engine decision and browser/media goals.
- [Wails v3 documentation](https://v3.wails.io/): retrieved 2026-11-16; current page documents Go/web frontend integration, tray support, three desktop platforms, and a v3.0.0 installation command. Exact release pin and minimum platform requirements remain M0 checks.
- [Go HTTP package](https://pkg.go.dev/net/http): implementation reference; verify transport configuration against the pinned Go release.
- [HTTP range and conditional request semantics](https://httpwg.org/specs/rfc9110.html#range.requests): correctness reference established in earlier project research.
- [Chrome native messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging) and [webRequest](https://developer.chrome.com/docs/extensions/reference/api/webRequest): browser constraints established in earlier project research; recheck against target browser versions during M5.

Schedules, scope boundaries, module layout, and estimates are proposed engineering decisions. This plan does not represent implemented or tested software.
