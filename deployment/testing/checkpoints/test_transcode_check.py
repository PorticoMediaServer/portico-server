#!/usr/bin/env python3
"""Synthetic tests for the bounded transcode check. No network, process or device.

Covers the owner's merge seams: a 202 carrying a real session view is owned and
cleaned up, 202->201 identity must stay stable, pure retry-only owns nothing, the
retry hint is one strict integer, custody is rechecked, budgets cap the wait,
cleanup failure cannot pass, and the probe must show a real video stream within
the requested ceiling.
"""
import importlib.util
import json
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE))

from api_smoke.client import Reply
from transcode_check.check import (Check, CheckFailure, HEIGHT_CEILING, action_class, bucket,
                                   mode_class, policy_mode, retry_hint, route_class,
                                   session_ref, status_class)
from transcode_check.media import (MediaError, admit_ffprobe, admit_media_path, classify,
                                   first_segment)


class FakeClient:
    def __init__(self, routes):
        self.routes = routes
        self.calls = []

    def call(self, operation, method, path, **kwargs):
        self.calls.append((operation, method))
        handler = self.routes.get(operation)
        if handler is None:
            raise AssertionError('unexpected operation: ' + operation)
        return handler(method, path, kwargs)


def reply(status=200, data=None, code=None, failure=None):
    return Reply('op', 'GET', status=status, data=data, code=code, failure=failure)


VIEW_BODY = {
    'id': 'sess-1', 'state': 'playing', 'revision': 'r1',
    'presentation': {'mode': 'stream', 'container': 'fmp4_hls', 'url': '/v1/media/grant',
                     'generation': 2, 'videoHeight': 480, 'bitrateKbps': 400,
                     'decision': {'video': {'action': 'convert', 'reasons': ['bitrate_above_request']},
                                  'audio': {'action': 'copy', 'reasons': []}}},
    'playbackInfo': {'route': 'converted', 'reason': {'kind': 'quality'},
                     'delivered': {'video': {'codec': 'h264', 'height': 480}}},
}


class FakeMedia:
    def __init__(self, height=480, codec='h264', fetch_error=None, probe_available=True):
        self.fetch_error = fetch_error
        self.probe_facts = {'available': probe_available,
                            'reason': 'ok' if probe_available else 'probe-unavailable',
                            'format': 'mp4',
                            'video': {'codec': codec, 'width': 320, 'height': height},
                            'audio': {'codec': 'aac', 'channels': 1, 'sampleRate': 48000},
                            'subtitleTracks': 0}
        self.fetch_calls = 0

    def fetch(self, path, limit):
        self.fetch_calls += 1
        if self.fetch_error:
            raise MediaError(self.fetch_error)
        return {'sample': b'x' * 4096, 'bytes': 4096, 'status': 206,
                'content': 'video/mp4', 'via': 'segment', 'container': 'fmp4'}

    def probe(self, sample):
        return self.probe_facts


class FakeDeadline:
    def __init__(self, remaining=30.0):
        self._remaining = remaining
        self.slept = []

    def remaining(self):
        return self._remaining

    def expired(self):
        return self._remaining <= 0


def routes(start=None, view=VIEW_BODY, stop=204, readback=404, view_error=None, capacity=True):
    """start is a list of (status, body) replies served in order, last one repeating."""
    script = list(start or [(201, VIEW_BODY)])

    def start_handler(method, path, kwargs):
        item = script.pop(0) if len(script) > 1 else script[0]
        status, data = item
        return reply(status=status, data=data)

    def view_handler(method, path, kwargs):
        if view_error:
            raise view_error
        return reply(data=view)

    def stop_handler(method, path, kwargs):
        return reply(status=stop, data=None)

    def readback_handler(method, path, kwargs):
        if readback == 404:
            return reply(status=404, code='not_found', data={'error': {'code': 'not_found'}})
        return reply(status=200, data={'state': 'playing'})

    table = {
        'playback-delivery-policy': lambda m, p, k: reply(
            data={'policy': {'directPlay': 'prefer', 'directStream': 'allow',
                             'transcode': 'allow', 'clamps': []}}),
        'playback-start': start_handler,
        'playback-session': view_handler,
        'playback-stop': stop_handler,
        'playback-stop-read': readback_handler,
    }
    if capacity:
        table['transcode-capacity'] = lambda m, p, k: reply(data={
            'enabled': True, 'toolchain': {'encoders': {'libx264': True},
                                           'filters': {'scale': True}, 'muxers': {'hls': True}},
            'dependencies': {'ffmpeg': {'ok': True}, 'ffprobe': {'ok': True}},
            'temporaryDirectory': {'ready': True, 'fallback': False},
            'presets': [{'id': 'a'}], 'warnings': [],
            'hardware': {'configured': 'software', 'probes': [{'backend': 'vaapi'}]},
            'directStreamRemux': True, 'planningPolicy': 'software'})
    return table


