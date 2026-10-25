# Unattended campaign checkpoints

CI runs on the whole `main` tree hourly. These separate M0 checkpoints run every
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

Android uses the existing qualified APK/Reader packaging and private native
metadata checks. Apple runs only after the other automated flows are stable.
`Portico_CertTest_API36` is never a target. All child processes, emulators and
scratch started by a run are retired in `finally` paths.

The loopback Hosted fixture uses the application’s existing isolated development-origin profile with a fresh certified signing root, fresh password-protected PostgreSQL and a verified-TLS private mail gateway. It makes no production-origin/TLS/device-proof acceptance claim. Every message recipient is under `m0.getportico.invalid`; no real user is mailed. Official generators run against each captured tree before compilation; any derived changes are returned as a patch receipt for Release adoption, never silently credited as merged main.
