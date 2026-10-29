#!/usr/bin/env python3
"""Contract tests for the fixture API calls, without a server or network.

The fixtures talk to the real v1 API, so their request shapes are part of this
directory's contract. These tests drive the production helpers
(``seed_state.saved_resources``, ``seed_state.Client``,
``seed_state.Observer``, ``playback_smoke``) through a fake HTTP opener and
assert only what the current server accepts:

* ``GET /v1/saved-resources`` reads its continuation from the top-level
  ``nextCursor`` field, accepts no query key outside cursor/limit/kind/pinned/
  shape, and refuses a limit outside 1..100 with 400 ``invalid_cursor``
  (server ``httpapi.savedPageQuery`` and ``catalog.SavedResources``).
* A saved-resources cursor is signed over kind, shape, pinned and limit, so a
  page must repeat the same filters and limit.
* Playback start/stop declare ``Portico-Playback: 2``; the server answers 400
  ``client_update_required`` for a client that does not (server
  ``playbackv1.PlaybackProtocol`` and ``requireObservationProtocol``).
* Playback reporting is ``POST .../observer`` for the epoch, then
  ``PUT .../observation`` with epoch/seq/generation/positionMs/monotonicMs/
  playClockMs/seekSerial/rate (server ``playbackv1.ClaimObserver`` and
  ``Observe``). There is no playback ``/timeline`` route.
* The session is rotated with ``POST /v1/auth/refresh``: no access bearer, the
  envelope answered directly, both credentials moved, and one request ID for
  the single replay a lost answer is allowed (server
  ``httpapi.identityAuthRoutes`` and ``identity.RefreshSession``). A refused
  refresh ends the run; the fixture never signs in again behind the caller.
* A music library's saved page reads back with the loudness step the server
  runs regardless, because every music scan outside file-list-only measures it
  (server ``catalog.AppliesLoudness``). That one step is derived from the
  published matrix; any other difference still fails. Online enrichment stays
  off: one local provider, no AcoustID, file lyrics, segment detection off.

Run with
``python3 -m unittest discover -s fixtures/test-library-v3 -p test_api_contract.py``.
Set ``PORTICO_OPENAPI`` to a current ``server/api/portico.openapi.json`` to
also check the bodies against the registered schema.
"""
from __future__ import annotations

import ast
import json
import os
import sys
import unittest
import urllib.error
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import playback_smoke  # noqa: E402
import seed_state  # noqa: E402

EPOCH = "A" * 22
LEASE = "2026-10-29T12:00:00Z"


class FakeResponse:
    """The subset of an HTTP response the fixture clients read."""

    def __init__(self, document=None, status=200):
        self.status = status
        self.headers = {"Content-Type": "application/json"}
        self._payload = b"" if document is None else json.dumps(document).encode()

    def read(self, limit=-1):
        return self._payload if limit is None or limit < 0 else self._payload[:limit]

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


class FakeOpener:
    """Queues responses and records the exact requests the helpers sent.

    A queued exception is raised instead, which is how a lost answer (a
    transport failure after the server committed) is simulated.
    """

    def __init__(self, responses):
        self.responses = list(responses)
        self.requests = []

    def open(self, request, timeout=None):
        self.requests.append(request)
        if not self.responses:
            raise AssertionError("unexpected request: " + request.full_url)
        response = self.responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return response


def fixture_client(responses):
    client = seed_state.Client("http://fixture.invalid",
                               installation="fixture-installation", token="fixture-token")
    client.opener = FakeOpener(responses)
    return client


class FakeAdmin:
    """The reads and the write a library settings page makes, without a server."""

    def __init__(self, reads):
        self.reads = list(reads)
        self.writes = []

    def read(self, path):
        del path
        return self.reads.pop(0)

    def call(self, method, path, body=None, **_):
        self.writes.append((method, path, body))
        return {}


def sent_query(request):
    return urllib.parse.parse_qs(urllib.parse.urlsplit(request.full_url).query)


def sent_path(request):
    return urllib.parse.urlsplit(request.full_url).path


def sent_header(request, name):
    for key, value in request.header_items():
        if key.lower() == name.lower():
            return value
    return None


def sent_body(request):
    return json.loads(request.data) if request.data else None


class FakeClock:
    """A deterministic device clock; the tests never sleep."""

    def __init__(self):
        self.value = 0.0

    def __call__(self):
        return self.value

    def advance(self, seconds):
        self.value += seconds


def grant(epoch=EPOCH):
    return FakeResponse({"epoch": epoch, "everyMs": 1000, "leaseUntil": LEASE})


