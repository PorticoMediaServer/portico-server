import contextlib
import importlib.util
import io
import base64
from pathlib import Path
import tarfile
import tempfile
import unittest
import zipfile
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('deploy_review', Path(__file__).with_name('deploy-review.py'))
deploy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deploy)
spec = importlib.util.spec_from_file_location('build_review', Path(__file__).with_name('build-review.py'))
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)
spec = importlib.util.spec_from_file_location('build_review_apk', Path(__file__).with_name('build-review-apk.py'))
apk = importlib.util.module_from_spec(spec)
spec.loader.exec_module(apk)


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

    def test_official_ed25519_public_root_is_accepted(self):
        pem = Path(__file__).resolve().parents[1] / 'deployment/hosted/root-public-key.pem'
        value = build.public_root(pem)
        self.assertEqual(len(base64.urlsafe_b64decode(value + '=')), 32)

    def test_other_public_key_format_is_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            pem = Path(temp) / 'root.pem'
            pem.write_text('-----BEGIN PUBLIC KEY-----\n' + base64.b64encode(b'wrong format').decode() + '\n-----END PUBLIC KEY-----\n')
            with self.assertRaises(ValueError):
                build.public_root(pem)

    def make_apk(self, path, extra=(), omit=()):
        files = {'assets/index.android.bundle': b'x' * 2048,
                 'assets/media-engine/THIRD-PARTY-NOTICES.txt': b'notices',
                 'lib/arm64-v8a/libporticompv.so': b'current bridge',
                 'lib/arm64-v8a/libmpv.so': b'engine'}
        with zipfile.ZipFile(path, 'w') as archive:
            for name, data in files.items():
                if name not in omit:
                    archive.writestr(name, data)
            for name in extra:
                archive.writestr(name, b'wrong architecture')

    def test_embedded_arm64_apk_with_current_bridge_is_accepted(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'review.apk'
            self.make_apk(path)
            apk.validate_apk(path)

    def test_foreign_abi_and_missing_bridge_are_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'review.apk'
            self.make_apk(path, extra=['lib/x86_64/libmpv.so'])
            with self.assertRaises(ValueError):
                apk.validate_apk(path)
            self.make_apk(path, omit=['lib/arm64-v8a/libporticompv.so'])
            with self.assertRaises(ValueError):
                apk.validate_apk(path)
