# Unattended campaign checkpoints

CI runs on the whole `main` tree hourly. The machine scheduler is configured for separate M0 checkpoints every
four hours without an agent driving the applications. They never gate a merge,
stop an owner's process, or change the public demo/Hosted deployment.

Every run freezes published `main` commits, builds on Netcup/Contabo, creates
fresh disposable state, and records each flow separately. Credentials, mail and
browser profiles exist only in private job scratch. Screenshots are withheld
while populated credential controls remain. Evidence contains source pins,
closed operation/error receipts and screenshots, never tokens or message bodies.

A second flaky result permanently drops that flow from automatic execution;
its receipt says that Justin's hands-on must cover it. Product failures do not
become passes, and one surface never holds another. A busy build/device lock
produces a resource-unavailable receipt and leaves the owner working.

The browser checkpoint is `browser.mjs` plus small modules under `browser/`:
`harness.mjs` owns the browser lifetime, per-flow records, redacted captures and
bounded receipts; `receipts.mjs` holds the closed operation/error/CSP allowlists;
`auth.mjs` the real signed-out and sign-in steps; `web.mjs` the web core flows;
`hosted.mjs` the ordinary Hosted account flows; `reading.mjs` publication
geometry. Reading passes only on the paginator's own section frame, rendered
through its whole ancestor chain, with visible text Range geometry on at least
two lines and at least 80 characters; images are recorded in the receipt but are
not a substitute for the unchanged fixture text. The Reading screen's shell
chrome is not a publication. The retired `web:search-real-title` flow is dropped,
not re-enabled. Receipts stay code-only: operation names (including the closed
staged Reading operations: create, progress, source, pages, annotations, search,
assets, file-save, close), status classes, CSP directive and blocked-class counts,
and geometry counts, never URLs, bodies, nonces or account identifiers.

Android automation is the next rollout stage, reusing the existing qualified
APK/Reader packaging and private native metadata checks. Apple runs only after the other automated flows are stable.
`Portico_CertTest_API36` is never a target. All child processes, emulators and
scratch started by a run are retired in `finally` paths.

The loopback Hosted fixture uses the application’s existing isolated development-origin profile with a fresh certified signing root, fresh password-protected PostgreSQL and a verified-TLS private mail gateway. It makes no production-origin/TLS/device-proof acceptance claim. Every message recipient is under `m0.getportico.invalid`; no real user is mailed. Official generators run against each captured tree before compilation; any derived changes are returned as a patch receipt for Release adoption, never silently credited as merged main.

The ledger and owner work lists refresh from all three published-main histories every two hours. The generator preserves Owner notes. A separate hourly machine audit writes CI failures to the Release inbox; app owners receive concrete findings without holding their merges.
