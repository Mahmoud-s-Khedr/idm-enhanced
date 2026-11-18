# ADR 0007: Bounded HLS VOD objects and offline assembly

Date: 2026-11-18

Status: Implemented; protected acceptance tracked in PR #9.
M6 PR #8 merged as `0acec4abf9e8fee1b427c427ecffa261bcac4e9b` after final
head `2d7fb98a1175ab363a4d8130705746e122f2de2e` passed run 44342722093.
The owner explicitly deferred human beta testing and authorized moving to M7.
This does not assert that the public R1 release checklist is complete.

## Supported profile and dependency choice

`internal/media` implements a narrow RFC 8216 parser and bounded HTTP analyzer.
The supported input is HLS compatibility versions 1–3, immutable playlists
explicitly declaring PLAYLIST-TYPE:VOD and ENDLIST, with complete MPEG-TS objects.
Master playlists expose URI-fingerprint choices and declared bandwidth/resolution;
empty selection chooses the highest declared bandwidth. A selected child must be
a media playlist, not another master. Codecs, when declared, are AVC (avc1 with a
six-digit hexadecimal profile) and AAC-LC (mp4a.40.2); output inspection enforces
H.264/AAC with at most one video and one audio stream. Audio-only and video-only
streams are allowed. Relative URIs resolve against the effective playlist URL.

KEY other than METHOD=NONE, encryption/DRM, live/event/untyped playlists, MAP/fMP4
input, byte ranges, discontinuities, alternate renditions and unknown EXT
semantics are rejected. This intentionally restrictive profile is not a general
RFC-compliant playback client: the RFC's forward-compatible tag ignoring policy
is unsuitable for silently assembling unsupported semantics. Source content can
still be unsupported even when its filename ends in .m3u8.

No Go/JS dependency is added. Standard-library parsing is small enough for this
explicit subset and its negative fixture matrix. A permissive general HLS client
would not remove the need to validate every supported semantic and resource
identity; broader parser/library selection can be revisited with an expanded
profile. Networking reuses engine.Stream and the manager's shared transport,
redirect policy, body validation, per-host concurrency and global bandwidth cap.
There is no second transfer loop or native-host engine.

## Ownership, persistence and recovery

The manager owns HLS jobs, admission, retries, cancellation, retained data and
publication. Media objects are independently complete files, not byte ranges or
synthetic child download jobs. Schema 005 adds analyzing/assembling states and a
private media_checkpoints table. Checkpoint v1 for ordinary files is unchanged;
media checkpoints have their own version 1. All released migrations stay intact.
Older binaries reject schema 5. Migration fixtures rebuild the actual released
jobs constraint before testing upgrades and transaction rollback.

Ordinary job rows contain only small media progress summaries. Store.Get loads
one detailed checkpoint; Store.List does not load every historical segment plan.
Job/summary/checkpoint writes share a transaction. Metadata deletion cascades to
its media checkpoint, while browser receipts remain deliberately independent.
Read-only legacy file status still works before upgrade.

Each object is allocated with a random ownership token and native file identity.
That allocation is persisted before any response writes. Workers download full
objects with the common engine, sync and close them, then the manager commits
only successful objects after every worker in the bounded window has joined.
Uncommitted bodies are truncated and downloaded again. A partially downloaded
plan must match both original and selected playlist digests/effective URLs on
resume. Cached complete objects are rehashed; strong-ETag objects receive a real
range probe before reuse. Objects without strong validators are redownloaded
when a plan is incomplete. A fully complete plan can retry assembly offline,
including when original signed URLs have expired. Each cached object's ownership,
length and digest are checked again while its bytes feed the assembler.

The shared physical owned-file helpers and finishVerified/publication boundary
serve both files and media; scheduling/checkpoint semantics stay distinct.
Verified output is synced/closed before its verified record commits. Explicit
retry of a retained verified checkpoint finishes publication locally. Before
that commit, cancelled media verification becomes paused rather than stuck in
an active state. Existing exclusive hard-link publication never overwrites a
foreign destination. Existing M2/M3 process-kill and recovery suites remain active.
No new power-loss or Windows directory-flush guarantee is made.