def run(client, media=None, owner=True, deadline=None, sleep=None):
    return Check(client, deadline=deadline or FakeDeadline(), owner=owner, item='item-1',
                 media=media if media is not None else FakeMedia(),
                 sleep=sleep or (lambda d, s: True)).execute()


class CheckTests(unittest.TestCase):
    def test_converted_with_real_bytes_and_probe_passes(self):
        media = FakeMedia()
        document = run(FakeClient(routes()), media)
        self.assertEqual(document['result'], 'passed')
        self.assertEqual(document['session']['mode'], 'stream')
        self.assertTrue(document['cleanup']['stopped'] and document['cleanup']['ended'])
        self.assertEqual(media.fetch_calls, 1)

    def test_planned_conversion_without_a_real_video_probe_cannot_pass(self):
        for media in (FakeMedia(probe_available=False), FakeMedia(height=None),
                      FakeMedia(height=720), FakeMedia(codec='other')):
            document = run(FakeClient(routes()), media)
            self.assertEqual(document['result'], 'unavailable', media.probe_facts)

    def test_202_with_real_view_is_owned_then_201_must_keep_identity(self):
        first = (202, dict(VIEW_BODY, retryAfterMs=10))
        document = run(FakeClient(routes(start=[first, (201, VIEW_BODY)])))
        self.assertEqual(document['result'], 'passed')
        self.assertEqual(document['admission']['attempts'], bucket(1))
        self.assertTrue(document['cleanup']['stopped'])

    def test_202_then_201_with_a_different_id_fails_and_stops_the_owned_one(self):
        other = dict(VIEW_BODY, id='sess-2')
        client = FakeClient(routes(start=[(202, dict(VIEW_BODY, retryAfterMs=10)), (201, other)]))
        document = run(client)
        self.assertEqual(document['result'], 'failed')
        self.assertEqual(document['steps'][-1]['reason'], 'session-identity-changed')
        self.assertIn('playback-stop', [call[0] for call in client.calls])
        self.assertTrue(document['cleanup']['stopped'])

    def test_retry_exhausted_with_an_owned_session_is_unavailable_but_cleaned(self):
        client = FakeClient(routes(start=[(202, dict(VIEW_BODY, retryAfterMs=10))]))
        document = run(client)
        self.assertEqual(document['result'], 'unavailable')
        self.assertEqual(document['admission']['reason'], 'retry-exhausted')
        self.assertIn('playback-stop', [call[0] for call in client.calls])
        self.assertTrue(document['cleanup']['stopped'])

    def test_pure_retry_only_owns_nothing(self):
        client = FakeClient(routes(start=[(202, {'state': 'preparing', 'retryAfterMs': 10})]))
        document = run(client)
        self.assertEqual(document['result'], 'unavailable')
        self.assertEqual(document['admission']['reason'], 'retry-only')
        self.assertNotIn('playback-stop', [call[0] for call in client.calls])

    def test_retry_hint_must_be_one_strict_integer(self):
        for bad in (0, 20000, True, 'soon', 1.5):
            client = FakeClient(routes(start=[(202, dict(VIEW_BODY, retryAfterMs=bad))]))
            document = run(client)
            self.assertEqual(document['result'], 'failed', bad)
            self.assertEqual(document['steps'][-1]['reason'], 'retry-hint-invalid')
        self.assertEqual(retry_hint({'retryAfterMs': 1}), 1)
        self.assertEqual(retry_hint({'retryAfterMs': 10000}), 10000)
        with self.assertRaises(CheckFailure):
            retry_hint({})

    def test_deadline_budget_stops_retrying_without_waiting(self):
        deadline = FakeDeadline(remaining=0.2)
        calls = []

        def no_sleep(d, seconds):
            calls.append(seconds)
            return False

        client = FakeClient(routes(start=[(202, dict(VIEW_BODY, retryAfterMs=5000))]))
        document = run(client, deadline=deadline, sleep=no_sleep)
        self.assertEqual(document['admission']['reason'], 'retry-exhausted')
        self.assertEqual(calls, [])

    def test_custody_rebound_before_fetch_fails_and_still_stops(self):
        document = run(FakeClient(routes()), FakeMedia(fetch_error='custody-unverified'))
        self.assertEqual(document['result'], 'failed')
        self.assertEqual(document['steps'][-1]['reason'], 'custody-unverified')
        self.assertTrue(document['cleanup']['stopped'])

    def test_cleanup_failure_cannot_pass(self):
        document = run(FakeClient(routes(readback=200)))
        self.assertEqual(document['cleanup']['ended'], False)
        self.assertEqual(document['result'], 'failed')

    def test_unexpected_body_shape_is_a_closed_failure_with_a_receipt(self):
        for body in ({}, {'id': 'sess-1'}, {'id': 'sess-1', 'presentation': {}}):
            document = run(FakeClient(routes(start=[(201, body)])))
            self.assertEqual(document['result'], 'failed')
            self.assertIn('steps', document)
            self.assertIsNotNone(document['cleanup'])
        document = run(FakeClient(routes(view_error=RuntimeError('boom'))))
        self.assertEqual(document['result'], 'failed')
        self.assertEqual(document['steps'][-1]['reason'], 'unexpected-shape')
        self.assertIsNotNone(document['cleanup'])

    def test_member_context_skips_owner_capacity(self):
        client = FakeClient(routes(capacity=False))
        document = run(client, owner=False)
        self.assertIsNone(document['capacity'])
        self.assertNotIn('transcode-capacity', [c[0] for c in client.calls])
        self.assertEqual(document['result'], 'passed')

    def test_closed_classes_and_paths(self):
        self.assertEqual(mode_class('stream'), 'stream')
        self.assertEqual(mode_class('hls'), 'other')
        self.assertEqual(route_class('converted'), 'converted')
        self.assertEqual(action_class('convert'), 'convert')
        self.assertEqual(policy_mode('never'), 'never')
        self.assertEqual(status_class(206), 206)
        self.assertEqual(bucket(4096), 'at-least-256')
        self.assertEqual(classify(b'#EXTM3U\n'), 'playlist')
        self.assertEqual(first_segment('#EXTM3U\nseg1.ts\n', '/v1/media/g'), '/v1/media/seg1.ts')
        self.assertIsNone(first_segment('#EXTM3U\nhttp://elsewhere/x.ts\n', '/v1/media/g'))
        with self.assertRaises(MediaError):
            admit_media_path('https://elsewhere/x')
        with self.assertRaises(MediaError):
            admit_ffprobe('ffprobe')
        with self.assertRaises(CheckFailure):
            session_ref({'id': 'x', 'presentation': {'generation': 0}})

    def test_receipt_has_no_url_token_or_identity(self):
        document = run(FakeClient(routes()), FakeMedia())
        encoded = json.dumps(document)
        for forbidden in ('http://', 'Bearer', '/v1/media', 'grant', 'sess-1', 'item-1'):
            self.assertNotIn(forbidden, encoded)

    def test_height_ceiling_is_the_requested_one(self):
        self.assertEqual(HEIGHT_CEILING, 480)


if __name__ == '__main__':
    unittest.main(verbosity=1)
