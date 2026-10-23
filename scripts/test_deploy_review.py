import contextlib
import importlib.util
import io
import os
import subprocess
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

    def test_outgoing_service_environments_are_preserved_by_actual_path(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            active = root / 'review-current.env'
            extra = root / 'review-extra.env'
            active.write_text('PORTICO_DATA_DIR=/var/lib/current-review\n')
            extra.write_text('GOMAXPROCS=1\n')
            active.chmod(0o600)
            command = root / 'systemctl'
            import shlex
            value = shlex.quote(str(active)) + ' (ignore_errors=no) ' + shlex.quote(str(extra)) + ' (ignore_errors=no)'
            command.write_text('#!/bin/sh\nprintf \'%s\\n\' ' + shlex.quote(value) + '\n')
            command.chmod(0o755)
            backup = root / 'backup'
            backup.mkdir()
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'])
            subprocess.run(['bash', '-c', 'backup="$1"\n' + deploy.BACKUP_ENVIRONMENTS, 'test', str(backup)], env=env, check=True)
            for source in (active, extra):
                saved = backup / 'environments' / source.relative_to('/')
                self.assertEqual(saved.read_bytes(), source.read_bytes())
                self.assertEqual(saved.stat().st_mode & 0o777, source.stat().st_mode & 0o777)

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

    def test_dependency_seed_with_other_lock_is_refused_before_copy(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            project, donor = root / 'project', root / 'donor'
            project.mkdir(); donor.mkdir()
            (project / 'package-lock.json').write_text('current lock')
            (donor / 'package-lock.json').write_text('older lock')
            with patch.object(apk.subprocess, 'run') as run:
                with self.assertRaises(ValueError):
                    apk.prepare_dependencies(project, donor / 'node_modules')
                run.assert_not_called()

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
