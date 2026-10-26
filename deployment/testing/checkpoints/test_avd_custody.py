#!/usr/bin/env python3
"""Synthetic regression tests for the AVD custody helper. No adb, CLI or device.

Run directly: `python3 test_avd_custody.py`. Every case uses fake callables and a
scratch lock directory, so nothing here touches an emulator, a session or a host.
"""
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SPEC = Path(__file__).with_name('avd_custody.py')
MODULE = importlib.util.spec_from_file_location('avd_custody', SPEC)
avd_custody = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(avd_custody)

AVD = 'PorticoTest_Phone_API36'
SERIAL = 'emulator-5554'
DISPLAY = 'PorticoTest Phone API36'


def document(*sessions):
    return {'success': True, 'data': {'sessions': list(sessions)}}


def row(name, serial=SERIAL, device=DISPLAY, platform='android', target='mobile'):
    return {'name': name, 'id': serial, 'device': device, 'platform': platform,
            'target': target, 'createdAt': 1792964662693, 'address': name,
            'runnerLogPath': '/private/runner.log', 'sessionStateDir': '/private/session'}


class CustodyTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.lock_dir = Path(self.scratch.name)
        # The helper's lock path is fixed at /tmp; the synthetic tests assert the real
        # path so a live AVD name is never created here except the lock file itself.
        self.receipt = self.lock_dir / 'receipt.json'
        self.effects = []

    def scenario(self, before=(), after=None):
        """One combined fake: the list as seen before and after the acquire.

        `after` defaults to the admitted row. The same state drives both callables, so
        a conflict test cannot accidentally pair an acquire from another scenario.
        """
        state = {'acquired': False}
        listed_after = [row('t3-synthetic0001')] if after is None else list(after)

        def sessions():
            return document(*(listed_after if state['acquired'] else before))

        def acquire():
            opened = {'sessionId': 't3-synthetic0001', 'serial': SERIAL, 'avd': AVD}
            state['acquired'] = True
            return opened

        return sessions, acquire

    def lease(self, before=(), after=None, **kwargs):
        sessions, acquire = self.scenario(before=before, after=after)
        options = dict(acquire=acquire, sessions=sessions, receipt=self.receipt,
                       adb_name=lambda serial: AVD)
        options.update(kwargs)
        return avd_custody.lease(AVD, SERIAL, **options)

    # 1. A refused admission runs no install, use or cleanup, and releases the lock.
    def test_admission_refusal_runs_no_body(self):
        def refused():
            self.effects.append('acquire')
            raise avd_custody.DeviceInUse()
        with self.assertRaises(avd_custody.DeviceInUse):
            with self.lease(acquire=refused) as held:
                self.effects.append('body')
        self.assertEqual(self.effects, ['acquire'])
        self.assertEqual(avd_custody.probe(AVD), 'free')
        recorded = json.loads(self.receipt.read_text())
        self.assertFalse(recorded['admitted'])
        self.assertFalse(recorded['lockHeld'])
        self.assertNotIn('sessionHandback', [k for k, v in recorded['sessionHandback'].items() if v])

    # 2. A serial that is not the named AVD refuses before any acquisition.
    def test_serial_avd_mismatch_does_not_acquire(self):
        def should_not_run():
            self.effects.append('acquire')
            return {'sessionId': 't3-synthetic0001', 'serial': SERIAL, 'avd': AVD}
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(adb_name=lambda serial: 'Portico_TV_API36', acquire=should_not_run):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'SERIAL_AVD_MISMATCH')
        self.assertEqual(self.effects, [])
        self.assertEqual(avd_custody.probe(AVD), 'free')

    # 3. A claimant existing before the lease skips before prepare AND before open.
    def test_stale_claimant_skips_before_prepare_and_open(self):
        def should_not_prepare():
            self.effects.append('prepare')
            return {'serial': SERIAL, 'avd': AVD}

        def should_not_open():
            self.effects.append('acquire')
            return {'sessionId': 't3-synthetic0001', 'serial': SERIAL, 'avd': AVD}

        with self.assertRaises(avd_custody.DeviceInUse):
            with self.lease(before=[row('t3-synthetic0001')],
                            prepare=should_not_prepare, acquire=should_not_open):
                self.effects.append('body')
        self.assertEqual(self.effects, [])
        self.assertEqual(avd_custody.probe(AVD), 'free')
        self.assertEqual(json.loads(self.receipt.read_text())['outcome'], 'in-use')

    # 4. The exclusive window: lock held through the body, explicit handback once.
    def test_full_window_with_explicit_handback(self):
        handbacks = []
        def close(identity):
            handbacks.append(dict(identity))
            return {'operation': 'app-close', 'result': 'closed'}
        with self.lease(handback=close) as held:
            self.assertEqual(avd_custody.probe(AVD), 'busy')
            self.assertEqual(held['sessionId'], 't3-synthetic0001')
            recorded = json.loads(self.receipt.read_text())
            self.assertTrue(recorded['admitted'] and recorded['lockHeld'])
        self.assertEqual(handbacks, [{'sessionId': 't3-synthetic0001', 'serial': SERIAL, 'avd': AVD}])
        self.assertEqual(avd_custody.probe(AVD), 'free')
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['sessionHandback'],
                         {'attempted': True, 'operation': 'app-close', 'result': 'closed'})
        self.assertEqual(recorded['agentSessionRetirement'], 'unproven by this helper')
        self.assertTrue(recorded['lockReleasedAt'])

    # 5. No handback callback means the helper says so instead of implying a close.
    def test_no_handback_is_reported_not_implied(self):
        with self.lease():
            pass
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['sessionHandback'], {'attempted': False})
        self.assertEqual(recorded['physicalState'], 'retained/unresolved by this helper')

    # 6. Strict acquire shape: no coercion, no extra or missing keys.
    def test_acquire_shape_is_strict(self):
        for opened in ({'sessionId': 't3-synthetic0001', 'serial': SERIAL},
                       {'sessionId': 't3-synthetic0001', 'serial': SERIAL, 'avd': AVD, 'extra': 'x'},
                       {'sessionId': 7, 'serial': SERIAL, 'avd': AVD},
                       't3-synthetic0001'):
            with self.assertRaises(avd_custody.CustodyRefused) as caught:
                with self.lease(acquire=lambda opened=opened: opened):
                    self.effects.append('body')
            self.assertEqual(caught.exception.code, 'ACQUIRE_SHAPE')
        self.assertEqual(self.effects, [])
        self.assertEqual(avd_custody.probe(AVD), 'free')

    # 7. A foreign session on the SAME SERIAL but a different device name is a conflict.
    def test_same_serial_wrong_device_name_conflicts(self):
        foreign = row('t3-foreign0001', serial=SERIAL, device='Portico_TV_API36')
        with self.assertRaises(avd_custody.DeviceInUse):
            with self.lease(before=[foreign]):
                self.effects.append('body')
        self.assertEqual(self.effects, [])

    # 7b. The same AVD on ANOTHER serial is also a conflict.
    def test_same_avd_other_serial_conflicts(self):
        foreign = row('t3-foreign0002', serial='emulator-5556', device=DISPLAY)
        with self.assertRaises(avd_custody.DeviceInUse):
            with self.lease(before=[foreign]):
                self.effects.append('body')
        self.assertEqual(self.effects, [])

    # 7c. A conflict that appears only after admission is refused, handle persisted.
    def test_conflict_after_admission_persists_handle(self):
        foreign = row('t3-foreign0003', serial=SERIAL, device='Portico_TV_API36')
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(after=[row('t3-synthetic0001'), foreign]):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'SESSION_NOT_EXCLUSIVE')
        self.assertEqual(self.effects, [])
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['openedSession'], 't3-synthetic0001')
        self.assertFalse(recorded['admitted'])

    # 9. A wrong device in the acquire mapping refuses, and nothing is used.
    def test_acquire_identity_mismatch(self):
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(acquire=lambda: {'sessionId': 't3-synthetic0001',
                                             'serial': 'emulator-5556', 'avd': AVD}):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'ACQUIRE_IDENTITY')
        self.assertEqual(self.effects, [])

    # 10. A handback that fails is recorded as refused, and the lock still releases.
    def test_handback_refusal_is_recorded(self):
        def refuse(identity):
            raise avd_custody.CustodyRefused('supported-close-absent')
        with self.lease(handback=refuse):
            pass
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['sessionHandback'],
                         {'attempted': True, 'result': 'refused', 'code': 'supported-close-absent'})
        self.assertEqual(avd_custody.probe(AVD), 'free')

    # 11. A body error still releases the lock and keeps the admitted fact.
    def test_body_error_releases_and_keeps_admission(self):
        with self.assertRaises(RuntimeError):
            with self.lease():
                raise RuntimeError('boom')
        recorded = json.loads(self.receipt.read_text())
        self.assertTrue(recorded['admitted'])
        self.assertFalse(recorded['lockHeld'])
        self.assertEqual(avd_custody.probe(AVD), 'free')

    # 12. `prepare` runs under the lock, so a boot or device_open callback sees it held.
    def test_prepare_callback_sees_lock_held(self):
        saw = {}

        def prepare():
            saw['lock'] = avd_custody.probe(AVD)
            return {'serial': SERIAL, 'avd': AVD}

        with self.lease(prepare=prepare):
            pass
        self.assertEqual(saw['lock'], 'busy')
        recorded = json.loads(self.receipt.read_text())
        self.assertTrue(recorded.get('prepared'))

    # 13. `prepare` that names a different device refuses before identity and open.
    def test_prepare_identity_is_strict(self):
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(prepare=lambda: {'serial': 'emulator-5556', 'avd': AVD}):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'PREPARE_IDENTITY')
        self.assertEqual(self.effects, [])

    # 14. A handback that returns None is not success.
    def test_handback_none_is_not_success(self):
        with self.lease(handback=lambda identity: None):
            pass
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['sessionHandback'], {'attempted': True, 'result': 'unspecified'})

    # 15. A handback that returns an empty mapping is not success either.
    def test_handback_empty_mapping_is_not_success(self):
        with self.lease(handback=lambda identity: {}):
            pass
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['sessionHandback'], {'attempted': True, 'result': 'unspecified'})



    # 19. The returned session listed twice is a duplicate, not an admission.
    def test_duplicate_same_name_is_refused(self):
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(after=[row('t3-synthetic0001'), row('t3-synthetic0001')]):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'SESSION_NOT_EXCLUSIVE')
        self.assertEqual(self.effects, [])

    # 20. The admitted row must be android.
    def test_admitted_row_must_be_android(self):
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(after=[row('t3-synthetic0001', platform='ios')]):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'SESSION_WRONG_PLATFORM')
        self.assertEqual(self.effects, [])

    # 21. The admitted row must name this serial and this AVD.
    def test_admitted_row_must_name_this_device(self):
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(after=[row('t3-synthetic0001', device='Portico_TV_API36')]):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'SESSION_NOT_ADMITTED')
        self.assertEqual(self.effects, [])

    # 22. A receipt parent that does not exist refuses before prepare, acquire or body.
    def test_missing_receipt_parent_refuses_before_anything(self):
        def should_not_run(tag):
            def call():
                self.effects.append(tag)
                return {'serial': SERIAL, 'avd': AVD} if tag == 'prepare' else {'sessionId': 't3-synthetic0001', 'serial': SERIAL, 'avd': AVD}
            return call
        missing = self.lock_dir / 'absent' / 'receipt.json'
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(receipt=missing, prepare=should_not_run('prepare'), acquire=should_not_run('acquire')):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'RECEIPT_UNAVAILABLE')
        self.assertEqual(self.effects, [])
        self.assertEqual(avd_custody.probe(AVD), 'free')

    # 23. A lease without a receipt path is refused outright.
    def test_receipt_is_required(self):
        with self.assertRaises(avd_custody.CustodyRefused) as caught:
            with self.lease(receipt=None):
                self.effects.append('body')
        self.assertEqual(caught.exception.code, 'RECEIPT_REQUIRED')
        self.assertEqual(self.effects, [])

    # 24. Callback refusal codes are normalised to a bounded closed form.
    def test_callback_codes_are_normalised(self):
        def refuse(identity):
            raise avd_custody.CustodyRefused('Not A Code: raw detail')
        with self.lease(handback=refuse):
            pass
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['sessionHandback'],
                         {'attempted': True, 'result': 'refused', 'code': 'unspecified'})

    # 25. A lowercase hyphenated callback code is kept as given.
    def test_bounded_callback_code_is_kept(self):
        def refuse(identity):
            raise avd_custody.CustodyRefused('panel-already-gone')
        with self.lease(handback=refuse):
            pass
        recorded = json.loads(self.receipt.read_text())
        self.assertEqual(recorded['sessionHandback'],
                         {'attempted': True, 'result': 'refused', 'code': 'panel-already-gone'})


if __name__ == '__main__':
    unittest.main(verbosity=1)
