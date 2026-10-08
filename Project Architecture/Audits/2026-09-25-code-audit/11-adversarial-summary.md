# Adversarial second-pass results

Both review passes are complete. The original component reports and review ledger incorporate the second-pass conclusions; use those as the current assessment. The individual adversarial reports preserve the reasoning and counterevidence behind each verdict.

## What the second pass checked

Reviewers were reassigned across areas: the Hosted Services reviewer challenged the server and ancillary findings; the server reviewer challenged Apple/React Native; the native reviewer challenged Web/shared-client code; the primary reviewer challenged Hosted Services. The primary also reconciled cross-component assumptions and selected high-impact source chains.

Every one of the 57 original prioritized entries received a verdict. Reviewers checked callers and consumers, guards and cleanup, relevant tests as source, feature configuration, exploit/delivery prerequisites, alternative explanations, severity, and whether the suggested repair would violate an existing invariant. They did not execute the applications, tests, builds, benchmarks or exploits. This was a second adversarial review of the findings and their surrounding paths, not a second independent every-line audit of the entire repository.

| Verdict | Entries | Meaning |
|---|---:|---|
| Upheld | 22 | Source mechanism and material framing survived the challenge |
| Narrowed | 31 | Mechanism survives, but trigger, consequence or scope needed qualification |
| Severity lowered | 3 | Mechanism survives at a lower practical priority |
| Moved to conditional | 1 | Source concern exists, but the reported end outcome remains unresolved |

The final ledger contains **54 source-supported findings and 3 conditional assessments**. Across those 57 entries, priorities are **8 P1, 48 P2 and 1 P3**. Additional lower-confidence concerns remain outside that count. These are review priorities, not a runtime incident count or an approved remediation backlog.

## Material corrections

| Entry | Final treatment | Why it changed or needed precision |
|---|---|---|
| NC-001, Top Shelf ownership | P2, lowered from P1 | The ownership/cleanup defect is established. Visible prior-viewer content requires usable cached posters; the separate artwork-capability defect NC-019 masks a clean-cache scenario. Credential revocation also differs between local and hosted authority. |
| SV-012, obsolete Cast protocol | P2, lowered from P1 | The configured receiver still uses removed endpoints, but Cast is optional and unconfigured by default. No basis was established for blocking every release on this feature. |
| WC-007, skipped service disposal | P3, lowered from P2 | Obsolete work survives cleanup, but subscriptions are removed and examined operations have finite deadlines. No permanent leak or stale-viewer publication was proved. |
| NC-024, Spotlight owner race | Conditional P2 | A delayed callback lacks an owner fence, but native exports are already serialized on the main queue and final CoreSpotlight ordering/visibility is unproved. The report no longer asserts a demonstrated post-clear privacy leak. |
| WC-001, cross-viewer browse cache | P1 retained; trigger narrowed | The strongest path seeds the cache during an in-place filter/sort transition, then changes account/profile within the same document. Full unmount can dispose pages before saving; full reload clears the cache. The exposure is metadata, not authorization to protected media bytes. |
| NC-003, cached receipt verification | P2 retained; attack claim narrowed | The decoder performs structural, identity, key and claim-agreement checks. The missing cryptographic-verdict binding is a source defect; simply editing an expiry is rejected, and no forged capability or external injection path was demonstrated. |
| HS-002, mutable DNS probe | P1 retained; consequence narrowed | Opening the table performs mutations. Actual loss depends on overlap with the daemon's append/compaction, and later feed activity may repair state. An isolated probe does not imply corruption. |
| SV-002, restore continuation | P1 retained; repair narrowed | Some errors follow a safe rollback; others leave unresolved state. Startup needs a trustworthy recovery disposition. Treating every error as fatal or blindly rolling back database migrations would be an unsafe blanket fix. |
| WC-004/008/011 and NC-010/011, response bounds | Priorities retained; limits clarified | Timeouts, watchdogs, concurrency controls and normal cooperative servers mitigate exposure, but do not provide the missing byte bounds. No infinite lifetime, measured memory crash or universal unauthenticated delivery was established. |
| NC-012/014/021/023, lifecycle and scaling | P2 retained; lifetime narrowed | The defects have specific boundaries: an existing mounted player, visited pages within a search generation, a revoked/expired captured credential, or early controller creation before capability loading. They are not permanent installation-wide failures. |
| AX-001/002/003, enrichment tooling | P2 retained; deployment and concurrency narrowed | Non-JSON errors pause a batch through the outer handler; the documented editable installation works despite the wheel-layout defect; inconsistent export reads require concurrent mutation. Atomic output publication does not make the source reads one snapshot. |

HS-008 remains a conditional deployment-recovery assessment, not an assertion that lack of automatic rollback is inherently a security defect. HS-009 remains conditional on configured Cast and the checked-in CSP being deployed. Fixing that policy alone does not resolve the independent server Cast protocol/authority issues.

## Suggested starting point for Claude

Start with the index and ledger, then challenge the eight P1 entries against recent project decisions and intervening changes. The priority queue is HS-001/002, SV-001/002/003/004 and WC-001/012. Group restore, release and Cast findings by their shared boundaries rather than applying isolated fixes that conflict with existing authority or recovery guarantees.

Recent context may explain intended restrictions, an unused packaging path, an external recovery procedure or a newer implementation. Record that evidence against the stable finding ID and distinguish a refuted source claim from a valid defect with lower deployment relevance. All proposed regressions remain unexecuted suggestions. No repair is authorized or implemented by this audit.

## Reports and evidence

- [Main index, scope and priority queue](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/00-overview.md>)
- [All 57 entries with final priority and verdict](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/06-review-ledger.md>)
- [Server and ancillary adversarial review](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/07-adversarial-hosted-reviewer.md>)
- [Apple / React Native adversarial review](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/08-adversarial-server-reviewer.md>)
- [Web / shared-client adversarial review](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/09-adversarial-native-reviewer.md>)
- [Hosted Services adversarial review](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/10-adversarial-primary-reviewer.md>)
- [Final source-integrity and report checks](</Users/justin/Documents/Codex Projects/Portico/portico-internal/Project Architecture/Audits/2026-09-25-code-audit/verification.json>)

Coverage matrices in the component reports distinguish deep review, sampling and inventory-only areas. Performance conclusions describe source-level growth or missing limits; they do not establish production latency, throughput or device capacity. Security review was limited to source and configuration: deployed policies, current dependency advisories, route-complete authorization proof, third-party decoder internals and exploit testing remain outside the evidence.

All 3,752 inventoried source/config files matched their initial hashes at final verification. The internal documentation repository advanced externally during the audit; its change was confined to a Deployment Information README and architecture backlog, and is recorded separately from the source check. Audit documents remain local and untracked. No application code was changed, committed or published.