def ack(seq):
    return FakeResponse({"acceptedSeq": seq, "leaseUntil": LEASE})


# The session envelope the server issues (internal/identity/service.go
# Envelope). A sign-in nests it under "session"; POST /v1/auth/refresh answers
# it directly (internal/httpapi/identity_routes.go:171).
INSTALLATION = "fixture-installation"
DEVICE = "device-1"
FAMILY = "family-1"
PROFILE = "profile-1"
EXPIRES = "2026-10-29T12:15:00Z"
ROTATED_EXPIRES = "2026-10-29T12:30:00Z"


def session(access, refresh, *, expires=EXPIRES, family=FAMILY, device=DEVICE,
            profile=PROFILE, installation=INSTALLATION, generation="1",
            horizon="2026-11-29T00:00:00Z"):
    return {"accessToken": access, "refreshToken": refresh, "deviceId": device,
            "installationId": installation, "expiresAt": expires,
            "viewer": {"accountId": "account-1", "profileId": profile,
                       "serverId": "server-1", "authority": "local", "role": "owner"},
            "sessionFamilyId": family, "tokenGeneration": generation,
            "authorizationHorizon": horizon}


def signed_in(profile=PROFILE):
    return FakeResponse({"session": session("access-1", "refresh-1", profile=profile),
                         "profiles": [{"id": profile, "primary": True}]})


def rotation(access="access-2", refresh="refresh-2", **overrides):
    fields = {"expires": ROTATED_EXPIRES, "generation": "2"}
    fields.update(overrides)
    return FakeResponse(session(access, refresh, **fields))


def denial(code="unauthorized", status=401):
    return FakeResponse({"error": {"code": code, "message": "refused"}}, status)


def expiry_clock(seconds_before_expiry, expires=EXPIRES):
    """A clock placed just inside the refresh lead time of an envelope."""
    clock = FakeClock()
    clock.value = seed_state._instant(expires) - seconds_before_expiry
    return clock


def session_client(responses, clock=None):
    client = seed_state.Client("http://fixture.invalid", installation=INSTALLATION)
    client.opener = FakeOpener(responses)
    client.clock = clock or FakeClock()
    client.pause = lambda seconds: None
    return client


class SavedResourcesContractTests(unittest.TestCase):
    def test_pages_with_the_accepted_limit_and_the_top_level_cursor(self):
        client = fixture_client([
            FakeResponse({"resources": [{"id": "first"}], "nextCursor": "opaque+/="}),
            FakeResponse({"resources": [{"id": "second"}], "nextCursor": ""}),
        ])

        resources = seed_state.saved_resources(client, "collection")

        self.assertEqual([resource["id"] for resource in resources], ["first", "second"])
        self.assertEqual(len(client.opener.requests), 2)
        first, second = client.opener.requests
        self.assertEqual(urllib.parse.urlsplit(first.full_url).path, "/v1/saved-resources")
        self.assertEqual(sent_query(first), {"kind": ["collection"], "limit": ["100"]})
        self.assertEqual(sent_query(second), {"kind": ["collection"], "limit": ["100"],
                                              "cursor": ["opaque+/="]})
        for request in client.opener.requests:
            self.assertLessEqual(int(sent_query(request)["limit"][0]), 100)

    def test_an_exhausted_first_page_sends_one_request(self):
        client = fixture_client([FakeResponse({"resources": [{"id": "only"}],
                                               "nextCursor": ""})])

        resources = seed_state.saved_resources(client, "view")

        self.assertEqual([resource["id"] for resource in resources], ["only"])
        self.assertEqual(len(client.opener.requests), 1)

    def test_a_repeated_cursor_ends_paging(self):
        client = fixture_client([
            FakeResponse({"resources": [{"id": "first"}], "nextCursor": "same"}),
            FakeResponse({"resources": [{"id": "second"}], "nextCursor": "same"}),
        ])

        resources = seed_state.saved_resources(client, "view")

        self.assertEqual([resource["id"] for resource in resources], ["first", "second"])
        self.assertEqual(len(client.opener.requests), 2)

    def test_maximum_bounds_the_collected_resources(self):
        client = fixture_client([FakeResponse({"resources": [{"id": "first"},
                                                              {"id": "second"}],
                                               "nextCursor": "more"})])

        resources = seed_state.saved_resources(client, "collection", maximum=1)

        self.assertEqual([resource["id"] for resource in resources], ["first"])
        self.assertEqual(len(client.opener.requests), 1)

    def test_a_repeated_cursor_is_reported_not_silent(self):
        client = fixture_client([
            FakeResponse({"resources": [{"id": "first"}], "nextCursor": "same"}),
            FakeResponse({"resources": [{"id": "second"}], "nextCursor": "same"}),
        ])
        report = {}

        seed_state.saved_resources(client, "view", report=report)

        self.assertIn({"step": "saved-resources", "kind": "view", "code": "cursor_repeated",
                       "collected": 2}, report["warnings"])

    def test_a_truncated_window_is_reported_not_silent(self):
        client = fixture_client([FakeResponse({"resources": [{"id": "first"},
                                                              {"id": "second"}],
                                               "nextCursor": "more"})])
        report = {}

        resources = seed_state.saved_resources(client, "collection", maximum=1, report=report)

        self.assertEqual([resource["id"] for resource in resources], ["first"])
        self.assertIn({"step": "saved-resources", "kind": "collection",
                       "code": "cursor_truncated", "maximum": 1, "collected": 2},
                      report["warnings"])


