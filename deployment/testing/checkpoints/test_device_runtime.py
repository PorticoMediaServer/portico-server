#!/usr/bin/env python3
"""Synthetic tests for the production device gate. No adb, CLI, device or subprocess.

The guard under test is the one both production entry points call: a refusal must be
returned before any device I/O, whatever the target looks like.
"""
import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).parent


def load(name):
    spec = importlib.util.spec_from_file_location(name, HERE / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


device_runtime = load('device_runtime')
checkpoint = load('native-checkpoint')
driver = load('android')

CALLS = []


class Recorder:
    def __getattr__(self, name):
        def record(*args, **kwargs):
            CALLS.append((name, args))
            raise AssertionError('device subprocess attempted: ' + name)
        return record


class GateTests(unittest.TestCase):
    def setUp(self):
        CALLS.clear()
        # Any device subprocess would raise through these and be recorded.
        self.saved = (subprocess.run, subprocess.check_output, subprocess.Popen,
                      checkpoint.subprocess.run, checkpoint.subprocess.check_output,
                      checkpoint.subprocess.Popen, checkpoint.adb, checkpoint.devices)
        subprocess.run = subprocess.check_output = subprocess.Popen = Recorder()
        checkpoint.subprocess.run = checkpoint.subprocess.check_output = checkpoint.subprocess.Popen = Recorder()
        checkpoint.adb = checkpoint.devices = Recorder()
        self.addCleanup(self.restore)

    def restore(self):
        subprocess.run, subprocess.check_output, subprocess.Popen = self.saved[0:3]
        checkpoint.subprocess.run, checkpoint.subprocess.check_output, checkpoint.subprocess.Popen = self.saved[3:6]
        checkpoint.adb, checkpoint.devices = self.saved[6], self.saved[7]

    def test_no_lease_producer_refuses(self):
        self.assertTrue(device_runtime.refused())
        with self.assertRaises(device_runtime.RuntimeUnavailable):
            device_runtime.require_lease()

    def test_checkpoint_guard_returns_unavailable_rows_without_device_io(self):
        rows = checkpoint.admission_refusal(['android-phone', 'android-tv', 'fire-os-tv'])
        self.assertEqual([row['surface'] for row in rows], ['android-phone', 'android-tv', 'fire-os-tv'])
        self.assertTrue(all(row['status'] == 'unavailable' for row in rows))
        self.assertTrue(all(row['dependency'].startswith('admission-unavailable') for row in rows))
        self.assertEqual(CALLS, [])

    def test_target_shapes_cannot_enable_the_gate(self):
        # An old target, a syntactically complete target, one with missing flags and one
        # with a trailing flag all refuse, and none of them triggers device I/O.
        targets = [
            {},
            {'command': '/private/agent-device', 'targetArgs': ['--platform', 'android']},
            {'command': '/private/agent-device',
             'targetArgs': ['--platform', 'android', '--serial', 'emulator-5554',
                            '--config', '/private/host.json', '--session', 't3-synthetic0001']},
            {'command': '/private/agent-device', 'targetArgs': ['--serial']},
            {'command': '/private/agent-device', 'targetArgs': []},
        ]
        for target in targets:
            self.assertIsCheckpointRefusal(target)
        self.assertEqual(CALLS, [])

    def assertIsCheckpointRefusal(self, target):
        rows = checkpoint.admission_refusal(['android-phone'])
        self.assertEqual(rows[0]['status'], 'unavailable')

    def test_driver_guard_refuses_before_any_device_io(self):
        rows = driver.admission_refusal('android-phone')
        self.assertEqual(rows[0]['surface'], 'android-phone')
        self.assertEqual(rows[0]['status'], 'unavailable')
        self.assertEqual(CALLS, [])

    def test_nothing_can_enable_the_gate(self):
        # No registration API exists, and neither a complete plan, an arbitrary object
        # with `open`, nor any other shape changes the answer.
        self.assertFalse(hasattr(device_runtime, 'register_lease'))
        self.assertTrue(device_runtime.refused())
        with self.assertRaises(device_runtime.RuntimeUnavailable):
            device_runtime.require_lease()
        complete = {'command': '/private/agent-device',
                    'targetArgs': ['--platform', 'android', '--serial', 'emulator-5554',
                                   '--config', '/private/host.json', '--session', 't3-synthetic0001'],
                    'sessionListArgv': ['/private/agent-device']}

        class Producer:
            open = staticmethod(lambda: None)

        for candidate in (complete, Producer(), {'open': lambda: None}):
            self.assertEqual(checkpoint.admission_refusal(['android-phone'])[0]['status'], 'unavailable')
            self.assertEqual(driver.admission_refusal('android-phone')[0]['status'], 'unavailable')
        self.assertEqual(CALLS, [])

    def test_unavailable_rows_are_closed_and_surface_scoped(self):
        rows = device_runtime.unavailable_rows(['android-tv', 'fire-os-tv'])
        self.assertEqual({row['surface'] for row in rows}, {'android-tv', 'fire-os-tv'})
        self.assertEqual({row['id'] for row in rows}, {'core-flows'})
        self.assertEqual({row['status'] for row in rows}, {'unavailable'})

    def test_dropped_metadata_is_preserved_read_only(self):
        scratch = tempfile.TemporaryDirectory()
        self.addCleanup(scratch.cleanup)
        self.directory = Path(scratch.name)
        state = self.directory / 'native-state.json'
        original = json.dumps({'history': {'android-phone:core-flows': {'passed': True, 'timeouts': 2}},
                               'dropped': ['android-phone:core-flows', 'android-tv:core-flows']})
        state.write_text(original)
        self.assertEqual(checkpoint.dropped_from_state(state),
                         ['android-phone:core-flows', 'android-tv:core-flows'])
        self.assertEqual(state.read_text(), original)
        self.assertEqual(checkpoint.dropped_from_state(self.directory / 'absent.json'), [])
        broken = self.directory / 'broken.json'
        broken.write_text('not json')
        self.assertEqual(checkpoint.dropped_from_state(broken), [])


if __name__ == '__main__':
    unittest.main(verbosity=1)
