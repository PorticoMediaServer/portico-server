# Ancillary repositories and cross-cutting review

Source-only audit on 2026-09-25. No apps, builds, tests, downloads, package installation, service calls, or deployments were executed. Paths below are relative to the Portico workspace root. This supplementary report prevents the public website and recommendation build pipeline from being silently omitted from the broad codebase request. Review depth was lower than the four main product lanes.

All three ancillary entries were independently challenged in `07-adversarial-hosted-reviewer.md`; the mechanisms survived, with upstream-frequency, outer-error-handling and editable-install scope clarified below.

## Ancillary findings

### AX-001 — P2 — Enrichment lookup skips configured retries for non-JSON transient responses

**Confidence:** High. **Area:** Acquisition reliability.

**Evidence:** `portico-recommendation-enhancement/src/portico_enrichment/similar_lookup.py:698–730`.

The similar-title lookup obtains the status, then requires `response.json()` to succeed before checking whether the status is 429/502/503/504. An HTML or empty rate-limit/service-unavailable response immediately records `invalid JSON` and raises `WikidataError`; the retry classification, Retry-After handling, and backoff below are unreachable. The Wikipedia/Wikidata wrappers elsewhere check transient HTTP status before parsing, making this an inconsistent path rather than an intentional global policy.

**Trigger/impact:** If an upstream transient response has an empty/non-JSON body while retries are enabled, this branch skips the intended retry. The outer handler (`similar_lookup.py:500–504,630–638`) records `api_error` and pauses the lookup batch; this is not necessarily an uncaught process exception. Valid JSON transient/maxlag responses and network errors have other retry handling; retry count zero intentionally disables retries. Upstream response frequency was not observed, so “common” is not an empirical finding.

**Fix:** Classify HTTP transient statuses and honor retry headers first; parse JSON only where needed for successful responses or JSON API errors such as maxlag. Preserve the last failure and request metadata without mislabeling a transport outage as malformed JSON.

**Suggested verification:** Fake 429/503 responses with empty/HTML bodies and Retry-After, transient 502, valid JSON maxlag, and a final nonretryable malformed 200 response.

### AX-002 — P2 — Normal package installation omits the transport SQL schema

**Confidence:** High for packaging mismatch; distribution use is conditional. **Area:** Build/install portability.

**Evidence:** `portico-recommendation-enhancement/pyproject.toml:1–23`; `src/portico_enrichment/transport.py:14,36–44` in that repository.

The module discovers `schema/transport.sql` by walking from its source file to the repository root. The setuptools configuration discovers only packages under `src` and provides no package-data/data-files rule for the root-level SQL. A standard wheel installation does not retain this repository layout or ship the schema at that computed path. `initialize_transport()` then cannot read the schema. The documented workflow is editable checkout setup (`README.md:47–56`, `Makefile:8–9`), which can work. Transport initialization/build in an ordinary wheel-only installation is the affected condition; not every CLI command needs this resource, and no deployed wheel workflow was established.

**Fix:** Package the SQL inside the Python package and load it with an installed-resource API. Include other required runtime resources explicitly and separate working-directory defaults from package resources.

**Suggested verification:** Build/install the wheel into a clean temporary environment without the checkout, then initialize/validate an empty transport using only installed resources. Not executed here.

### AX-003 — P2 — Transport export reads candidates, decisions and provenance from separate snapshots

**Confidence:** High for consistency gap; requires concurrent ledger changes. **Area:** Dataset integrity.

**Evidence:** `portico-recommendation-enhancement/src/portico_enrichment/transport.py:146–156`; `storage.py:467–496,520–523`.

`build_transport()` invokes `fetch_candidates()`, `fetch_decisions()`, and `fetch_sources()` sequentially. Each opens its own SQLite connection and reads independently; there is no shared read transaction or immutable build revision. Enrichment/acquisition work can update the ledger between these reads, producing a published transport that combines old identity/provider facts with new decisions or source provenance. Final validation checks the artifact's internal schema/relationships, not whether its input belonged to one ledger snapshot.

**Trigger/impact:** Run an export while acquisition or decision ingestion modifies related rows. Rows can be omitted, provenance can refer to a different decision epoch, or a decision can be exported against older candidate facts. No claim that an already-published dataset suffered this race.