class PlaybackProtocolHeaderTests(unittest.TestCase):
    def test_request_declares_the_observation_protocol(self):
        client = fixture_client([FakeResponse({"presentation": {"url": "/media/1"}},
                                               status=201)])

        page = playback_smoke.request(
            client, "POST", "/v1/playback/sessions", {"itemId": "item-1"},
            {"Idempotency-Key": "start-key-1"}, ok=(201,))

        self.assertEqual(page, {"presentation": {"url": "/media/1"}})
        request = client.opener.requests[0]
        self.assertEqual(sent_header(request, "Portico-Playback"), "2")
        self.assertEqual(sent_header(request, "Idempotency-Key"), "start-key-1")

    def test_start_helper_returns_once_the_presentation_exists(self):
        client = fixture_client([FakeResponse({"presentation": {"url": "/media/1"}},
                                               status=201)])

        session = playback_smoke.start(client, "item-1")

        self.assertEqual(session["presentation"]["url"], "/media/1")
        self.assertEqual(sent_header(client.opener.requests[0], "Portico-Playback"), "2")

    def test_stop_helper_declares_the_observation_protocol(self):
        client = fixture_client([FakeResponse(status=204)])

        playback_smoke.stop(client, {"id": "session-1"})

        request = client.opener.requests[0]
        self.assertEqual(request.get_method(), "DELETE")
        self.assertEqual(sent_header(request, "Portico-Playback"), "2")


class SeedHistoryFlowTests(unittest.TestCase):
    def test_history_claims_observes_and_stops_without_a_timeline_route(self):
        client = fixture_client([
            FakeResponse(status=204),
            FakeResponse({"id": "session-1",
                          "presentation": {"url": "/media/1", "generation": 4}}, status=201),
            grant(),
            ack(1),
            FakeResponse(status=204),
        ])
        report = {"created": {"history": []}}

        created = seed_state.start_history(client, "item-1", 0.25, report, duration_ms=100000)

        self.assertTrue(created)
        self.assertEqual([sent_path(request) for request in client.opener.requests], [
            "/v1/me/devices/current/capabilities",
            "/v1/playback/sessions",
            "/v1/playback/sessions/session-1/observer",
            "/v1/playback/sessions/session-1/observation",
            "/v1/playback/sessions/session-1",
        ])
        start, stop = client.opener.requests[1], client.opener.requests[4]
        self.assertEqual(start.get_method(), "POST")
        self.assertEqual(stop.get_method(), "DELETE")
        self.assertEqual(sent_body(stop), {"positionMs": 25000})
        self.assertEqual(sent_body(client.opener.requests[2]), {"generation": 4})
        self.assertEqual(sent_body(client.opener.requests[3])["state"], "paused")
        self.assertEqual(sent_body(client.opener.requests[3])["positionMs"], 25000)
        self.assertEqual(report["created"]["history"], ["item-1"])
        self.assertNotIn("/timeline", "".join(
            sent_path(request) for request in client.opener.requests))


