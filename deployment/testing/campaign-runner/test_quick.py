#!/usr/bin/env python3
import contextlib
import datetime
import fcntl
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock
import job


class QuickTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.cfg = {"workRoot": str(self.root), "slots": str(self.root / "slots"),
                    "maxJobs": 5, "quickSlot": 4, "maxEmulators": 2,
                    "minMemoryBytes": 0, "minDiskFreeBytes": 0}
        (self.root / "slots").mkdir()
        self.held = []

    def tearDown(self):
        for fd in self.held:
            os.close(fd)
        self.tmp.cleanup()

    def hold(self, name):
        fd = os.open(self.root / "slots" / name, os.O_RDWR | os.O_CREAT, 0o600)
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        self.held.append(fd)

    def run_probe(self, **flags):
        with contextlib.redirect_stdout(io.StringIO()):
            return job.run({"probe": True, **flags}, self.cfg)

    def test_general_never_takes_reserved_slot(self):
        for n in range(4): self.hold('slot-%d.lock' % n)
        self.assertEqual(self.run_probe(), 75)
        self.assertEqual(self.run_probe(quick=True), 0)

    def test_quick_uses_idle_general_capacity_when_reserved_slot_busy(self):
        self.hold('slot-4.lock')
        self.assertEqual(self.run_probe(quick=True), 0)
        self.assertEqual(self.run_probe(), 0)

    def test_quick_prefers_reserved_slot(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertEqual(job.run({"probe": True, "quick": True}, self.cfg), 0)
        self.assertIn('"slot": 4', output.getvalue())

    def test_quick_cannot_exceed_total_host_slots(self):
        for n in range(5): self.hold('slot-%d.lock' % n)
        self.assertEqual(self.run_probe(quick=True), 75)
        self.assertEqual(self.run_probe(), 75)

    def test_captures_emulators_cannot_take_quick(self):
        self.assertEqual(self.run_probe(quick=True, browser=True), 75)
        self.assertEqual(self.run_probe(quick=True, emulator=True), 75)

    def test_two_emulators_each_require_general_slot(self):
        self.hold('emulator-0.lock'); self.hold('emulator-1.lock')
        self.assertEqual(self.run_probe(emulator=True), 75)
        self.assertEqual(self.run_probe(), 0)
        self.assertEqual(self.run_probe(quick=True), 0)

    def test_weekend_deadline_refuses_new_work(self):
        self.cfg['shutdownAt'] = '2020-10-26T09:00:00Z'
        self.assertEqual(self.run_probe(quick=True), 75)
        self.assertEqual(self.run_probe(), 75)

    def quota(self, cap, roots):
        self.cfg.update({"porticoCapBytes": cap, "porticoRoots": [str(root) for root in roots]})
        for root in roots:
            root.mkdir(exist_ok=True)

    @staticmethod
    def scan(returncode, stdout=""):
        return subprocess.CompletedProcess(["du"], returncode, stdout, "")

    def test_quota_scan_retries_after_concurrent_scratch_deletion(self):
        a, b = self.root / 'a', self.root / 'b'
        self.quota(100, [a, b])
        scans = [self.scan(0, "40\t%s\n" % a), self.scan(1, ""),
                 self.scan(0, "20\t%s\n" % a), self.scan(0, "20\t%s\n" % b)]
        with mock.patch.object(job.subprocess, "run", side_effect=scans) as run, \
                mock.patch.object(job.time, "sleep") as sleep:
            self.assertEqual(self.run_probe(), 0)
        self.assertEqual(run.call_count, 4)
        self.assertEqual(sleep.call_count, 1)

    def test_quota_scan_discards_partial_counts_between_attempts(self):
        a, b = self.root / 'a', self.root / 'b'
        self.quota(50, [a, b])
        scans = [self.scan(0, "40\t%s\n" % a), self.scan(1, "999\t%s\n" % b),
                 self.scan(0, "10\t%s\n" % a), self.scan(0, "10\t%s\n" % b)]
        with mock.patch.object(job.subprocess, "run", side_effect=scans) as run, \
                mock.patch.object(job.time, "sleep"):
            self.assertEqual(self.run_probe(), 0)
        self.assertEqual(run.call_count, 4)

    def test_quota_scan_fails_closed_when_no_full_scan_succeeds(self):
        a = self.root / 'a'
        self.quota(100, [a])
        output = io.StringIO()
        with mock.patch.object(job.subprocess, "run",
                               return_value=self.scan(1, "9\t%s\n" % a)) as run, \
                mock.patch.object(job.time, "sleep"), contextlib.redirect_stdout(output):
            self.assertEqual(job.run({"probe": True}, self.cfg), 75)
        self.assertEqual(run.call_count, job.FOOTPRINT_SCAN_ATTEMPTS)
        self.assertIn("REFUSED: Portico footprint unmeasurable", output.getvalue())
        self.assertEqual(list((self.root / 'slots').glob('slot-*.lock')), [])

    def test_quota_scan_fails_closed_when_du_cannot_start(self):
        a = self.root / 'a'
        self.quota(100, [a])
        output = io.StringIO()
        with mock.patch.object(job.subprocess, "run", side_effect=FileNotFoundError) as run, \
                mock.patch.object(job.time, "sleep"), contextlib.redirect_stdout(output):
            self.assertEqual(job.run({"probe": True}, self.cfg), 75)
        self.assertEqual(run.call_count, job.FOOTPRINT_SCAN_ATTEMPTS)
        self.assertIn("REFUSED: Portico footprint unmeasurable", output.getvalue())

    def test_quota_scan_fails_closed_when_existence_check_errors(self):
        a = self.root / 'a'
        self.quota(100, [a])
        output = io.StringIO()
        with mock.patch.object(job.Path, "exists", side_effect=OSError) as exists, \
                mock.patch.object(job.time, "sleep"), contextlib.redirect_stdout(output):
            self.assertEqual(job.run({"probe": True}, self.cfg), 75)
        self.assertEqual(exists.call_count, job.FOOTPRINT_SCAN_ATTEMPTS)
        self.assertIn("REFUSED: Portico footprint unmeasurable", output.getvalue())

    def test_quota_cap_refuses_a_complete_scan_over_limit(self):
        a = self.root / 'a'
        self.quota(100, [a])
        output = io.StringIO()
        with mock.patch.object(job.subprocess, "run",
                               return_value=self.scan(0, "101\t%s\n" % a)), \
                contextlib.redirect_stdout(output):
            self.assertEqual(job.run({"probe": True}, self.cfg), 75)
        self.assertIn("REFUSED: Portico footprint cap", output.getvalue())

    def test_quota_scan_skips_a_root_deleted_before_it_is_measured(self):
        self.cfg.update({"porticoCapBytes": 1,
                         "porticoRoots": [str(self.root / 'already-gone')]})
        with mock.patch.object(job.subprocess, "run") as run:
            self.assertEqual(self.run_probe(), 0)
        run.assert_not_called()

    def test_quick_real_child_and_slot_cleanup(self):
        root = self.root / 'jobs/check'
        (root / 'source').mkdir(parents=True)
        spec = {"root": str(root), "cwd": ".", "quick": True, "commit": "fixture", "lane": "fixture",
                "argv": [sys.executable, '-c', 'print("quick child ran")']}
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(job.run(spec, self.cfg), 0)
        self.assertEqual(self.run_probe(quick=True), 0)

    def test_overflow_child_keeps_quick_deadlines_and_releases_general_slot(self):
        self.hold('slot-4.lock')
        root = self.root / 'jobs/overflow'
        (root / 'source').mkdir(parents=True)
        spec = {"root": str(root), "cwd": ".", "quick": True, "commit": "fixture", "lane": "fixture",
                "argv": [sys.executable, '-c', 'pass']}
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertEqual(job.run(spec, self.cfg), 0)
        event = json.loads(output.getvalue().splitlines()[0])
        self.assertEqual(event['slot'], 0)
        self.assertTrue(event['quick'])
        self.assertEqual(event['softTimeoutSeconds'], 285)
        self.assertEqual(event['hardTimeoutSeconds'], 300)
        self.assertEqual(self.run_probe(), 0)

    def test_hard_timeout_kills_sigterm_ignoring_owned_group(self):
        code = 'import signal,time;signal.signal(signal.SIGTERM,signal.SIG_IGN);print("ready",flush=True);time.sleep(30)'
        child = subprocess.Popen([sys.executable, '-c', code], start_new_session=True, stdout=subprocess.PIPE)
        self.assertEqual(child.stdout.readline().strip(), b'ready')
        started = time.monotonic()
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(job.wait_child(child, .05, .15), 124)
        child.stdout.close()
        self.assertLess(time.monotonic() - started, 1)
        self.assertIsNotNone(child.returncode)


if __name__ == '__main__': unittest.main()
