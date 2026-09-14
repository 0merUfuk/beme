# Be Me — Security Policy

## Reporting

Report vulnerabilities privately to the repository owner via GitHub
security advisories. Do not open public issues for vulnerabilities.

## Security thesis

> The model does not choose its authority. A session capability determines
> its maximum scope. A source does not declare its own authority. The trusted
> ingestion path assigns the authority ceiling. Where privacy matters,
> absence of access is stronger than a deny filter.

Implemented boundaries (contract- and regression-tested):

- Capability-bound serving; requests narrow only (policy tests).
- The request schema structurally lacks elevation fields (schema tests).
- Work-safe projection built only from approved safe-manifest sources
  (construction-level separation; end-to-end leak test).
- Ingestion never executes source code; hard excludes; secret scan; symlink
  containment; size/depth/time bounds (ingestion tests).
- Content self-assigned authority clamps to informational (injection test).
- Workspace registry: real-path matching; clones never inherit trust;
  ambiguity fails closed (workspace tests).
- stdio-only transport (ADR-014); no listener exists in v1.
- Feedback writes are quarantined; promotion is user-owned.

## Honest boundary statement (cooperative-local)

Separate processes, stores, builders, and capabilities provide
application-level least privilege. They are NOT an OS confidentiality
boundary: a same-user process with arbitrary shell/filesystem access can
read any file the OS user can read. `isolated-admin` claims require verified
OS separation (ADR-017) and are not made in v1. A pack sent to a cloud model
leaves the machine — local-first means no Be Me cloud service is required,
not that data never leaves the device (§7.5).
