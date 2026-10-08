# Product contract governance

Contract ID: PC-GOVERNANCE. Revision: portico.reconciled.1.

Product decisions are separate from implementation design and verification. Justin authorized this reconciliation to preserve original UX/product choices and use architectural judgment on implementation. That authorization covers these documented corrections; it does not certify a runtime or approve unrelated product changes.

The [registry](registry.json) identifies the current corpus. Approved/reconciled means intended behavior; implemented means exact code and required platform evidence demonstrate it. No entry is marked implemented by this documentation pass. Dated feature evidence remains useful only within its actual tested scope.

Changes record the problem, original/current rule, affected domains, existing implementation impact, and required conformance outcomes in decisions.md and the corresponding family. Product changes require Justin's decision unless already authorized. Routine implementation choices, safe internal representation changes and mechanisms that preserve UX are delegated. Do not turn a historical exception template into repeated permission requests for already-authorized work.

A platform adaptation may change geometry, native APIs and transport, but cannot silently change account authority, media identity, library hierarchy, state ownership, quality intent, destructive semantics, visibility or product scope. An advertised capability requires an end-to-end working implementation. Hiding an unimplemented action is honest staging, not feature completion or approved removal from scope.

Document any exceptional product divergence using one form: Contract; Scope; Decision; Reason; Approved-by; Review-when; Evidence. Original visual/foundation template variations are historical. Do not import empty old exception registries as evidence of conformance.

Domain authors and independent reviewers examine cancellation, lost/duplicate responses, changed authority, concurrent devices, stale generations, partial I/O, crash/restore, scale and real UI before claiming completion. Code review precedes behavioral tests under documents 33–36. Schema validation alone does not prove semantics, and simulator evidence does not prove physical media/output behavior. The current native restriction remains simulator-only; hardware gaps remain explicit release evidence needs.

Validation of this documentation corpus checks registry and CAP coverage, all local document links, source provenance, retained product constants, cross-domain precedence, and the consistency of the added failure scenarios. Old JSON/YAML artifacts are parsed as references; their old schema vectors are not misrepresented as tests of the rebuilt implementation. Each implemented API must own a current schema and port the retained outcome vectors. Do not execute old contract compiler scripts into the new build.

Planning, contracts, evidence, prototypes, test fixtures and private provenance remain unpublished under existing repository policy. This reconciliation does not change deployment authority or the frozen-original-workspace boundary.
