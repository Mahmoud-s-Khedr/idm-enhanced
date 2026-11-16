# Product scope and differentiation

## What already exists and where could this product compete?

### Takeaway
Modern UI, multiple connections, queues, scheduling, browser integration and cross-platform support are already baseline features. Position this as a reliability-focused, local-first download manager with unusually good recovery and diagnostics; validate demand before treating it as a business.

### Cited Findings
- IDM advertises dynamic segmentation, browser handoff, recovery/resume, scheduled queues, categories and bulk page-link downloading. Its acceleration claims are marketing, not a benchmark we independently verified. — [IDM features](https://www.internetdownloadmanager.com/features.html)
- AB Download Manager explicitly offers modern UI and dark mode, multiple connections, queues, scheduler, speed limiter, browser capture and Android/Windows/Linux/macOS availability; it is free and open source. This directly defeats the proposed uniqueness of simply making a modern cross-platform IDM. — [AB Download Manager](https://abdownloadmanager.com/)
- FDM lists parallel acceleration, scheduler, resume, traffic control, torrent support, media preview/conversion, Android remote control, Windows/macOS/Linux/Android and a modern interface. It says free/no ads; its feature page distinguishes the old 3.9 open-source offering from the current generation, so do not call current FDM open source from this page. — [FDM features](https://www.freedownloadmanager.org/features.htm)
- Motrix supports HTTP, FTP, BitTorrent and Magnet; its current homepage advertises task control/prioritization and plugins for naming and link resolution. Do not claim plugins or clean UI are novel. — [Motrix](https://motrix.app/)

### Inferences
- Product hypothesis: people downloading large datasets, software images, models and media on unstable connections could value trustworthy resume, understandable failures, checksum verification and transparent retry behavior. These are proposed priorities, not proven unmet needs or claims competitors lack them.
- Commercial opportunity is niche and crowded. Consumers have strong free alternatives. A generic consumer subscription is difficult to justify; no evidence collected establishes willingness to pay or market growth.
- Prefer open-source free core initially, optional donations/sponsorship; later test a paid desktop Pro license for workflow automation/support or paid remote/team capabilities only if actual users request them. Donations rarely provide predictable income; paid cloud features create operations and support obligations.
- Recruit 10–15 target users using existing tools, collect examples of failed workflows, and ship a prototype addressing the recurring issue. Do not rely on survey enthusiasm alone: ask users to use it for their real transfers.

### Gaps
- No independent speed, reliability or resource benchmarks were performed.
- No willingness-to-pay, total market size or growth evidence collected. Avoid invented numerical revenue forecasts.
- No current competitor paid price comparison collected; free competitors suffice to establish competitive pressure.

## What belongs in the MVP and how should delivery proceed?

### Takeaway
Ship a dependable HTTP(S) desktop product first. Browser handoff belongs in the first public beta; universal video downloading, torrents, cloud sync and a plugin ecosystem should wait.

### Cited Findings
- Core expectations are evidenced by queues/scheduling/limiting/browser integration across [AB](https://abdownloadmanager.com/) and [IDM](https://www.internetdownloadmanager.com/features.html).
- Protocol expansion, media tools and remote control substantially broaden the competitive feature surface, as demonstrated by [FDM](https://www.freedownloadmanager.org/features.htm) and [Motrix](https://motrix.app/).

### Inferences
- MVP: paste/drop URLs and batch import; HTTP(S), safe parallel ranges where supported, pause/resume/retry/cancel; queue concurrency and per-host limits; restart recovery; filenames/destination/conflict handling; size/speed/ETA/state; global and per-download speed caps; history/filter/search; basic categories; notifications/tray; Chrome/Edge/Firefox explicit 'Download with app' action and selected automatic handoff after that works. No account required.
- First follow-up: schedules, rule-based folders/names, duplicate detection, checksums with expected hash comparison, expiring-link replacement with content validation, per-site browser handoff controls, proxy support, CLI/API, useful redacted diagnostics.
- Later optional: authorized non-DRM HLS/DASH/FFmpeg media workflow, remote management, S3-compatible sources/signed-link workflows. Treat mobile and Safari integration as distinct projects; do not equate a desktop framework with production mobile support.
- Defer torrents, whole-site crawling, universal site extraction, DRM-related work, AI features, cloud sync, plugins and extensive post-download automation. Arbitrary post-download command execution also complicates trust/security.
- With an existing transfer engine, estimate 6–10 focused solo weeks for a usable beta: engine integration and persistence 2–3, desktop workflow 2, browser integration 1–2, three-OS packaging/recovery tests/polish 1–3. Estimate only; student part-time availability stretches calendar time. A custom segmented engine plus robust distribution can take 3–6+ months, depending on experience and scope; do not promise IDM parity in weeks.
- Build transfer correctness before decorative UI. Core release checks: disconnect/reconnect, force-kill/relaunch, disk full, server ignores ranges, object changes during resume, authentication expires, filename traversal, downloaded bytes equal expected content. Measure CPU/RAM and single-stream versus segmented transfers under defined network conditions.
- Use a separate identity/name; build an alternative inspired by IDM rather than representing it as an official IDM version.

### Gaps
- Scope estimates are engineering judgments and cannot guarantee release dates or browser-store review timelines.

## How valuable is it for this developer's portfolio and learning?

### Takeaway
Strong systems portfolio/learning project, speculative commercial desktop utility rather than inherently SaaS. It is most useful if it demonstrates recovery, network correctness and measurable engineering results, and if scope is bounded to protect video-infrastructure goals.

### Cited Findings
- Existing alternatives show that feature breadth alone is insufficient differentiation: [AB](https://abdownloadmanager.com/), [FDM](https://www.freedownloadmanager.org/features.htm), [Motrix](https://motrix.app/).

### Inferences
- Suggested subjective scores: market/monetization 4/10, technical challenge 8/10 (difficulty, not desirability), portfolio signal 8/10 if custom recovery/network behavior is demonstrated, learning 9/10. Scores are judgments, not research metrics.
- Strongest demo: download a large file, cut connectivity and kill the app, relaunch and safely resume, then verify the final checksum; explain server behavior and retry history clearly.
- Transferable skills: async Rust, bounded concurrency/backpressure, HTTP semantics, persistence and crash consistency, desktop IPC, browser messaging, OS distribution, CI and observability. Transfer reliability and future non-DRM media workflows align with backend/video clients; general desktop theming contributes less.
- Distinct from the stated existing web/SaaS projects, though overlaps with cross-platform app experience. Avoid turning it into a second broad video platform competing for time with the graduation project.
- If portfolio-first, a custom focused Rust HTTP core provides more engineering signal than a thin aria2 skin, but engine reuse can deliver a real product faster. This is a product-vs-learning tradeoff, not a universal technical answer.
- Costs can remain low for local-first design: release hosting plus OS signing/distribution work, test machines/VMs and maintenance. Do not introduce server proxying of file payloads: bandwidth costs and privacy obligations would be unnecessary. No need for PostgreSQL, Redis, Kubernetes or backend auth in the desktop MVP.
- Next step: fix target desktop OSes and one user pain, implement a failure-recovery technical spike, then trial with users before expanding scope. Timebox the first milestone to 2 weeks; stop or narrow if no advantage over existing tools appears.

### Gaps
- User's exact commercial ambitions, team size, weekly capacity and preferred target audience remain unstated. Recommendations assume a solo/small team targeting Windows/macOS/Linux desktop.
