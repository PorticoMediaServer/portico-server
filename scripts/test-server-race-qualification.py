#!/usr/bin/env python3
"""Release custody boundaries: complete coverage and current-run evidence."""
import copy
import importlib.util
import json
import pathlib
import re
import tempfile
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location('qualification', ROOT / 'scripts/qualify-server-race.py')
q = importlib.util.module_from_spec(spec)
spec.loader.exec_module(q)
ORIGIN = {'GITHUB_RUN_ID': '123', 'GITHUB_RUN_ATTEMPT': '2', 'GITHUB_SHA': 'a' * 40,
          'GITHUB_REPOSITORY': q.REPOSITORY, 'GITHUB_EVENT_NAME': 'workflow_dispatch',
          'GITHUB_WORKFLOW_SHA': 'a' * 40, 'GITHUB_WORKFLOW_REF': q.REPOSITORY + '/.github/workflows/release.yml@refs/tags/v0.0.1'}


def coverage():
    plan = {'schema': 1, 'shards': 16, 'goVersion': 'go1.26.10', 'flags': q.FLAGS,
            'environment': q.BASE_ENV, 'origin': ORIGIN, 'sourceDigest': 'b' * 64,
            'codeFiles': q.code_hashes(), 'packages': [
                {'module': 'server', 'package': 'example/suite', 'directory': 'source/suite',
                 'goPackage': 'suite', 'tests': ['Example_answer', 'FuzzDecode', 'TestRoundTrip']},
                {'module': 'apikit', 'package': 'example/init', 'directory': 'apikit/init',
                 'goPackage': 'initpkg', 'tests': []}]}
    shards = []
    for index in range(16):
        invocations = []
        for group in q.package_groups(plan, index):
            names = group['names']
            events = [{'Action': 'start', 'Package': group['package']}]
            for name in names:
                events.extend([{'Action': 'run', 'Package': group['package'], 'Test': name},
                               {'Action': 'pass', 'Package': group['package'], 'Test': name}])
            events.append({'Action': 'pass', 'Package': group['package']})
            pattern = '^(' + '|'.join(re.escape(name) for name in names) + ')$' if names else '^$'
            invocations.append({**group, 'flags': [*q.FLAGS, '-run', pattern, group['package']],
                                'exitCode': 0, 'events': events, 'skips': [], 'logSha256': 'c' * 64})
        shards.append({'schema': 1, 'origin': ORIGIN, 'sourceDigest': plan['sourceDigest'],
                       'planSha256': q.sha(q.encoded(plan)), 'shard': index, 'invocations': invocations})
    return plan, shards


