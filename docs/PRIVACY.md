# Be Me — Privacy

## What is stored locally

Source descriptors, workspace registrations (paths and identities), derived
projection stores (normalized records + provenance + FTS index), tombstones,
quarantined observations. All on the operator's machine, in
platform-appropriate directories.

## What is resolved and served

A ContextPack contains bounded, provenance-linked record text that passed
Stage-A policy under the bound capability. Work-safe packs contain only
declassified safe records; work-safe provenance omits private source IDs,
repo names, paths, locators, and correlatable hashes.

## What leaves the machine

Be Me itself never sends data anywhere (no telemetry, no network transport
in v1). The *harness* may send a resolved pack to its configured model
provider — including a cloud provider. That downstream use is outside Be
Me's control. Never interpret "local-first" as "data never leaves the
device."

## Learning

Agent feedback becomes a quarantined observation file locally. Observations
are non-normative, excluded from packs by default, and promote to canonical
only through explicit user approval (ADR-010). Unaccepted agent output is
never evidence of preference.

## Deletion

`beme forget` is a logical forget: the record is tombstoned in its projection
and in the durable ledger under the canonical root, so it never resolves
again — even after a rebuild, corrupt-store recovery, or a restored backup.
A rebuild does not erase the content; the source still holds it.

`beme purge --confirm <key> <key>` is the physical purge (FR-055, ADR-027):
a distinct, user-owned, irreversible action. It erases the record — through
every provenance ref it actually has — from both projection stores (rewriting
the files so the bytes do not survive), from persisted traces, and from
pending observations that restate it; with `--remove-canonical` it also
deletes every source file those refs locate. If it fails part-way, re-running
the same command finishes the remaining cleanup; running it again after
completion is a no-op.

What remains is minimal: a keyed HMAC fingerprint of the record's identity
(source ID + record ID) and of the purge key. The ledger holds no content,
no digest of content or text, no readable IDs, and no timestamps, so a copy
of the ledger alone cannot be used to confirm a guess about what was purged.
The HMAC key (`ledger/purge.key`) is stored separately and excluded from Git
by a generated `.gitignore`; back it up with the ledger but never share or
commit it. The fingerprint blocks re-ingestion and resolution when the same
record returns through sync, rollback, rebuild, or a backup restore;
deliberately re-authoring the same words under a new ID is not blocked. Git
history and external backups are outside Be Me's reach: the purge report
lists them as residuals with the remediation.

## Public/private boundary

The engine repository contains no user data. Real evaluation cases live in
a private deployment directory and never reach public CI (ADR-015).