class PlaybackObserverTests(unittest.TestCase):
    def test_claim_posts_only_the_generation_and_requires_the_grant(self):
        client = fixture_client([grant()])

        result = seed_state.claim_observer(client, "session-1", 7)

        self.assertEqual(result, {"epoch": EPOCH, "everyMs": 1000, "leaseUntil": LEASE})
        request = client.opener.requests[0]
        self.assertEqual(request.get_method(), "POST")
        self.assertEqual(sent_path(request), "/v1/playback/sessions/session-1/observer")
        self.assertEqual(sent_body(request), {"generation": 7})
        self.assertEqual(sent_header(request, "Portico-Playback"), "2")

    def test_a_malformed_grant_is_refused_not_accepted(self):
        for document, field in (
                ({"epoch": "short", "everyMs": 1000, "leaseUntil": LEASE}, "epoch"),
                ({"epoch": EPOCH, "everyMs": 0, "leaseUntil": LEASE}, "everyMs"),
                ({"epoch": EPOCH, "everyMs": 1000, "leaseUntil": "tomorrow"}, "leaseUntil"),
                ({"epoch": EPOCH, "everyMs": 1000}, "leaseUntil")):
            with self.subTest(field=field):
                client = fixture_client([FakeResponse(document)])
                with self.assertRaises(seed_state.Refusal) as caught:
                    seed_state.claim_observer(client, "session-1", 7)
                self.assertEqual(caught.exception.code, "observer_grant_shape")
                self.assertEqual(caught.exception.detail, field)

    def test_samples_follow_the_production_observer_sequence(self):
        clock = FakeClock()
        client = fixture_client([grant(), ack(1), ack(2)])
        observer = seed_state.Observer(client, "session-1", 7, now=clock)

        observer.observe("playing", 0)
        clock.advance(1.5)
        observer.observe("paused", 2500, seeked=True)

        claim_request, first, second = client.opener.requests
        self.assertEqual(sent_path(claim_request), "/v1/playback/sessions/session-1/observer")
        self.assertEqual(sent_body(claim_request), {"generation": 7})
        self.assertEqual(sent_body(first), {
            "epoch": EPOCH, "state": "playing", "seq": 1, "generation": 7,
            "positionMs": 0, "monotonicMs": 0, "playClockMs": 0,
            "seekSerial": 0, "rate": 1.0})
        self.assertEqual(sent_body(second), {
            "epoch": EPOCH, "state": "paused", "seq": 2, "generation": 7,
            "positionMs": 2500, "monotonicMs": 1500, "playClockMs": 1500,
            "seekSerial": 1, "rate": 1.0})
        self.assertEqual(sent_path(first), "/v1/playback/sessions/session-1/observation")
        self.assertEqual(sent_path(second), "/v1/playback/sessions/session-1/observation")
        for request in client.opener.requests:
            self.assertEqual(sent_header(request, "Portico-Playback"), "2")

    def test_the_play_clock_counts_only_time_actually_playing(self):
        clock = FakeClock()
        client = fixture_client([grant(), ack(1), ack(2), ack(3)])
        observer = seed_state.Observer(client, "session-1", 7, now=clock)

        observer.observe("playing", 0)
        clock.advance(2.0)
        observer.observe("paused", 2000)
        clock.advance(3.0)
        observer.observe("paused", 2000)

        _, playing, paused, still_paused = client.opener.requests
        self.assertEqual(sent_body(playing)["playClockMs"], 0)
        self.assertEqual(sent_body(paused)["playClockMs"], 2000)
        self.assertEqual(sent_body(paused)["monotonicMs"], 2000)
        self.assertEqual(sent_body(still_paused)["playClockMs"], 2000)
        self.assertEqual(sent_body(still_paused)["monotonicMs"], 5000)

    def test_an_epoch_change_is_surfaced_not_retried(self):
        client = fixture_client([
            grant(EPOCH),
            FakeResponse({"error": {"code": "observation_epoch_changed",
                                    "message": "Another player now reports this playback."}}, 409),
        ])
        observer = seed_state.Observer(client, "session-1", 7, now=FakeClock())

        with self.assertRaises(seed_state.Refusal) as caught:
            observer.observe("playing", 1000)

        self.assertEqual(caught.exception.code, "observation_epoch_changed")
        self.assertEqual(caught.exception.status, 409)
        self.assertEqual(len(client.opener.requests), 2)

    def test_an_ack_without_accepted_seq_is_refused_not_accepted(self):
        client = fixture_client([grant(), FakeResponse({"leaseUntil": LEASE})])
        observer = seed_state.Observer(client, "session-1", 7, now=FakeClock())

        with self.assertRaises(seed_state.Refusal) as caught:
            observer.observe("playing", 0)

        self.assertEqual(caught.exception.code, "observation_ack_shape")
        self.assertEqual(caught.exception.detail, "acceptedSeq")

    def test_an_ended_sample_stops_further_reporting(self):
        client = fixture_client([grant(), ack(1)])
        observer = seed_state.Observer(client, "session-1", 7, now=FakeClock())

        observer.observe("ended", 5000)

        with self.assertRaises(seed_state.Refusal) as caught:
            observer.observe("playing", 0)
        self.assertEqual(caught.exception.code, "observer_ended")
        self.assertEqual(len(client.opener.requests), 2)

    def test_the_fixture_modules_name_no_playback_timeline_route(self):
        for name in ("seed_state.py", "playback_smoke.py"):
            source = Path(__file__).with_name(name).read_text(encoding="utf-8")
            tree = ast.parse(source)
            for node in ast.walk(tree):
                if isinstance(node, ast.Constant) and isinstance(node.value, str):
                    self.assertNotIn("/timeline", node.value, name)

    def test_bodies_match_the_current_openapi(self):
        default = Path(__file__).resolve().parents[2] / "server/api/portico.openapi.json"
        spec_path = default
        self.assertTrue(spec_path.is_file(), "canonical server OpenAPI must be present")
        spec = json.loads(spec_path.read_text(encoding="utf-8"))
        observation = spec.get("paths", {}).get("/v1/playback/sessions/{id}/observation", {})
        claim = spec.get("paths", {}).get("/v1/playback/sessions/{id}/observer", {})
        self.assertIn("put", observation, "current observation route must remain registered")
        self.assertIn("post", claim, "current observer route must remain registered")
        body_schema = observation["put"]["requestBody"]["content"]["application/json"]["schema"]
        self.assertLessEqual(set(body_schema["required"]), set(body_schema["properties"]))
        claim_schema = claim["post"]["requestBody"]["content"]["application/json"]["schema"]
        self.assertEqual(claim_schema["required"], ["generation"])

        client = fixture_client([grant(), ack(1), ack(1)])
        observer = seed_state.Observer(client, "session-1", 7, now=FakeClock())
        observer.observe("paused", 2500)
        sample = sent_body(client.opener.requests[1])
        self.assertEqual(set(sample), set(body_schema["required"]))
        self.assertLessEqual(set(sample), set(body_schema["properties"]))
        self.assertEqual(set(sent_body(client.opener.requests[0])), {"generation"})


