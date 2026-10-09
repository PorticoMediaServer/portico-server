# Home artwork query investigation — 9 October 2026

Target hardware is the existing demo VPS: one CPU core and about 1 GB RAM. Justin's requirement is ordinary browsing and several direct-play viewers on this class of hardware. Transcoding is disabled through the supported owner-settings API and verified in the persisted playback policy (`transcoding_enabled=0`). The existing three synthetic live feeds run FFmpeg with stream copy, not conversion.

## Reproduced failure

Public HTTPS observations on the initial `2eb0e755` release:

| Request | Status | Wall time |
| --- | --- | --- |
| Home, default preview size | 503 timeout | 5.455 s |
| Home, six items per row | 200 | 4.275 s |
| Home, one item per row | 200 | 1.860 s |
| Libraries | 200 | 0.400 s |
| Items, limit 12 | 200 | 0.410 s |
| Three concurrent default Home requests | All 503 | About 5.495 s each |

These timings include the public network/Cloudflare path. Web, Android and Vega displayed the same Home failure. A live CPU profile attributed 78.43% cumulative sampled CPU to `catalog.resolveArtworkContext`, rather than recommendation ranking.

## Cause and correction

The artwork resolver pins requested entities and selected-artwork joins, but its ordinary join to `artwork_objects` allowed SQLite to place all ready objects before selected artwork. With the demo's cache (10,813 objects and 4,345 selections), the plan searched ready objects by status for each requested target, then probed selections by digest. Repeating that work across Home rows exceeded the existing five-second request deadline.

Commit `7b34e52d62150788efae7bba54990865cf25509d` pins the object join after selections with `CROSS JOIN`. The plan now seeks selections by their `(kind, entity_id, role, subject)` primary key, then seeks the selected object by digest. No schema, artwork cache, ranking, visibility policy or deadline changes are required. The query was extracted into a constant so the regression test exercises the actual production statement.

On a private copy of the demo database, using Go's actual SQLite driver and `GOMAXPROCS=1`, Home dropped from 1.60–1.66 s to 73.8–123.9 ms. Twelve-target artwork resolution dropped from 174 ms to 1.09 ms. These are local diagnostic timings, not VPS measurements.

The regression supplies representative populated-cache planner statistics, asserts indexed seeks, and verifies ready artwork selection and exclusion of deleting objects. It fails with the old join and passes with the correction. Existing artwork inheritance/category tests pass. HTTP Home, browse, search, detail, read-path and compression checks pass. The complete catalogue suite also passed in 449.717 seconds.

## Live validation

The fixed release was activated on the demo VPS after preserving the stopped state and previous configuration in `/var/backups/portico/demo-20261009T154608Z`. Readiness returned 200, the catalogue retained all 356 entities, and transcoding remained disabled.

Three independently signed-in client installations each requested Home simultaneously for three rounds. All nine requests returned 200 in 1.269–1.721 seconds over public HTTPS; the equivalent baseline had returned three 503 responses at approximately 5.495 seconds. These sessions share the demo owner's profile and are not a multi-account capacity test.

Regular Ubuntu Chrome displayed the populated Home page, with a fresh `/v1/home` request returning 200 in 481.4 ms. Earlier timeout entries remain in the browser's request history. Public wall times include Germany/network/proxy latency and must not be presented as server calculation time. Justin's soft target for ordinary Home and Discover calculation is 100–200 ms.

Raw CPU profiles, plans, build logs and timing JSON are kept under `apps/portico-server/.scratch/home-investigation-20261009/`; the database copy stays under the private credentials directory and is never committed.
