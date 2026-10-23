import contextlib
import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('deploy_review', Path(__file__).with_name('deploy-review.py'))
deploy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deploy)


class ReviewDeployment(unittest.TestCase):
    def run_stage(self, member):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'portico-server').write_bytes(b'fixture executable')
            with tarfile.open(root / 'web.tar', 'w') as archive:
                archive.addfile(member)
            args = ['deploy-review.py', 'a' * 40, str(root), '--cycle', 'review-check', '--stage-only']
            with patch('sys.argv', args), patch.object(deploy, 'remote') as remote, patch.object(deploy.subprocess, 'run') as run:
                with contextlib.redirect_stderr(io.StringIO()):
                    deploy.main()
                return remote.call_count, run.call_count

    def test_valid_built_web_stages_only_to_fresh_review(self):
        self.assertEqual(self.run_stage(tarfile.TarInfo('./index.html')), (2, 2))

    def test_path_escape_is_refused_before_any_remote_call(self):
        with self.assertRaises(SystemExit):
            self.run_stage(tarfile.TarInfo('../index.html'))

    def test_links_are_refused_before_any_remote_call(self):
        member = tarfile.TarInfo('./index.html')
        member.type = tarfile.SYMTYPE
        member.linkname = '/etc/passwd'
        with self.assertRaises(SystemExit):
            self.run_stage(member)

    def test_archive_without_entry_is_refused(self):
        with self.assertRaises(SystemExit):
            self.run_stage(tarfile.TarInfo('assets/app.js'))

    def test_remote_shell_bodies_parse(self):
        import subprocess
        for body in (deploy.STAGE, deploy.ACTIVATE):
            self.assertEqual(subprocess.run(['bash', '-n'], input=body, text=True).returncode, 0)
