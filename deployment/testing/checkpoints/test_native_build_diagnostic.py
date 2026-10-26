#!/usr/bin/env python3
"""Pure tests for the native build's closed diagnostic vocabulary.

Imports the production module by path (its name carries a dash) and exercises the
real helper and receipt shaper; main() is never executed, nothing is built.
"""
import importlib.util
import unittest
from pathlib import Path

HERE = Path(__file__).parent
spec = importlib.util.spec_from_file_location('native_build', HERE / 'native-build.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

RUNTIME_TOKENS = ('bounded-build-timeout', 'existing-reader-differs',
                  'matching-source-map-unavailable', 'reader-budget',
                  'reader-evidence-unavailable', 'reader-hash', 'reader-member', 'reader-receipt')
UNAVAILABLE_TOKENS = ('native-lane-busy', 'native-source-dirty', 'native-unpublished-work',
                      'pinned-node-unavailable', 'gradle-busy')


class VocabularyTests(unittest.TestCase):
    def test_every_actual_runtime_token_is_kept(self):
        for token in RUNTIME_TOKENS:
            error = RuntimeError(token)
            self.assertEqual(module.closed_reason(error), token)
            receipt = module.receipt_for(error)
            self.assertEqual(receipt['status'], 'fail')
            self.assertEqual(receipt['reason'], token)

    def test_every_unavailable_token_keeps_status_and_reason(self):
        for token in UNAVAILABLE_TOKENS:
            error = module.Unavailable(token)
            receipt = module.receipt_for(error)
            self.assertEqual(receipt['status'], 'unavailable')
            self.assertEqual(receipt['reason'], token)

    def test_build_command_failed_family_is_bounded(self):
        for code in (1, 2, 127, 255):
            token = 'build-command-failed-%d' % code
            self.assertEqual(module.closed_reason(RuntimeError(token)), token)
        for token in ('build-command-failed-0', 'build-command-failed-256',
                      'build-command-failed-1000', 'build-command-failed-01',
                      'build-command-failed-', 'build-command-failed-x'):
            self.assertIsNone(module.closed_reason(RuntimeError(token)), token)

    def test_unknown_word_path_message_and_nonstring_are_class_only(self):
        for value in ('unexpected-reader-state', '/home/runner/work/jobs/x',
                      'gradle failed with exit 1', 'ReaderHash', ''):
            receipt = module.receipt_for(RuntimeError(value))
            self.assertEqual(receipt['status'], 'fail')
            self.assertNotIn('reason', receipt, value)
        for value in (1, None, b'reader-hash', ['reader-hash']):
            receipt = module.receipt_for(RuntimeError(value))
            self.assertNotIn('reason', receipt, value)
        self.assertIsNone(module.closed_reason(RuntimeError()))
        self.assertIsNone(module.closed_reason(ValueError('reader-hash')) if False else module.closed_reason(RuntimeError('reader-hash ')))

    def test_vocabulary_is_exact_and_finite(self):
        self.assertEqual(module.CLOSED_REASONS, frozenset(RUNTIME_TOKENS + UNAVAILABLE_TOKENS))
        self.assertEqual(len(module.CLOSED_REASONS), 13)

    def test_receipt_shape_is_stable(self):
        receipt = module.receipt_for(RuntimeError('reader-hash'))
        self.assertEqual(sorted(receipt), ['class', 'id', 'reason', 'status', 'surface'])
        self.assertEqual(receipt['surface'], 'android')
        self.assertEqual(receipt['id'], 'published-main-package')

    def test_cli_refusal_shapes_exit_without_starting_any_build(self):
        import contextlib, io, json, runpy, sys
        from unittest.mock import patch
        output=io.StringIO()
        # The fixed-config guard runs before any config read or subprocess.
        with patch.object(sys,'argv',['native-build.py','--config','/not-the-campaign-config']), \
             patch.object(module.subprocess,'Popen',side_effect=AssertionError('no build')), \
             contextlib.redirect_stdout(output),contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as exited:
                runpy.run_path(str(HERE/'native-build.py'),run_name='__main__')
        self.assertEqual(exited.exception.code,75)
        self.assertEqual(json.loads(output.getvalue())['status'],'unavailable')


if __name__ == '__main__':
    unittest.main(verbosity=1)