class SessionRefreshTests(unittest.TestCase):
    """The session is rotated the ordinary way, and only the ordinary way.

    ``POST /v1/auth/refresh`` takes no access bearer, answers the session
    envelope directly, and rotates both credentials. A lost answer is retried
    with the same request ID because the server replays that receipt; a
    refused refresh is not answered with a new sign-in.
    """

    def test_a_sign_in_is_captured_with_its_refresh_credential(self):
        client = session_client([signed_in()])

        signed, token = seed_state.sign_in(client, "owner", "password")

        self.assertEqual(token, "access-1")
        self.assertEqual(client.token, "access-1")
        self.assertEqual(client.refresh_token, "refresh-1")
        self.assertEqual(client.session_family, FAMILY)
        self.assertEqual(client.device_id, DEVICE)
        self.assertEqual(client.profile_id, PROFILE)
        self.assertEqual(client.expires_at, seed_state._instant(EXPIRES))
        self.assertEqual(signed["profiles"][0]["id"], PROFILE)

    def test_a_session_near_expiry_rotates_before_the_next_read(self):
        client = session_client([signed_in(), rotation(), FakeResponse({"id": "server-1"})])
        seed_state.sign_in(client, "owner", "password")
        client.clock = expiry_clock(30)

        client.read("/v1/system")

        paths = [sent_path(request) for request in client.opener.requests]
        self.assertEqual(paths, ["/v1/direct/sign-in", "/v1/auth/refresh", "/v1/system"])
        refresh = client.opener.requests[1]
        self.assertEqual(refresh.get_method(), "POST")
        self.assertEqual(set(sent_body(refresh)),
                         {"refreshToken", "installationId", "requestId"})
        self.assertEqual(sent_body(refresh)["refreshToken"], "refresh-1")
        self.assertEqual(sent_body(refresh)["installationId"], INSTALLATION)
        self.assertRegex(sent_body(refresh)["requestId"], r"^[A-Za-z0-9_-]{8,128}$")
        self.assertIsNone(sent_header(refresh, "Authorization"),
                          "the refresh route takes no access bearer")
        self.assertEqual(sent_header(client.opener.requests[2], "Authorization"),
                         "Bearer access-2")
        self.assertEqual(client.refresh_token, "refresh-2")
        self.assertEqual(client.expires_at, seed_state._instant(ROTATED_EXPIRES))
        self.assertEqual(client.generation, 2)
        self.assertEqual((client.device_id, client.session_family, client.profile_id),
                         (DEVICE, FAMILY, PROFILE))
        self.assertEqual(client.refreshes, 1)

    def test_a_read_401_rotates_once_and_replays_the_same_read(self):
        client = session_client([signed_in(), denial(), rotation(),
                                 FakeResponse({"id": "server-1"})])
        seed_state.sign_in(client, "owner", "password")

        client.read("/v1/system")

        paths = [sent_path(request) for request in client.opener.requests]
        self.assertEqual(paths, ["/v1/direct/sign-in", "/v1/system", "/v1/auth/refresh",
                                 "/v1/system"])
        replay = client.opener.requests[3]
        self.assertEqual(replay.get_method(), "GET")
        self.assertEqual(sent_header(replay, "Authorization"), "Bearer access-2")
        self.assertEqual(client.refreshes, 1)

    def test_a_write_401_is_not_replayed_with_a_rotated_token(self):
        client = session_client([signed_in(), denial()])
        seed_state.sign_in(client, "owner", "password")

        with self.assertRaises(seed_state.Refusal) as caught:
            client.call("POST", "/v1/libraries/library-1/scans", {})

        self.assertEqual(caught.exception.status, 401)
        self.assertEqual([sent_path(request) for request in client.opener.requests],
                         ["/v1/direct/sign-in", "/v1/libraries/library-1/scans"])
        self.assertEqual(client.refreshes, 0)
        self.assertEqual(client.refresh_token, "refresh-1")

    def test_a_refused_refresh_ends_the_run_instead_of_signing_in_again(self):
        client = session_client([signed_in(), denial(), denial()])
        seed_state.sign_in(client, "owner", "password")

        with self.assertRaises(seed_state.Refusal) as caught:
            client.read("/v1/system")

        self.assertEqual(caught.exception.code, "session_refresh_refused")
        self.assertEqual(caught.exception.status, 401)
        self.assertEqual(caught.exception.detail, "unauthorized")
        self.assertEqual([sent_path(request) for request in client.opener.requests],
                         ["/v1/direct/sign-in", "/v1/system", "/v1/auth/refresh"])
        # No second sign-in, so no account is re-authenticated and no
        # restricted profile can be selected without its PIN.
        self.assertEqual(client.token, "access-1")
        self.assertEqual(client.refresh_token, "refresh-1")
        self.assertEqual(client.refreshes, 0)

    def test_a_session_that_is_not_a_usable_envelope_is_refused(self):
        broken_sessions = (
            (session("access-1", "refresh-1", expires="soon"), "expiresAt"),
            ({field: value for field, value in session("access-1", "refresh-1").items()
              if field != "sessionFamilyId"}, "sessionFamilyId"))
        for broken, detail in broken_sessions:
            with self.subTest(detail=detail):
                client = session_client([FakeResponse({"session": broken})])

                with self.assertRaises(seed_state.Refusal) as caught:
                    seed_state.sign_in(client, "owner", "password")

                self.assertEqual(caught.exception.code, "session_envelope_shape")
                self.assertEqual(caught.exception.detail, detail)
                self.assertEqual((client.token, client.refresh_token), ("", ""))

    def test_a_rotation_that_changes_scope_is_refused_without_taking_effect(self):
        for overrides, detail in (({"family": "family-2"}, "sessionFamilyId"),
                                  ({"device": "device-2"}, "deviceId"),
                                  ({"profile": "profile-2"}, "viewer.profileId")):
            with self.subTest(detail=detail):
                client = session_client([signed_in(), rotation(**overrides)])
                seed_state.sign_in(client, "owner", "password")

                with self.assertRaises(seed_state.Refusal) as caught:
                    client.refresh()

                self.assertEqual(caught.exception.code, "session_refresh_scope")
                self.assertEqual(caught.exception.detail, detail)
                self.assertEqual(client.token, "access-1")
                self.assertEqual(client.refresh_token, "refresh-1")

    def test_a_refresh_that_did_not_rotate_is_refused_and_not_believed(self):
        for overrides, detail in (({"refresh": "refresh-1"}, "refreshToken"),
                                  ({"generation": "1"}, "tokenGeneration")):
            with self.subTest(detail=detail):
                client = session_client([signed_in(), rotation(**overrides)])
                seed_state.sign_in(client, "owner", "password")

                with self.assertRaises(seed_state.Refusal) as caught:
                    client.refresh()

                self.assertEqual(caught.exception.code, "session_refresh_not_rotated")
                self.assertEqual(caught.exception.detail, detail)
                self.assertEqual(client.refresh_token, "refresh-1")

    def test_a_lost_refresh_answer_is_replayed_with_the_same_request_id(self):
        client = session_client([signed_in(), urllib.error.URLError("answer lost"),
                                 rotation()])
        seed_state.sign_in(client, "owner", "password")

        client.refresh()

        refresh_requests = client.opener.requests[1:]
        self.assertEqual([sent_path(request) for request in refresh_requests],
                         ["/v1/auth/refresh", "/v1/auth/refresh"])
        self.assertEqual(sent_body(refresh_requests[0]), sent_body(refresh_requests[1]))
        self.assertEqual(client.refresh_token, "refresh-2")
        self.assertEqual(client.refreshes, 1)

    def test_a_profile_selection_replaces_the_account_session_it_was_given(self):
        selected = FakeResponse({"session": session("access-selected", "refresh-selected")})
        client = session_client([signed_in(profile=""), selected, rotation(),
                                 FakeResponse({"id": "server-1"})])

        seed_state.sign_in(client, "owner", "password")
        token = seed_state.select_profile(client, PROFILE)
        self.assertEqual(token, "access-selected")
        self.assertEqual(client.refresh_token, "refresh-selected")
        self.assertEqual(client.profile_id, PROFILE)
        client.clock = expiry_clock(30)

        client.read("/v1/system")

        refresh = client.opener.requests[2]
        self.assertEqual(sent_path(refresh), "/v1/auth/refresh")
        # The rotated session belongs to the profile just selected, not to the
        # account-only session that signed in.
        self.assertEqual(sent_body(refresh)["refreshToken"], "refresh-selected")
        self.assertEqual(sent_header(client.opener.requests[3], "Authorization"),
                         "Bearer access-2")
        self.assertEqual(client.profile_id, PROFILE)

    def test_an_expiring_session_without_a_refresh_credential_is_not_invented(self):
        client = session_client([FakeResponse({"session": session("access-1", "")}),
                                 FakeResponse({"id": "server-1"})])
        seed_state.sign_in(client, "owner", "password")
        client.clock = expiry_clock(30)

        client.read("/v1/system")

        self.assertEqual([sent_path(request) for request in client.opener.requests],
                         ["/v1/direct/sign-in", "/v1/system"])
        self.assertEqual(client.refreshes, 0)

    def test_a_reply_that_merely_mentions_an_access_token_is_not_a_session(self):
        client = session_client([signed_in(), FakeResponse({"accessToken": "other-1"})])
        seed_state.sign_in(client, "owner", "password")

        client.read("/v1/me/access-tokens")

        self.assertEqual(client.token, "access-1")
        self.assertEqual(client.refresh_token, "refresh-1")


