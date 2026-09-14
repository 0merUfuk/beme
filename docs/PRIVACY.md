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

`beme forget` tombstones (logical forget). Rebuild purges derived copies.
Physical purge from Git history is a distinct, user-owned, irreversible
action with an anti-resurrection tombstone only (FR-055).

## Public/private boundary

The engine repository contains no user data. Real evaluation cases live in
a private deployment directory and never reach public CI (ADR-015).
