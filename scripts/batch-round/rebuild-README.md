# Fresh Contabo test server

Run this only when the setup/reset is explicitly cleared. It is not CI. Default `rebuild-test-server.py` is a no-network plan; live stages reject CI. Host reset admits only runner@portico-runner, fixed appdata/media/unit paths, explicit literal confirmation and an existing generated-clean build matching a full green-main commit. It generates media in staging before stopping the old server, retires old active state/media to private rollback paths, creates fresh identity/appdata and starts only the three named test services. It never touches the demo, Hosted database or unrelated service.

The fixture generator/state helper is preserved on `fixtures/test-library-v3`, pin `8f43a827b`, not main. Export that reviewed source plus its procedural input fixtures into a dedicated `FIXTURE_SOURCE`, preserving directory structure; record `.fixture-revision` containing `8f43a827b`. Do not assume main has the generator. Verify the export against `git archive 8f43a827b fixtures/test-library fixtures/video/hdr-generated fixtures/video/dovi-probe-r40 fixtures/discs/dvd-two-title-video-ts`. Do not regenerate over an actively scanning library: hardlink ctime is part of its revision.

Build with current private `deployment/testing/walks/server.py` tooling from internal main containing e2fb4e4 (the old archived toolkit uses the removed portico-web-next workspace). Reuse the qualified FFmpeg/Sonic runtime; record commit, build manifest and GitHub completed-green run before deployment. Do not deploy a pending head as green. Build on Contabo, never mini-1. The host script requires the qualified bundle, existing fixture Python venv and existing managed test unit; it does not install system dependencies.

```sh
# On Contabo, these variables are fixed verified operator paths, not credentials:
python3 scripts/batch-round/rebuild-test-server.py --stage host --fixture-source "$FIXTURE_SOURCE"
# After authorization:
python3 scripts/batch-round/rebuild-test-server.py --stage host --fixture-source "$FIXTURE_SOURCE" --build "$GREEN_BUILD" --commit "$GREEN_SHA" --confirm RESET-CONTABO-TEST-SERVER --execute
# On the operator host, over the existing private loopback49300 tunnel:
python3 scripts/batch-round/rebuild-test-server.py --stage provision --fixture-source "$FIXTURE_SOURCE" --manifest "$MANIFEST" --credentials "$CREDENTIALS" --report "$EVIDENCE/seed.json" --execute
python3 scripts/batch-round/claim-test-server.py --fixture-source "$FIXTURE_SOURCE" --credentials "$CREDENTIALS" --hosted-session "$HOSTED_SESSION" --report "$EVIDENCE/claim.json" --execute
```

Before provisioning a genuinely fresh DB, atomically preserve the private walk credentials, remove only the old `fixtureStateV3` cache and reuse account passwords. Never reuse old session or claim grants. The pinned seeder is still subject to real API drift; any stale-continuation or missing state is a setup failure to repair, not an excuse to pause scans or count a partial run ready. Media manifest entry counts include versions and container records: reconcile grouped item counts rather than equating media files with titles.

Hosted registration/login uses the designated existing private credential file, normal `/v1/registrations`/`/v1/sessions` and the private continuation. Complete the ordinary email/code proof if the reset removed the account. The claim tool takes an owned 0600 authenticated Hosted-session JSON containing captured `base`, `session.accessToken`, `session.account.id` and `verifiedBy`; it does not create accounts or bypass verification. Its default Hosted API transport is the operator's private `127.0.0.1:49410` SSH forward to the documented Hosted loopback19410; capture the real HTTPS origin in `base`. It approves the server-produced exact claim, continues the durable same operation, and requires final installed+acknowledged readback. It never replaces another account's existing claim. No token, proof, password or verification code is printed.

`TEST-SERVER-READY.md` in the private campaign folder is the handover authority. Fresh startup alone is not readiness: complete accounts, profiles, counts, saved/reading/playback state, local and Hosted sign-in/claim, health, access and actual source/CI pin. Retain private rollback until successful handover; remove only that reset's retired disposable test data afterwards. Keep running service, media, credentials, required runtime and minimal receipts.
