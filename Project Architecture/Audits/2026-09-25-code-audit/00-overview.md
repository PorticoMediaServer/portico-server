# Portico code audit — review index

Audit date: September 25, 2026 (America/Halifax). Prepared for independent Astra / Claude review. **Static source review only; no remediation performed.**

## Assessment

**Status: both passes complete.** Every reported finding received an adversarial second review; the original reports now incorporate its corrections. Start with the [second-pass results](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/11-adversarial-summary.md>) for what changed.

This pass found substantive security, availability, recovery, build, client correctness and performance defects across the requested systems. The first review priorities are the optional native OAuth completion proof, the leaked server database writer gate, unsafe restore recovery, mutable DNS health checks, and viewer data surviving identity changes. Two independent packaging paths also omit required source inputs. These are specific source-supported failure mechanisms, not a general assertion that Portico is insecure or that these failures have occurred in production.

The code also contains substantial safeguards: current-authority checks, durable operation receipts, bounded workers, constrained outbound networking, explicit media custody, secure credential storage and windowed collections. Several findings concern interactions that bypass or outlive those safeguards. The detailed reports include counterevidence so a reviewer can challenge each conclusion without reconstructing the whole audit.

## Read the audit

| Report | Scope | Prioritized entries |
|---|---|---:|
| [Hosted Services](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/01-hosted-services.md>) | Hosted account/auth services, PostgreSQL, DNS, API toolkit, deployment and recovery scripts | 9: 7 core defects + 2 deployment/feature-conditional entries |
| [Server](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/02-server.md>) | Go media server, storage, recovery, identity, playback, downloads, workers, release and container tooling | 12 |
| [Web and shared client-core](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/03-web-and-client-core.md>) | Web lifecycle, identity caches, playback/audio, transport, collections, downloads, guide | 12 |
| [Apple / React Native](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/04-apple-react-native.md>) | Native modules, React integration, offline media, Top Shelf, navigation, rendering and accessibility | 21 reviewed entries: 20 findings + 1 conditional assessment |
| [Ancillary and cross-cutting](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/05-ancillary-and-cross-cutting.md>) | Recommendation enrichment pipeline, public website, shared-source equivalence, evidence boundaries | 3 |
| [Review ledger](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/06-review-ledger.md>) | Compact list of every prioritized entry with triage conditions | 57 total |
| [Source inventory](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/source-inventory.json>) | Baselines, source/config hashes and original repository status | Inventory, not a reviewed-file attestation |

There are **57 reviewed entries: 8 P1, 48 P2 and 1 P3**. Of these, **54 are source-supported findings and 3 are explicitly conditional assessments**: HS-008 (deployment recovery), HS-009 (configured Cast/CSP), and NC-024 (Spotlight ordering). Several other entries establish a missing bound or unsafe interleaving whose impact requires the stated input or timing. Additional concerns are kept outside the count, including NC-016–018. NC-001 and SV-012 were downgraded to P2; WC-007 was downgraded to P3; NC-024 moved out of established findings. “High confidence” describes the source mechanism; it does not mean runtime reproduction or measured production frequency. No P0 was established.

## Highest-priority review queue

| ID | Why review first | Important condition / limit |
|---|---|---|
| HS-001 | An initiator can omit native OAuth callback-possession proof and receive a victim's linked-account session | Victim must complete provider authentication through the initiator's URL; no MFA bypass claimed |
| SV-001 | Subtitle/lyrics publication finishes the old transaction wrapper and leaks the process-wide writer gate | Ordinary authorized feature use can trigger it; persists until process restart |
| SV-002 | Restore journal overwrite and unconditional startup continuation can serve missing/mixed state | Interruption or failed recovery I/O; original backup may remain recoverable |
| HS-002 | DNS healthcheck opens and repairs storage concurrently owned by the daemon | Loss depends on overlap with append/compaction; the probe itself has confirmed mutation side effects |
| WC-001 | Global browse windows disclose previous-viewer metadata before revalidation | Same running browser tab across account/profile transitions; not proof of media authorization bypass |
| SV-003 | Isolated server release snapshot omits required embedded resources | Supplied release builder after prerequisites pass; no build was executed |
| SV-004 | Docker web build omits the local i18n source package | Supplied clean-context Docker build; no image was built |
| WC-012 | Advertised LAN HTTP web entrypoint depends on secure-context browser APIs | Private-IP HTTP; HTTPS and localhost have different behavior |

These are proposed review priorities, not approved changes. Review source assumptions before assigning fix scope. Recovery findings SV-002/005/006 belong together; release findings SV-003/004/010 belong together; Cast spans HS-009, SV-007/008/009/012 and the client integration. Shared client-core defects apply to both identical package copies and should be fixed at the canonical source before synchronizing.

## Performance assessment

The strongest static performance findings have concrete growth mechanisms: decoded-audio seek walks the file prefix (WC-003); guide windows refetch whole lineups (WC-010); recommendation reads replay the full pending profile backlog (SV-011); native Search/Saved retain and mount all visited pages (NC-014). Resource limits are also incomplete around browser/native response accumulation and MP4 table expansion (WC-004/008/009/011, NC-008/010/011).