class LibrarySettingsReadbackTests(unittest.TestCase):
    """A music library reads back the loudness step the server must run.

    ``internal/catalog/library_sources.go`` AppliesLoudness/musicLoudnessPolicy
    measures source loudness for every music library outside a file-list-only
    scan, so the served analysis is the stored policy's, not the saved page's.
    The read-back comparison derives that one step from the matrix and still
    fails on any other difference, and the online-enrichment guard stays exact.
    """

    # The published matrix shape: exactly the operations
    # administration.AnalysisOperations publishes, which are the static list
    # filtered by analysisRuntime. artwork_extraction, chapter_read,
    # deep_stream_analysis, subtitle_extraction, video_preview,
    # subtitle_download and lyrics are not runtime-backed, so a client never
    # sees them and this fixture must not choose them either.
    EVERY = ["movie", "show", "episode", "music", "book"]
    VIDEO = ["movie", "show", "episode"]
    MATRIX = {"costClasses": [{"id": "metadata-only", "order": 1},
                              {"id": "single-pass-read", "order": 2},
                              {"id": "decode", "order": 3},
                              {"id": "network", "order": 4}],
              "operations": [
                  {"id": "probe", "costClass": "metadata-only", "network": False,
                   "mediaKinds": EVERY, "requires": []},
                  {"id": "local_metadata", "costClass": "metadata-only", "network": False,
                   "mediaKinds": EVERY, "requires": ["probe"]},
                  {"id": "subtitle_discovery", "costClass": "metadata-only", "network": False,
                   "mediaKinds": VIDEO, "requires": ["probe"]},
                  {"id": "loudness", "costClass": "single-pass-read", "network": False,
                   "mediaKinds": EVERY, "requires": ["probe"]},
                  {"id": "audio_fingerprint", "costClass": "single-pass-read", "network": False,
                   "mediaKinds": ["music"], "requires": ["probe"]},
                  {"id": "trickplay", "costClass": "decode", "network": False,
                   "mediaKinds": VIDEO, "requires": ["probe"]},
                  {"id": "chapter_images", "costClass": "decode", "network": False,
                   "mediaKinds": VIDEO, "requires": ["probe"]},
                  {"id": "checksum", "costClass": "single-pass-read", "network": False,
                   "mediaKinds": EVERY, "requires": []},
                  {"id": "waveform", "costClass": "decode", "network": False,
                   "mediaKinds": EVERY, "requires": ["probe"]}]}
    ASKED = ["probe", "local_metadata"]
    MUSIC_READBACK = ["local_metadata", "loudness", "probe"]

    def document(self, kind="music", depth="basic"):
        return {"libraryKind": kind, "revision": 4, "analysisMatrix": self.MATRIX,
                "defaults": {},
                "settings": {"scanDepth": depth, "providers": [{"provider": "local",
                                                                "mediaKind": kind}],
                             "analysis": ["local_metadata", "probe"],
                             "acoustidEnabled": False, "segmentDetection": "server"}}

    def readback(self, analysis, *, providers=None, kind="music", depth="basic", **extra):
        settings = {"analysis": analysis,
                    "providers": providers or [{"provider": "local", "mediaKind": kind}],
                    "acoustidEnabled": False, "segmentDetection": "off"}
        settings.update(extra)
        owner = FakeAdmin([self.document(kind, depth), {"settings": settings}])
        report = {"libraries": {}}
        seed_state.quiet_library(owner, "Synthetic Music", "library-1", report)
        return owner, report

    def test_the_mandatory_music_step_is_derived_from_the_published_matrix(self):
        self.assertEqual(
            seed_state.mandatory_analysis(self.document("music", "basic"), "basic"),
            {"loudness"})
        self.assertEqual(
            seed_state.mandatory_analysis(self.document("music", "file_list_only"),
                                         "file_list_only"), set())
        self.assertEqual(
            seed_state.mandatory_analysis(self.document("book", "basic"), "basic"), set())

    def test_only_published_local_metadata_steps_are_asked_for(self):
        self.assertEqual(seed_state.metadata_only_analysis(self.document("music")),
                         self.ASKED)
        self.assertEqual(seed_state.metadata_only_analysis(self.document("tv")),
                         self.ASKED + ["subtitle_discovery"])
        self.assertEqual(seed_state.metadata_only_analysis(self.document("book")),
                         self.ASKED)

    def test_a_music_library_reads_back_without_a_false_failure(self):
        owner, report = self.readback(self.MUSIC_READBACK)

        self.assertNotIn("failures", report)
        written = owner.writes[0][2]["settings"]
        # The fixture itself still asks for metadata only: loudness is the
        # server's step, not one this page turns on.
        self.assertEqual(written["analysis"], self.ASKED)
        self.assertEqual([provider["provider"] for provider in written["providers"]],
                         ["local"])
        self.assertFalse(written["acoustidEnabled"])
        self.assertEqual(written["segmentDetection"], "off")

    def test_another_step_in_the_readback_is_still_a_failure(self):
        _, report = self.readback(self.MUSIC_READBACK + ["waveform"])

        failure = report["failures"][0]
        self.assertEqual(failure["step"], "library-settings-readback")
        self.assertEqual(failure["mismatched"], ["analysis"])
        self.assertEqual(failure["expectedAnalysis"], self.MUSIC_READBACK)

    def test_a_missing_mandatory_step_is_still_a_failure(self):
        _, report = self.readback(self.ASKED)

        self.assertEqual(report["failures"][0]["mismatched"], ["analysis"])

    def test_an_online_provider_or_online_audio_setting_is_still_a_failure(self):
        _, providers = self.readback(
            self.MUSIC_READBACK,
            providers=[{"provider": "local"}, {"provider": "musicbrainz"}])
        self.assertEqual(providers["failures"][0]["mismatched"], ["providers"])

        _, acoustid = self.readback(self.MUSIC_READBACK, acoustidEnabled=True)
        self.assertEqual(acoustid["failures"][0]["mismatched"], ["acoustidEnabled"])

        _, segments = self.readback(self.MUSIC_READBACK, segmentDetection="server")
        self.assertEqual(segments["failures"][0]["mismatched"], ["segmentDetection"])


if __name__ == "__main__":
    unittest.main()
