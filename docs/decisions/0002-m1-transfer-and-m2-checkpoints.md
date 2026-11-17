# ADR 0002: Sequential transfers and the M2 checkpoint contract

Date: 2026-11-17

Status: M1 implemented; M2 protocol implemented by ADR 0003 (hosted acceptance pending)

## Ownership and dependency decision

Extend the existing engine prototype rather than introduce another HTTP loop.
`internal/engine.Transfer` owns HTTP validation, bounded redirects, streaming and
cancellation. `engine.Download` owns one transfer's partial file, verification and
publication. `internal/cli` is a presentation adapter. There are no durable jobs,
queues, or duplicate manager policies in M1. M2 will introduce storage for durable
metadata and a manager for job lifecycle; those layers will call these transfer
facilities rather than recreate them. No runtime dependency is added: net/http,
context, crypto/sha256, and os provide the needed behavior.

M1 uses a random private partial in the destination directory. `Sync`, successful
`Close`, optional checksum verification, and cancellation checks precede `os.Link`.
Hard-link publication is exclusive: it cannot replace an existing file or symlink.
The owned partial link is then removed. Filesystems without hard links fail
explicitly. This avoids a check-then-rename overwrite race and does not imply
crash/power-loss durability. No overwrite option is introduced.

## M2 checkpoint protocol

Define format version 1 with job ID, source identity, destination and owned partial
path, representation encoding, expected length if known, committed disjoint byte
intervals, generation, and finalization phase. Storage owns the SQLite schema,
migrations and transactional commits. The manager owns persisted state transitions.
The engine validates identity and owns byte writes. Adapters never edit checkpoints.

1. Create an owned partial exclusively beside the destination and persist its
   identity/path before scheduling work. Never accept a preexisting unrelated file
   as a partial merely because its name matches.
2. Resume only after a fresh HTTP probe establishes the same representation with
   a strong validator and validated length/range semantics. Use If-Range and
   identity encoding. Missing/weak identity, changed resources, ignored ranges,
   conflicting Content-Range or unexpected encoding trigger safe restart or an
   actionable error, never blind append. A 416 does not establish completion.
3. At a checkpoint barrier, stop/reconcile writes for the intervals being committed,
   flush their file data with Sync, then transactionally commit only those durable
   intervals and the incremented checkpoint generation. A failed sync or DB commit
   must not advance acknowledged durable progress. Memory-only counters may lead
   persisted progress but cannot become recovery evidence.
4. On restart, reconcile file ownership/length and persisted intervals, revalidate
   the remote identity, and redownload any uncertain bytes. Extra bytes in a file
   are not evidence of committed progress. Resume active jobs through explicit
   manager transitions rather than restoring a running state without workers.
5. Before publication, verify completeness and any expected checksum; sync/close
   the partial, persist a verified/publishing phase, publish without replacement,
   and apply platform-specific directory durability steps before persisting
   completed. Retain sufficient identity evidence to reconcile crashes between
   publication and the completed transaction without adopting unrelated files.
6. Pause preserves the owned partial/checkpoint after the barrier. Cancellation
   follows the manager's explicit retention choice. Do not delete downloaded data
   merely when removing a history record.

M2 must fixture-test faults on either side of each persistence/finalization step,
changed resources, ignored ranges, truncated files and process restart. Power-loss
claims require separate filesystem/platform evidence; process-kill tests alone are
insufficient. SQLite driver selection and its dependency review belong to M2.