**Fix:** Export all required inputs, including per-candidate provider IDs, from one explicit read transaction on one connection, or from a transactionally captured immutable ledger snapshot. Merely sharing a connection/context is insufficient without a read transaction. Record the selected snapshot/revision in build metadata; avoid a build-side write lock lasting through expensive output work.

**Suggested verification:** Interleave an update exactly between identity, decision and source reads. Assert export sees either the complete before-state or complete after-state and that its provenance is tied to that state.

## Further ancillary concerns requiring nuance

- `Storage.fetch_candidates()` performs a provider-ID query per candidate and materializes every candidate; decisions and sources are also loaded wholesale. This makes export memory proportional to the dataset and introduces N+1 SQL work. This is an offline batch pipeline, so O(dataset) is not inherently wrong; establish expected population and memory budget before deciding whether streaming/batched joins are necessary.
- Most ledger methods use `with self.connect()` as a transaction context but do not explicitly close the SQLite connection. SQLite's connection context is not a close operation. Object finalization may reclaim it, but deterministic closure would make connection/resource lifetime clearer in long-lived acquisition processes. No exhaustion was demonstrated.
- Export's temporary-file cleanup surrounds validation/rename but not the earlier insertion/VACUUM phase. An exception before that later `try/finally` can leave a temporary transport file. The original output is protected by rename-after-validation, which is good; expand cleanup ownership for failed builds.
- Allowed sources and request destinations are mostly fixed and private packet writers use restrictive modes. Requests default to following redirects, and response bodies generally lack explicit byte caps. Redirect destination validation and capped bodies would strengthen the stated source boundary, but no redirect bypass or hostile upstream was established.
- The pipeline repository currently has an unborn `main` branch and untracked source. Per-file hashes in `source-inventory.json` identify the audited content; there is no commit-based reproduction point for it yet. No commit or publication was performed.

## Public website review

`getportico-tv` at `949f0d4a7a25a88f66af7b6247013a4ff00e552e` is configured as a static Astro site. Reviewed layout/route generation, documentation/search scripts, download destination data, build identity/check scripts, package/configuration, and CI/security workflow source. The three small browser scripts manipulate attributes/text-derived search matches rather than injecting user HTML. Download availability is represented explicitly; unavailable native installers are disabled “Coming soon” buttons, so those were not reported as fake working downloads. CI actions are pinned and the downloaded secret-scanner archive has a checksum check.

No substantive independent website code defect was established in those paths. This is not proof of visual/accessibility correctness, active hosting headers, package advisory status, or external-link availability. No page was launched and no external URL was probed. The default LAN HTTP setup instructions must be reconciled with **WC-012** in the main product review.

## Shared-code and integration observations

- React Native's shared-package copy was compared statically by the native reviewer: client-core 186/186 files, design 6/6, i18n 15/15 matched their server-repository counterparts by SHA-256. This is copy consistency, not functional validation. Shared defects are assigned once in the web/core report unless their native manifestation needs a separate fix.
- The workspace root is no longer a Git repository. Server/web, React Native, Hosted/internal, and website have distinct repositories. Old `services/*` and `apps/portico-web` paths in historical material are not the current source locations. All reports use the current split layout.
- Release/recovery defects in HS/SV reports and the Hosted Cast CSP mismatch span repositories. Fix them with contract/integration checks after unit fixes; success of an isolated package's tests would not demonstrate the full deployment path.
- The audit directory is inside internal architecture documentation. At final status inspection the audit files are untracked local review artifacts. Nothing was committed, pushed, published, or deployed by this audit; an external documentation-only commit during review is recorded in the index.

## Security evidence boundary

The audit reviewed security-sensitive source paths but is not a penetration test, dependency-advisory scan, historical secret scan, fuzzing run, cryptographic audit, or production configuration attestation. Checked-in Caddy/systemd/container policies are evidence of intended configuration, not proof of the live configuration. Installed third-party binaries and full dependency implementations were not audited line by line. No actual secret values are reproduced in these reports.

The useful next security review is to challenge each claimed boundary: identify the attacker/role, exact reachable input, minimum preconditions, existing server/native defenses, and what evidence would falsify the finding. Each component report includes those qualifications and targeted verification suggestions.
