"""Pure tests for the Magazine Issues prime retry and observation policy.

Run directly: python3 deployment/testing/walks/server/test_state_prime.py
"""
from __future__ import annotations

import importlib.util
import pathlib
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parent
_SPEC = importlib.util.spec_from_file_location(
    'walks_server', ROOT / '__init__.py', submodule_search_locations=[str(ROOT)])
_MODULE = importlib.util.module_from_spec(_SPEC)
sys.modules['walks_server'] = _MODULE
_SPEC.loader.exec_module(_MODULE)

from walks_server import state

ITEMS = {'items': [{'id': 'item', 'sourceId': 'source', 'format': 'epub'}]}
SESSION = {'sessionId': 'session', 'metadata': {'publicationType': {'value': 'magazine'}}}


class Clock:
    def __init__(self, step):
        self.now = 0
        self.step = step

    def monotonic(self):
        return self.now

    def sleep(self, seconds):
        self.now += self.step


class PrimeTest(unittest.TestCase):
    def run_prime(self, responses, step=30, calls=None):
        calls = [] if calls is None else calls
        sequence = iter(responses)

        def fake_call(client, method, path, body=None, **kwargs):
            calls.append((method, path, kwargs.get('timeout')))
            value = next(sequence)
            if isinstance(value, BaseException):
                raise value
            return value

        previous_call, previous_time = state._call, state.time
        state._call, state.time = fake_call, Clock(step)
        try:
            return state._prime_magazine_issues(
                None, 'token', 'installation', {'Magazine Issues': 'library'}), calls
        finally:
            state._call, state.time = previous_call, previous_time

    def changed(self):
        return state.StateError('api-http', 412, 'periodicals-read')

    def assert_closed(self, calls):
        self.assertIn(('DELETE', '/v1/reading/sessions/session', None), calls)

    def assert_poll_timeouts(self, calls):
        polls = [timeout for method, path, timeout in calls
                 if path.endswith('/reading/periodicals')]
        self.assertTrue(polls)
        for timeout in polls:
            self.assertIsInstance(timeout, (int, float))
            self.assertGreater(timeout, 0)
            self.assertLessEqual(timeout, 45)

    def test_412_then_groups(self):
        result, calls = self.run_prime(
            [ITEMS, SESSION, self.changed(), {'groups': [{'id': 'group'}]}, None])
        self.assertEqual(result, 1)
        self.assertEqual(sum(1 for method, path, timeout in calls
                             if path.endswith('/reading/periodicals')), 2)
        self.assert_poll_timeouts(calls)
        self.assert_closed(calls)

    def test_repeated_412_keeps_last_when_latest(self):
        first, last = self.changed(), self.changed()
        calls = []
        with self.assertRaises(state.StateError) as caught:
            self.run_prime([ITEMS, SESSION, first, last, None], calls=calls)
        self.assertIs(caught.exception, last)
        self.assertEqual((caught.exception.code, caught.exception.status,
                          caught.exception.operation),
                         ('api-http', 412, 'periodicals-read'))
        self.assert_closed(calls)

    def test_412_then_empty_timeout_reports_empty(self):
        calls = []
        with self.assertRaises(state.StateError) as caught:
            self.run_prime([ITEMS, SESSION, self.changed(), {'groups': []}, None],
                           calls=calls)
        self.assertEqual(caught.exception.code, 'magazine-group-missing')
        self.assertIsNone(caught.exception.status)
        self.assert_closed(calls)

    def test_other_http_status_propagates(self):
        calls = []
        with self.assertRaises(state.StateError) as caught:
            self.run_prime([ITEMS, SESSION,
                            state.StateError('api-http', 500, 'periodicals-read'), None],
                           calls=calls)
        self.assertEqual((caught.exception.code, caught.exception.status,
                          caught.exception.operation),
                         ('api-http', 500, 'periodicals-read'))
        self.assert_closed(calls)

    def test_empty_200_timeout_reports_empty(self):
        calls = []
        with self.assertRaises(state.StateError) as caught:
            self.run_prime([ITEMS, SESSION, {'groups': []}, {'groups': []}, None],
                           calls=calls)
        self.assertEqual(caught.exception.code, 'magazine-group-missing')
        self.assert_closed(calls)

    def test_412_after_empty_is_latest(self):
        calls = []
        with self.assertRaises(state.StateError) as caught:
            self.run_prime([ITEMS, SESSION, {'groups': []}, self.changed(), None],
                           calls=calls)
        self.assertEqual((caught.exception.code, caught.exception.status,
                          caught.exception.operation),
                         ('api-http', 412, 'periodicals-read'))
        self.assert_closed(calls)


if __name__ == '__main__':
    unittest.main()