Other scale concerns remain hypotheses, such as active HLS capacity, large original-download fairness, decoded PCM budgets, Hosted export size and worker backlog capacity. No latency, CPU, memory, throughput, frame-rate, database plan or device capacity measurements were taken. The reports therefore identify where cost grows and what to measure; they cannot conclude that current production performance meets a target.

## Security assessment

The review covered authentication and continuation possession, current account/profile authority, cache and extension privacy, credential/signature custody, untrusted parsing, network destination restrictions, response budgets, pairing abuse controls, storage recovery and release provenance. Priority security review includes HS-001, WC-001, NC-001/003/004/005 and SV-007; NC-024 remains a conditional privacy assessment; denial-of-service and integrity implications appear in several other findings.

A conditional malformed-response finding proves a client-side missing defense, not that an arbitrary unauthenticated internet attacker can deliver the input. No broad account IDOR, forged provider token, TLS bypass or remote code execution was established. This is also not a security clearance: route-complete authorization proof, dependency/advisory review, third-party decoder internals, deployed policy and exploit testing were not completed.

## Baseline and scope

The workspace is a collection of split repositories, not one Git repository. Current source paths were verified rather than relying on historical monorepo locations.

| Repository | Baseline | Initial state |
|---|---|---|
| `apps/portico-server` | `820dde1342aaf33dbddc69a03c859e225191de7f` | Clean |
| `apps/portico-react-native` | `62463dda2ac7e163249857e9f3f0efc288a137d9` | Clean |
| `portico-internal` | `33ce9641e825273b19f995a0ee5d89a8c64328b3` | Existing untracked `Deployment Information/`; its private contents excluded |
| `getportico-tv` | `949f0d4a7a25a88f66af7b6247013a4ff00e552e` | Clean |
| `portico-recommendation-enhancement` | Unborn repository | Existing source untracked; hashes anchor this review |

The source/config inventory contains 3,752 entries. It includes tests, fixtures, lockfiles, shared copies and selected vendored source; its aggregate size must not be described as unique reviewed application code. Dependency installations, build outputs, private/operator directories, generated enrichment data caches and audit output are excluded. It is a captured source inventory rather than an atomic filesystem snapshot.

Three requested **GPT-6 Astra High** subagents reviewed Hosted Services, the Go server, and Apple/React Native. The primary agent reviewed Web/shared-client code and ancillary repositories, traced integration boundaries, and independently checked selected high-impact chains: OAuth opt-out, DNS mutation, writer-gate ownership, restore startup handling, missing build inputs, Cast issuance/rate accounting, native link parsing, Top Shelf ownership/artwork capabilities, native Home paging and playback capability ordering. The second pass reassigned Hosted reviewer → server/ancillary findings, server reviewer → native findings, native reviewer → Web/shared findings, and primary → Hosted findings. This is a consolidated audit plus adversarial review of every finding, not two independent exhaustive audits of every source file.

The component reports provide package/surface coverage and distinguish deep call-path review, representative sampling and inventory-only areas. **This was a broad, detailed, risk-directed codebase audit, not a literal every-line verification of all source, tests, generated code and vendor code.** Large server packages still contain paths needing future review. No visual/app behavior was exercised, so presentation, actual accessibility announcements and hardware/media interoperability remain acceptance gaps.

## Review and remediation workflow

The ledger records the second-pass verdict for each entry. For your/Claude review, add accept / narrow / reject / investigate, supporting counterevidence, final priority and owner. Keep independent review notes distinct from the original observation. Before repairing, verify that the baseline still matches and check whether intervening work already addressed the path. Use each finding's proposed regression as a starting point; do not treat an unexecuted test suggestion as proof.

Suggested work order: identity/privacy and global availability; recovery integrity; build/provenance; download/offline and Cast protocol correctness; bounded transport/rendering/query cost; remaining lifecycle/accessibility issues. Scope each repair to the actual invariant and retain existing authority, durability and compatibility safeguards.

## Actions and proof limits

Only audit artifacts were written. No application code was edited, committed or published. No app, browser, simulator, service, test suite, build, benchmark, migration, restore, production API request, network probe or exploit ran. Existing tests were read as evidence of intended coverage only. No secret values were copied into the reports. Public documentation/advisory databases were not queried, so no claim about current dependency vulnerabilities or platform certification is made.

Source-integrity verification is recorded in [verification.json](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/verification.json>); its scope is the inventoried files plus repository revision/status checks. All 3,752 inventoried files matched their captured hashes at final verification. During the audit, the internal repository advanced externally to `8f412e4d5993ff4c2c71491ba35a1204a3b83500` (a Deployment Information README and architecture backlog change); no inventoried application source changed. Audit artifacts are currently untracked local internal documents, not committed or published. These checks do not detect every change to excluded/uninventoried data.