Assembly failure retains all complete objects. Successful jobs retain their cache
until history removal, which deletes only proven owned temporary objects and
metadata; completed output is kept. Explicit restart can abandon unproven files,
matching the existing file policy. Cancellation/discard/removal never deletes an
unproven file or reports successful cleanup of a foreign cache. Interrupted
cleanup is idempotent. Unrecorded allocations may remain for inspection.

## FFmpeg process boundary

FFmpeg and ffprobe are optional separately installed runtime tools, not bundled
libraries or installer payloads. Core file downloads do not require them. Missing
tools leave a retryable failed media job with actionable guidance. Controlled
fake executables cover process errors and cancellation on native test platforms;
the explicit media-smoke task requires real tools and does not skip when missing.

Go streams ordered, verified MPEG-TS bytes into FFmpeg stdin. The child receives
no remote URL, remote manifest, segment filename or shell command. Input is forced
to MPEG-TS with only the pipe protocol; output is stream-copy fragmented MP4 to
an already owned Go file. AAC ADTS-to-ASC is a bitstream framing conversion, not
transcoding. `-xerror` preserves mux failures. FFprobe forces the MOV/MP4 demuxer
and file-only input; exit status, MP4 header, streams, duration and size are checked
before manager verification/finalization. Raw child stderr is discarded rather
than exposing source metadata. Command arguments are arrays. A partial output is
never published when muxing, inspection or full-input consumption fails.

Bounds: 1 MiB playlists, 32 KiB lines, 16 KiB URLs, 64 variants, 2,048 objects,
2 MiB resolved URI text, 4 MiB stored checkpoint, 256 MiB/object. Active jobs,
workers, host slots and byte rate use existing manager limits. Interactive
analysis admits two calls and has a 30-second request deadline; pause/sleep/close
cancel and join analyses. Object transfers and assembly have ten-minute child
contexts; output is capped at twice input bytes plus 16 MiB, probe output at
64 KiB, probe time at 30 seconds. Child threads are set to one and individual
allocations capped at 64 MiB; no whole-process RSS or fixed speedup claim is made.

## Browser and desktop boundary

The additive v1 download `mode: hls` field is generated from Go. Omitting it keeps
M5 file envelopes and receipt fingerprints unchanged. HLS mode participates in
the fingerprint, so a request ID cannot switch job semantics. Older strict hosts
reject the new field instead of silently treating media as a file. The manager
owns MP4 naming/collision policy. Acknowledgement still means durably queued,
not source validation, automatic start, assembly or completion.

Both browsers share explicit HLS link menus and popup mode, with highest declared
bandwidth by default. The desktop can analyze and select a specific variant.
Find media in this tab uses activeTab plus scripting after an explicit action,
without cookies, broad host permissions, continuous observation or interception.
The serialized function is self-contained and inspects at most 64 main-frame
media elements and the last 256 resource timing entries, returning at most 32
HTTP(S) candidates. Only recognized resource URL suffixes are inferred; blob,
iframe/shadow content, session-bound and unobservable sources remain limited.
Candidates stay in popup memory; only an explicitly queued request enters the
existing latest-request retry storage. Detection does not prove VOD support.

MPEG-TS fixture data shares the .ts suffix with TypeScript. Only
`testdata/hls/**/*.ts` is marked binary and omitted from the TS formatter/linter;
no handwritten source, coverage threshold, bundle budget or required job is exempted.

## References

- [RFC 8216, HTTP Live Streaming](https://www.rfc-editor.org/rfc/rfc8216.html)
- [FFmpeg command, formats and bitstream filters](https://ffmpeg.org/ffmpeg-all.html)
- [Chrome scripting and activeTab permissions](https://developer.chrome.com/docs/extensions/reference/api/scripting)
- [Firefox scripting.executeScript and serialization](https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/scripting/executeScript)
