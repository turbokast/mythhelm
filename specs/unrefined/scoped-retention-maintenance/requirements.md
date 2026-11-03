# Scoped export, retention, erasure and restoration — Requirements

> Unrefined. Run /refine-spec scoped-retention-maintenance before /spec.
>
> Give users scoped control over durable MYTHHELM history through previewed export/import, dependency-aware cleanup, recoverable erasure maintenance and tested restoration. Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0, especially §§4–6, 8–9, 12 and §18; all I/AT/G identifiers below refer to v2. Requirement capture grants no destructive operation, implementation or native execution.

## Context

- **Backlog card**: MH-25
- **Issue**: [#131 — Scoped export, retention, erasure and tested restoration](https://github.com/turbokast/mythhelm/issues/131).
- **Source**: card Summary/Notes and adoption-time grounding; v2 §§5, 8–9, 12; Implementation Plan W13.
- **Grounding revision**: `615e74a6bb6907f87bd2065e2f576f5e18b5dec5`; source claims below were read at this revision.

Users need to export useful evidence, control retention and erase eligible local history without corrupting active work or overstating what was deleted. A scoped snapshot must preserve relationships, provenance, compatibility and explicit omissions. Exporting data does not grant publication authority or make a foreign archive trusted input; restoring old data does not replay processes, revive grants or reverse external effects.

The ledger is append-only while history is retained. Erasure requires an explicit maintenance operation with a stable preview, applicable authority, exclusive writer quiescence, a consistent rewrite and recoverable installation. A logical SQLite delete, checkpoint, vacuum or file unlink alone is not evidence of physical erasure. Backups, exports, native/provider copies and derived learning stores need separate inventories and honest dispositions; locally selected copies cannot be silently forgotten when declaring completion.

This capture extends the existing migration/backup and scoped-contract foundations. Current active-retention checks govern which non-secret admission values may be kept, not general garbage collection or authority to erase. Broader optional learning and context retrieval must consume these guarantees before advertising retention/erasure, but their whole implementation must not become a cycle preventing a useful local maintenance core.

### Premise grounding

- **HOLDS — A general scoped export/GC/erasure CLI remains planned.** The complete command registry in `internal/cli/dispatch.go:31` includes migration/review operations but no export or GC command; W13 and v2 §5.3 define the missing lifecycle. Existing rendered receipts/export helpers are not that lifecycle.
- **PARTIAL — The adoption-time schema-v1 statement is historical.** The current migration set includes `internal/journal/migrations/0010_service_assignment_epochs.sql`, and v2 records exist; `0001_init.sql:26` still rejects normal journal updates/deletes. The missing requirement is a reviewed maintenance contract over current compatible history, not an assumption that the application still has only schema v1.
- **HOLDS — Consistent backup and tested restoration are reusable.** `internal/migrate/backup.go:57` creates a consistent SQLite copy with integrity checking and a digest/schema/build sidecar; `Restore` at line 165 checks compatibility and integrity, stages replacement and refuses nonempty WAL. Its caller-quiescence requirement at lines 154–157 remains essential; the helper alone is not a maintenance owner.
- **PARTIAL — Ownership and preview seams exist for migration.** `internal/migrate/apply.go:81` acquires migration/root/run ownership before mutation, and `internal/migrate/preview/plan.go:88` computes a read-only migration plan. These are migration-specific contracts to extend or compose, not generic permission to replace an active service ledger.
- **PARTIAL — Artifact/retention vocabulary and local binding classification exist.** `internal/v2contract/records_evidence.go:151` defines repository identity, digest, dependencies and retention roots for Artifact. `internal/admission/retention.go:24` checks an exact non-secret binding policy/approval and refuses forbidden provenance at lines 35–50; it has no deletion effect and grants no authority. A complete reachability, storage-copy inventory and derived-provenance invalidation algorithm still needs design.
- **UNVERIFIABLE — Complete copy coverage and recoverable erasure on every platform.** No erasure or restore experiment is run by this capture. Current local-copy inventory, filesystem durability/race boundaries, all active dependency/owner roots, derived-store provenance and external-copy limits require refinement/evidence (OQ-1/OQ-2); no unknown destination is declared erased.

## Objectives

- **O1**: Preview and export/import authorised repository/run history with exact identities, scope, compatibility and missing-data disclosures.
- **O2**: Reclaim eligible local data without destroying active, pinned, referenced or ownership-ambiguous provenance.
- **O3**: Erase selected eligible history through quiescent, crash-recoverable maintenance and explain surviving controlled and external copies precisely.
- **O4**: Restore compatible snapshots without replaying native work, resurrecting authority or corrupting the existing ledger.

## Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Guaranteed SSD overwrite, secure hardware sanitisation, provider/native-history deletion or deleting unknown remote copies. | I09/I19: report unknown/external/uncontrolled copies and manual next actions; never inspect native secrets or claim complete physical/global erasure. |
| N2 | Cloud backup service, remote live SQLite, automatic external uploads or a public sharing service. | I03/I04 and §§5.1/8.4: local export is useful offline; any later destination needs separate preview, authority and truthful copy tracking. |
| N3 | General search/learning implementation or retraining algorithms. | MH-24/MH-27 consume scoped provenance; any enabled derived index/example/model/policy retaining selected data must be invalidated, deleted or rebuilt before affected claims resume. |
| N4 | Force-purging active work, unrelated source checkouts/Git remotes or native account stores. | I05/I08/I12/I19/I24: preserve active ownership, original source and external effects; refuse unsafe scope even with generic confirmation. |
| N5 | Treating imported receipts, model claims or restored grants as current acceptance/authority. | I03/I07/I20: preserve historical evidence labels, revalidate current use and independently reconcile effects. |

## Functional Requirements

### FR-1 — Scoped immutable preview and authority (§§5.1, 5.3, 8.2; I02/I03/I20)

- **AC-1.1** [AT-16/AT-43, G13] When export, GC, erasure or restore is previewed, the system shall authenticate the caller and filter repository/run authority before revealing payload, object names, counts, digests or dependency/index metadata; equal content in another repository shall not imply access or deletion rights.
- **AC-1.2** [I04/I20, AT-12] Before a destructive or destination-writing operation, the system shall bind authority to an immutable plan naming the exact root/repository scope, eligible terminal records, selected local copies/destination, snapshot/policy revisions, retained roots, estimated reclaimed bytes and consequences; stale revision, changed root/copy identity, expired grant or changed dependency reachability shall require a new preview rather than widening an old approval.
- **AC-1.3** [I03/I09] When a dry-run is requested, the system shall produce a bounded consequence report without changing canonical data, starting a native process, mutating account settings or granting authority; estimates and unknown copy coverage shall remain labelled, and opaque hashes alone shall not substitute for a meaningful scoped preview.

### FR-2 — Retention roots and bounded storage (§§4.1, 5.3, 9.2; I07/I12/I20/I23)

- **AC-2.1** [AT-43, G13/G16] Before any object becomes collectible, the system shall compute transitive reachability from active or ownership-ambiguous runs/attempts, pinned artifacts/snapshots, admitted task/dependency/context manifests, handoffs and unacknowledged delivery references, retained verification/acceptance evidence, active policy/learning derivations, unresolved effect/outbox operations and required recovery material; unresolved references or incomplete traversal shall preserve the affected objects and block unsafe cleanup.
- **AC-2.2** [I09/I20/I25] When applying default retention, routine redacted streaming logs shall expire after 14 days and optional learning examples after 90 days unless a shorter explicit user policy applies; raw capture stays off, and receipts/reviewable artifacts remain until explicit cleanup or an explicitly configured rule. Expiry never overrides AC-2.1 or separately required authority.
- **AC-2.3** [I10/I21] If storage or operation bounds prevent safe retention/rewrite, then the system shall stop new admissions or refuse the maintenance operation with recoverable guidance rather than destroy protected evidence, silently truncate history or claim unobserved reclaimed capacity.

### FR-3 — Scoped snapshot export and compatibility (§§5.1–5.2, 8.4; I09/I19/I25)

- **AC-3.1** [AT-41/AT-43] When exporting authorised history, the system shall read one consistent bounded snapshot and emit a versioned manifest with repository/run scope, ledger/schema/build identities, original artifact/content/Git identities, dependencies, provenance labels, checksums and explicit omitted or missing-artifact records; an archive missing mandatory reconstructive inputs shall be marked incomplete, not a complete backup.
- **AC-3.2** [I04/I19, §8.4] Before an export leaves the selected local scope, the system shall preview included sensitivity classes and destination, exclude native authentication/hidden native state and unsupported raw capture, and require separate explicit authority for any supported raw capture or external sharing. Redaction shall not be presented as proof that arbitrary text contains no secrets.
- **AC-3.3** [AT-41, I20] If a requested legacy representation cannot express current safety/provenance facts or the reader cannot safely decode a future schema, then the system shall disclose incompatibility/loss and refuse unsafe conversion or provide a bounded explicitly incomplete read-only representation; it shall never silently drop facts to make an import appear compatible.

### FR-4 — Quiescent maintenance ownership (§§3.1, 5.2–5.3, 6; I05/I06/I12/I18/I24)

- **AC-4.1** [AT-06–AT-09/AT-42] Before ledger rewrite, restore or destructive copy cleanup, the system shall acquire the canonical instance/root maintenance authority, prevent new admissions, reconcile/drain all relevant current and legacy writers and close database handles under the supported ownership protocol; a missing heartbeat, expired lease, empty WAL or terminal display label alone shall not prove quiescence.
- **AC-4.2** [I08/I12/I24] If an owner, filesystem identity, external effect or durable operation is unresolved, then the operation shall retain the original data and recovery records, refuse destructive progress and expose an ownership/maintenance blocker; changing the state-root path shall not create a second authority to bypass that refusal.
- **AC-4.3** [I12/I20] When maintenance is interrupted or repeated, its operation identity, exact plan, phase and staged-copy identities shall determine idempotent reconciliation; replay shall never dispatch old native work, execute imported tool requests, repeat a purge on a different object or release unresolved reservations.

### FR-5 — Validated rewrite and recoverable swap (§5.3; I07/I12/I20/I23)

- **AC-5.1** [AT-43, G16] When erasing eligible authorised terminal history, the system shall rewrite a consistent replacement ledger excluding that selected content while preserving retained records and references, normal append-only guards, independent repository data, schema compatibility and required projections; ordinary journal DELETE/UPDATE or trigger removal shall not become a public erasure API.
- **AC-5.2** [AT-42/AT-43] Before replacement is installed, the system shall validate referential integrity, schema/build compatibility, preserved-object identities, dependency closure, scoped exclusions and derived-state disposition; staging/disk/validation failure shall leave the original ledger usable and record no successful purge.
- **AC-5.3** [I12, AT-42] When installing a validated replacement, the system shall use a tested durable recoverable swap with consistent WAL/sidecar handling and a bounded recovery path; a crash before or after each durable boundary shall recover one identified authoritative ledger, never resume competing writers or discard the last valid copy.

### FR-6 — Derived stores, copies and truthful erasure (§§5.3, 12.4; I09/I20)

- **AC-6.1** [AT-43] When selected data contributes to indexes, derived exports, optional examples/datasets, derived models or promoted policies, the system shall enumerate those local derivations and invalidate/delete/rebuild them as required by their provenance; unsupported or uncertain derivation shall block affected reuse and an unqualified completion claim rather than silently preserve the data in a cache.
- **AC-6.2** [I04/I09, AT-43] When erasure is requested, the system shall inventory known locally controlled copies including migration/maintenance backups, rollback/staging files, exports, snapshots, worker diagnostics/spools and relevant WAL/sidecars; copies shall be individually scoped for purge or explicitly retained with reason. A rollback copy kept for safety remains a surviving sensitive copy until separately and safely finalised under the same applicable authority.
- **AC-6.3** [I09/I19] When reporting erasure, the system shall emit a content-free receipt recording the operation/outcome and surviving-copy classes without erased prompts, paths, account identifiers or erased-content-derived hashes; it shall distinguish selected-local-copy handling, logical removal, unresolved copies and physical/provider/native-history limitations, never equating SQLite deletion/vacuum or unlink with global erasure.

### FR-7 — Safe import and tested restoration (§§5.1–5.2, 6.4; I03/I07/I12/I20)

- **AC-7.1** [AT-41/AT-42, G16] Before restore/import writes the target, the system shall validate version/format, manifest/content checksums, repository/root mapping, required artifact availability and retained-reference integrity in staging; unsupported schema, corrupt/truncated content, traversal, symlink/junction, case collision, duplicate-member or decompression-bound failure shall leave the target unchanged.
- **AC-7.2** [I03/I07/I12] When restoring compatible history, the system shall retain historical identities and evidence labels but treat grants, reservations, native sessions, outbox effects and prior process ownership as historical/unresolved until current authority/effect reconciliation; restored receipt/check success shall not certify a different current candidate or start native work.
- **AC-7.3** [AT-42/AT-43] When testing backup/export restoration, the system shall demonstrate a sanitised round trip preserving authorised records, dependency/artifact identities and explicit omissions, plus failure controls for missing artifacts, newer schema, nonempty WAL/live writer, corrupt checksum, insufficient disk and interrupted replacement; a migration-only database copy shall not be advertised as a complete artifact backup without its coverage manifest.

### FR-8 — User control and discriminating evidence (§§15, 17.2, 18; I07/I25)

- **AC-8.1** [AT-35/AT-43] When maintenance is inspected through CLI/plain/JSONL and supported client views, the user shall see dry-run versus committed disposition, selected scope, protected roots, progress phase, actual or estimated bytes, survivors, blockers and recovery action; an incomplete plan or cancelled operation shall never read as successfully erased/restored.
- **AC-8.2** [I07/I12/I24, G13/G16] Before advertising a platform/filesystem combination, effect tests shall show cross-repository denial, active/dependency/ambiguous-owner preservation, stale-preview refusal, interruption at every swap phase, derived-store invalidation and no native replay; removing a root, weakening scope or skipping validation shall make the corresponding test fail.

## Non-Functional Requirements

- **NFR-1** [I13/I19] Contributor verification uses synthetic temporary roots, ledgers, artifacts and process owners; no agent credentials, inference, external account actions, native-store extraction or production deletion is required. Platform durability claims require separate versioned evidence.
- **NFR-2** [I21, §4.3] One operation is limited to 1 repository, 1,000 explicitly selected terminal runs, 10,000 traversed objects and 100,000 dependency edges; it shall refuse incomplete traversal rather than process an unsafe prefix. Encoded manifests/control envelopes are at most 1 MiB/64 nesting; payload is streamed with at most 16 MiB application buffering, at most 10,000 regular-file members and 1 GiB total expanded input/output per operation, including staged ledger content. Smaller supported-format/native limits win; exceeding these frozen conservative bounds requires a reviewed contract revision, never silent truncation.
- **NFR-3** [I21/I24] Read-only preview has a 30-second deadline; quiescence acquisition waits at most 30 seconds without force-killing an unverified owner; export/import/rewrite/restore each has a 5-minute total deadline including bounded effect-free retries (at most two after 100/500 ms). Recovery has a separate 5-minute safety deadline; if safe completion cannot be established, the durable maintenance state remains fenced against new admissions and the next explicit recovery reconciles it. Timeout never releases unresolved ownership or deletes the last valid copy.
- **NFR-4** [I09/I20, G16] Before mutation the system shall verify sufficient supported local-disk staging capacity for the preserved original, consistent recovery copy, replacement/selected artifact staging and a 64 MiB safety margin; concurrent disk loss remains a tested failure, not a guaranteed reservation. Published compatibility binds archive/DB/event versions, executable build, OS/filesystem and actual integrity/restore evidence; security-sensitive output and destination files use supported owner-restricted permissions.

## Candidate delivery streams for refinement

| Stream | Useful bounded outcome | Dependencies |
|---|---|---|
| A — Scoped inventory, roots and preview | Inspect reachable data, sensitive-copy coverage and safe cleanup consequences without mutations. | Merged contracts, scoped authority and current storage inventory; fixture design can proceed now. |
| B — Snapshot export/import and restore | Local sanitised portability with exact compatibility, artifact completeness and safe staged restoration. | A's frozen manifests plus existing backup/owner primitives; missing context mechanisms remain explicit. |
| C — Retention/erasure maintenance | Authorised quiescent rewrite, recoverable swap, derived invalidation and honest surviving-copy receipt. | A/B, merged canonical-owner maintenance protocol and applicable scoped-context/provenance roots; enabled learning stores supply their own lineage adapters. |

## Dependencies

- **Requirements now; production gates later.** Capture and synthetic design do not assert full S1, scoped-context or maintenance readiness. Production capability requires applicable G13/G16/AT-41–AT-43 evidence and safe authority/ownership on each advertised platform/filesystem.
- **MH-21 and completed migration foundations**: extend `specs/done/supervisor-migration/` and the canonical contract/controller work, preserving historical events and schema/version domains. Backup helpers do not replace maintenance ownership.
- **MH-24 scoped context / MH-15 continuity**: consume actual artifact/dependency/context and acknowledged-delivery roots as their contracts merge. The independent local inventory/export core may be designed before those complete cards; do not form a cycle by requiring optional learning/search before scoped maintenance.
- **Current `supervisor-attachment-control`**: preserve the exact non-secret active-retention policy/binding checks and consume merged authenticated operations/root/generation contracts; structural approval records do not grant maintenance effects. Coordinate source scope with concurrent ownership/admission work before later implementation.
- **Learning and derived stores**: MH-26/MH-27 and later learning/model/policy work cannot advertise complete retention/erase support until their derivations participate in this contract; absent stores need no fictitious deletion, unknown stores do not count as absent.
- **Supersession**: no existing event trigger, migration, backup or historical receipt contract is silently weakened. New maintenance/persistence changes need reviewed architecture/domain-pure tasks, explicit compatibility and restoration evidence. Lifecycle/product sync remains the coordinator's separately approved work.

## Impacted components

- **Canonical contracts/storage**: `internal/v2contract/records_evidence.go`, `records_coordination.go`, `internal/journal/` and additive migrations; artifact roots/provenance and immutable maintenance receipts compose with existing event/operation records.
- **Backup/maintenance ownership**: `internal/migrate/backup.go`, `apply.go`, `drain.go`, `preview/plan.go`, `internal/control/` and canonical supervisor operations; compose tested quiescence, consistent snapshots and compatible restoration without a second owner.
- **Policy/privacy**: `internal/admission/retention.go`, `internal/admissionbinding/retention.go`, scoped context/derived-store integrations and `internal/security/`; local binding retention approval stays distinct from export/erasure authority.
- **Clients/evidence**: `internal/cli/dispatch.go`, `internal/tui/`, `tests/e2e/`, support matrices and user/limitations/restore documentation; future tasks remain one domain and expose safe preview/cancel/recovery outcomes.

## Open Questions

These are delegated engineering/evidence investigations for refinement, not founder decision requests; numeric bounds are already fixed in NFR-2/NFR-3.

- **OQ-1 — Complete roots and controlled-copy inventory.** Blocks destructive implementation: derive every retained record/artifact/copy and its owner from current storage plus merged context/operation contracts. Options: a complete supported scope with explicit roots; a smaller supported subset with omissions; refusal where reachability/copy classification remains unknown. Default: unknown or referenced data stays protected, never inferred collectible from age.
- **OQ-2 — Maintenance authority and durable installation protocol.** Blocks production rewrite/restore: freeze exclusive ownership/fencing, WAL/sidecar and directory durability semantics, crash phases and effect reconciliation per supported platform/filesystem. Options must preserve the old valid ledger through staging failure and identify a single authority after recovery; no lease-expiry takeover or normal-event delete shortcut.
- **OQ-3 — Versioned archive and sanitisation contract.** Blocks export/import implementation: choose a deterministic streaming format with exact manifests, missing-data representations, scope-bound IDs, redaction exclusions and hostile-member refusal. Default: local regular-file export only; no external upload, raw native store or implicit authority import. Sanitised portability and complete backup are distinct advertised coverage classes.
- **OQ-4 — Derived lineage and finalisation evidence.** Blocks complete erasure claims for enabled derived stores: map data→index/example/model/policy derivations, define invalidation/rebuild and prove selected rollback/backup/export copies are handled without losing recovery prematurely. Unsupported external/native/provider copies remain disclosed, not deleted by assertion.

## Definition of Done

- [ ] FR-1–FR-8/NFR-1–NFR-4 have a refined criterion/evidence map and independently reviewed domain-pure implementation tasks with stable acceptance IDs.
- [ ] Scoped dry-run and sanitised export/import demonstrate original identities, dependency completeness/omissions and denial before metadata leakage.
- [ ] Active, pinned, unresolved-owner and dependency-referenced provenance survives cleanup; disk pressure/bound exhaustion blocks effects safely.
- [ ] Quiescent rewrite/restore and each interrupted swap phase preserve one valid recoverable authority without native or external-effect replay.
- [ ] Derived stores and selected local copies have explicit purge/retention dispositions; content-free receipts state surviving backup/native/provider and physical-erasure limits truthfully.
- [ ] Advertised schema/archive/platform/filesystem combinations carry actual integrity/restore evidence; historical grants/acceptance are not silently promoted after import.