class CoverageBoundaries(unittest.TestCase):
    def setUp(self):
        self.context = patch.object(q, 'origin', return_value=ORIGIN)
        self.context.start()
        self.addCleanup(self.context.stop)
        self.plan, self.shards = coverage()

    def test_complete_examples_fuzz_and_no_test_package(self):
        report = q.validate_results(self.plan, self.shards)
        self.assertEqual(sorted(n for r in report['results'] for n in r['names']),
                         ['Example_answer', 'FuzzDecode', 'TestRoundTrip'])
        self.assertEqual(sum(r['initOnly'] for r in report['results']), 1)

    def test_each_function_and_package_assigned_exactly_once(self):
        rows = [r for s in self.shards for r in s['invocations']]
        self.assertEqual(len([r for r in rows if r['package'] == 'example/init']), 1)
        for name in ['Example_answer', 'FuzzDecode', 'TestRoundTrip']:
            self.assertEqual(sum(name in r['names'] for r in rows), 1)

    def test_missing_duplicate_or_empty_shard_fails(self):
        cases = [self.shards[:-1], self.shards[:-1] + [self.shards[0]]]
        missing = copy.deepcopy(self.shards)
        next(s for s in missing if s['invocations'])['invocations'].clear()
        cases.append(missing)
        for shards in cases:
            with self.subTest(shards=len(shards)), self.assertRaises(ValueError):
                q.validate_results(self.plan, shards)

    def test_example_and_fuzz_must_run_and_complete(self):
        for name in ['Example_answer', 'FuzzDecode', 'TestRoundTrip']:
            shards = copy.deepcopy(self.shards)
            for shard in shards:
                for row in shard['invocations']:
                    row['events'] = [e for e in row['events'] if e.get('Test') != name]
            with self.subTest(name=name), self.assertRaises(ValueError):
                q.validate_results(self.plan, shards)

    def test_canceled_skipped_foreign_or_uninstrumented_invocation_fails(self):
        mutations = [lambda r: r.update(exitCode=-15),
                     lambda r: r['events'][-1].update(Action='skip'),
                     lambda r: r['events'][-1].update(Package='foreign/package'),
                     lambda r: r['flags'].remove('-race')]
        for mutate in mutations:
            shards = copy.deepcopy(self.shards)
            mutate(next(r for s in shards for r in s['invocations']))
            with self.assertRaises(ValueError): q.validate_results(self.plan, shards)

    def test_source_workflow_builder_run_and_attempt_mismatches_fail(self):
        for field in ['GITHUB_SHA', 'GITHUB_WORKFLOW_SHA', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT', 'GITHUB_WORKFLOW_REF']:
            plan = copy.deepcopy(self.plan)
            plan['origin'][field] = 'foreign'
            with self.subTest(field=field), self.assertRaises(ValueError): q.validate_plan(plan)
        plan = copy.deepcopy(self.plan)
        plan['codeFiles']['scripts/build-server-release.py'] = '0' * 64
        with self.assertRaises(ValueError): q.validate_plan(plan)
        shards = copy.deepcopy(self.shards)
        shards[0]['sourceDigest'] = '0' * 64
        with self.assertRaises(ValueError): q.validate_results(self.plan, shards)

    def test_missing_foreign_expired_or_ambiguous_artifact_rejected_before_download(self):
        good = {'name': 'server-race-plan-2', 'id': 1, 'expired': False,
                'workflow_run': {'id': 123, 'head_sha': ORIGIN['GITHUB_SHA']}}
        foreign = copy.deepcopy(good); foreign['workflow_run']['id'] = 456
        expired = copy.deepcopy(good); expired['expired'] = True
        wrong_head = copy.deepcopy(good); wrong_head['workflow_run']['head_sha'] = 'd' * 40
        with patch.object(q.subprocess, 'run') as download:
            for inventory in [[], [good, good], [foreign], [expired], [wrong_head]]:
                with self.assertRaises(ValueError): q.fetch_artifact('server-race-plan-2', inventory)
            download.assert_not_called()

    def test_failed_missing_or_duplicate_job_cannot_authenticate(self):
        run = {'head_sha': ORIGIN['GITHUB_SHA'], 'run_attempt': 2, 'event': 'workflow_dispatch',
               'head_repository': {'full_name': q.REPOSITORY}, 'repository': {'full_name': q.REPOSITORY}}
        names = ['server-race-prepare', *[f'server-race-shard-{i}' for i in range(16)]]
        jobs = [{'id': i, 'name': name, 'status': 'completed', 'conclusion': 'success'} for i, name in enumerate(names)]
        failed = copy.deepcopy(jobs); failed[3]['conclusion'] = 'cancelled'
        skipped = copy.deepcopy(jobs); skipped[3]['conclusion'] = 'skipped'
        with patch.object(q, 'api', return_value=run), patch.object(q, 'fetch_artifact') as fetch:
            for rows in [jobs[:-1], jobs + [jobs[0]], failed, skipped]:
                with patch.object(q, 'pages', return_value=rows), self.assertRaises(ValueError): q.fetched_report()
            fetch.assert_not_called()

    def test_nonrace_job_must_finish_successfully_before_authentication(self):
        run = {'head_sha': ORIGIN['GITHUB_SHA'], 'run_attempt': 2, 'event': 'workflow_dispatch',
               'head_repository': {'full_name': q.REPOSITORY}, 'repository': {'full_name': q.REPOSITORY}}
        names = ['server-race-prepare', *[f'server-race-shard-{i}' for i in range(16)], 'server-race-merge']
        jobs = [{'id': i, 'name': name, 'status': 'completed', 'conclusion': 'success'} for i, name in enumerate(names)]
        with patch.object(q, 'api', return_value=run), patch.object(q, 'fetch_artifact') as fetch:
            for outcome in ['missing', 'skipped', 'cancelled', 'failure']:
                rows = jobs if outcome == 'missing' else jobs + [{'id': 30, 'name': 'qualification', 'status': 'completed', 'conclusion': outcome}]
                with patch.object(q, 'pages', return_value=rows), self.assertRaises(ValueError):
                    q.fetched_report(require_merge=True)
            fetch.assert_not_called()

    def test_source_declared_skip_retains_reason_and_is_not_reported_passed(self):
        shards = copy.deepcopy(self.shards)
        row = next(r for s in shards for r in s['invocations'] if 'TestRoundTrip' in r['names'])
        next(e for e in row['events'] if e.get('Test') == 'TestRoundTrip' and e['Action'] == 'pass')['Action'] = 'skip'
        with self.assertRaises(ValueError): q.validate_results(self.plan, shards)
        row['skips'] = [{'test': 'TestRoundTrip', 'reason': 'fixture.go:12: release tier only'}]
        report = q.validate_results(self.plan, shards)
        self.assertEqual(report['functions'], {'discovered': 3, 'passed': 2, 'skipped': 1})
        self.assertEqual(report['sourceDeclaredSkips'][0]['reason'], 'fixture.go:12: release tier only')

    def test_unsigned_or_tampered_passed_json_cannot_seal(self):
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        key = Ed25519PrivateKey.generate()
        public = key.public_key().public_bytes_raw()
        report = q.validate_results(self.plan, self.shards)
        report['authentication'] = {'id': 22, 'digest': 'sha256:' + 'd' * 64}
        required = ['server-race-prepare', *[f'server-race-shard-{i}' for i in range(16)], 'server-race-merge', 'qualification']
        report['evidence'] = {'jobs': [{'id': i, 'name': name, 'conclusion': 'success'} for i, name in enumerate(required)]}
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'report.json'
            q.write(path, report)
            with self.assertRaises(FileNotFoundError): q.verify_report(path, public, 'b' * 64, self.plan['codeFiles']['scripts/build-server-release.py'])
            path.with_suffix('.sig').write_bytes(key.sign(path.read_bytes()))
            q.verify_report(path, public, 'b' * 64, self.plan['codeFiles']['scripts/build-server-release.py'])
            with self.assertRaises(ValueError): q.verify_report(path, public, 'wrong-source', self.plan['codeFiles']['scripts/build-server-release.py'])
            path.write_bytes(path.read_bytes() + b' ')
            with self.assertRaises(Exception): q.verify_report(path, public, 'b' * 64, self.plan['codeFiles']['scripts/build-server-release.py'])


if __name__ == '__main__':
    unittest.main()
