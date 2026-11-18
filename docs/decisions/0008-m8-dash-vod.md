# ADR 0008: Bounded static DASH and local track muxing

Date: 2026-11-19

Status: Implemented; protected acceptance is tracked in [PR #10](https://github.com/Mahmoud-s-Khedr/idm-enhanced/pull/10). The owner authorized
M8 implementation and protected merge, with human beta workflows deferred until
afterward. M7 PR #9 merged as 5c14ec3489c61dadd7b697361731148f3c7f097a after final
head 2c24de51223a9ef31a947f3a3de69b34f0265e06 passed Quality run 44407355708.

## Profile and parser boundary

The media layer uses standard-library encoding/xml and a closed, bounded grammar.
No Go or JavaScript dependency is added. A general playback client would still
require explicit rejection of unsupported download semantics; this narrow profile
keeps those checks reviewable. See the [DASH-IF timing model](https://dashif.org/Guidelines-TimingModel/)
for static availability, timeline and repeat semantics.

Supported MPDs declare type=static and the standard 2011 DASH XML namespace, with
one Period beginning at zero and a finite positive duration up to 24 hours.
Representations contain H.264/AVC video or AAC-LC audio in fragmented ISO BMFF.
Audio-only and video-only presentations work; combined choices select one video
and one audio representation. Alternate audio languages are selectable as explicit
combinations in the desktop and CLI. Empty selection chooses highest combined
bandwidth, retaining manifest order for ties. Language labels are bounded plain
language identifiers. Browser handoff deliberately chooses the default combination.

SegmentTemplate is inherited from Period through AdaptationSet to Representation.
A representation may override attributes and inherit its parent's timeline.
Initialization is mandatory. Media addressing supports Number, Time,
RepresentationID, Bandwidth, escaped dollars and one-digit zero-padding widths.
Fixed duration and contiguous SegmentTimeline t/d/r addressing are supported.
Period duration decimals remain exact rational values (bounded to 128 characters) until the ceiling conversion to integer timescale ticks; floating point is used only for the duration summary. Negative r=-1 repeats are expanded only up to a following explicit, aligned start
or the finite Period end; expansion is bounded before allocating resource lists.
Timelines start at zero and cover the Period without gaps/overlaps. Every track must end exactly at the Period boundary; terminal overlap and fractional ticks requiring clipping are rejected. presentationTimeOffset must be absent or zero.
BaseURL is resolved at each level against the effective redirected manifest URL;
multiple alternatives, credentials, fragments and HTTPS downgrade are rejected.

Live/dynamic MPDs, multiple Periods, encryption/ContentProtection/DRM, XLink,
SegmentList, SegmentBase, byte ranges, nonzero offsets, gaps/discontinuities,
subtitles, multiplexed representations and unknown semantic elements/attributes
are rejected. This is an initial supported DASH subset, not universal extraction.
ProgramInformation and an empty ServiceDescription are permitted metadata; XML
entities/DTDs and foreign semantic namespaces are rejected, never fetched.

Bounds retain M7's 1 MiB manifest, 16 KiB URL, 2 MiB resolved-URI text,
64 representation/combined choices, 2,048 objects, 256 MiB per object and 4 MiB
checkpoint limits. XML adds 16 levels and 16,384 elements. All network transfers
reuse engine.Stream and manager concurrency/bandwidth/retry controls.

## Persistence and lifecycle

DASH is an explicit job kind, never an HLS or file-range job. The existing manager
owns admission, object downloads, checkpoint ordering, cache ownership, retries,
cancellation, verification, publication and history cleanup for both media kinds.
DASH media checkpoint v2 adds contiguous track ends and kinds to the persisted
plan, including each initialization object. Decode rejects incomplete, duplicate
or invalid track partitions. Incomplete resume compares immutable manifest digests,
effective URLs, object URLs and track partitions, then uses M7's object validation.
Complete verified caches can retry muxing offline. File checkpoint v1 and HLS
media checkpoint v1 remain unchanged.

Migration 006 advances the database to version 6 without rewriting existing rows.
This is a compatibility barrier: older binaries reject the database rather than
interpreting a DASH job as a file. Released migrations are unchanged. Tests cover
migration rollback, preserved HLS/file checkpoints and durable browser receipts.

## Process boundary and disk cost

The manager supplies hash-verified owned object readers separately for each track.
Go builds seekable initialization+fragment files inside a random private staging
directory beside the output, protected with the shared native filesystem owner/permission helper (including a user-only Windows DACL), accommodating Windows without Unix-only descriptors.
Each staged input is inspected with bounded ffprobe output and must contain exactly its declared supported track before muxing. FFmpeg receives only those local generated paths, forced MOV input, file-only
protocols, disabled external data references, explicit selected stream maps and
stream-copy MP4 output into a generated private seekable file. Seekable output preserves AAC priming/edit lists; Go then copies it with cancellation and a hard byte cap into the existing owned output. FFmpeg receives a file-size limit, and Go independently rejects oversized temporary output before copying. Staged inputs and final output must match the MPD duration within one microsecond of ffprobe precision. No remote URL/manifest or cache filename is
passed to FFmpeg. See [FFmpeg protocol controls](https://ffmpeg.org/ffmpeg-protocols.html) and [MP4 muxer timing options](https://ffmpeg.org/ffmpeg-formats.html).
The existing bounded subprocess runner, ffprobe validation, output sync, verified
checkpoint and exclusive manager publication remain shared with HLS.

Staging is removed after success or ordinary failure; downloaded owned caches
remain available for retry. Temporary disk space can include the cache, one staged track copy, a private muxed MP4 and the owned assembled output. A process kill during staging/muxing can leave an
unpublished .idm-mux-* directory for inspection; no power-loss cleanup guarantee
is made. Unverified output is never published. FFmpeg/ffprobe remain optional,
separately installed tools and are not bundled. Local real playback evidence does
not establish Windows/macOS interactive playback or signing.
