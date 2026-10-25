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
spec = importlib.util.spec_from_file_location('smoke_review', Path(__file__).with_name('smoke-review.py'))
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


class ReviewDeployment(unittest.TestCase):
    def run_stage(self, member, extra=()):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'portico-server').write_bytes(b'fixture executable')
            with tarfile.open(root / 'web.tar', 'w') as archive:
                archive.addfile(member)
            args = ['deploy-review.py', 'a' * 40, str(root), '--cycle', 'review-check', '--stage-only', *extra]
            with patch('sys.argv', args), patch.object(deploy, 'remote') as remote, patch.object(deploy.subprocess, 'run') as run:
                with contextlib.redirect_stderr(io.StringIO()):
                    deploy.main()
                return remote.call_count, run.call_count

    def test_valid_built_web_stages_only_to_fresh_review(self):
        self.assertEqual(self.run_stage(tarfile.TarInfo('./index.html')), (2, 2))

    def test_installed_runtime_root_is_explicit_and_confined(self):
        self.assertEqual(self.run_stage(tarfile.TarInfo('./index.html'), ['--runtime-root', '/opt/portico/media-runtimes/f3ef43fa/bundle']), (2, 2))
        with self.assertRaises(SystemExit):
            self.run_stage(tarfile.TarInfo('./index.html'), ['--runtime-root', '/tmp/unqualified'])

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
        for body in (deploy.STAGE, deploy.ACTIVATE, deploy.ROLLBACK):
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

    def test_feature_or_stale_cutoff_is_refused_before_build(self):
        cutoff, current = 'a' * 40, 'b' * 40
        with patch.object(build.subprocess, 'check_output', return_value=current + '\trefs/heads/main\n'), patch.object(build.subprocess, 'run') as run:
            with self.assertRaisesRegex(ValueError, 'actual published origin/main'):
                build.require_current_main(Path('/checkout'), cutoff)
            run.assert_not_called()

    def test_current_remote_main_still_requires_checked_local_ancestry(self):
        cutoff = 'a' * 40
        with patch.object(build.subprocess, 'check_output', return_value=cutoff + '\trefs/heads/main\n'), patch.object(build.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, 'git')):
            with self.assertRaises(subprocess.CalledProcessError):
                build.require_current_main(Path('/checkout'), cutoff)

    def test_private_stdin_waits_for_a_whole_ready_line(self):
        self.assertFalse(smoke.stdin_ready('{"argv":["console.log(\"PRIVATE_STDIN_READY\")"]}'))
        self.assertTrue(smoke.stdin_ready('public metadata\nPRIVATE_STDIN_READY\n'))

    def test_private_credentials_refuse_unsafe_permissions_and_links(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'review.md'
            import json
            path.write_text('```json\n' + json.dumps({'username': 'fixture', 'password': os.urandom(24).hex()}) + '\n```')
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                smoke.read_credentials(path)
            path.chmod(0o600)
            self.assertEqual(smoke.read_credentials(path)['username'], 'fixture')
            path.write_text(json.dumps({'username': 'fixture', 'password': 'synthetic-test-value', 'url': 'https://demo.getportico.tv'}))
            self.assertEqual(smoke.read_credentials(path), {'username': 'fixture', 'password': 'synthetic-test-value'})
            link = Path(temp) / 'link'
            link.symlink_to(path)
            with self.assertRaises(OSError):
                smoke.read_credentials(link)

    def test_activation_without_real_browser_inputs_is_refused_before_mutation(self):
        with patch('sys.argv', ['deploy-review.py', 'a' * 40, '/artifact', '--cycle', 'review-test', '--activate-only']), patch.object(deploy, 'remote') as remote:
            with self.assertRaises(SystemExit):
                deploy.main()
            remote.assert_not_called()

    def test_refused_browser_launch_never_changes_the_public_service(self):
        args = ['deploy-review.py', 'a' * 40, '/artifact', '--cycle', 'review-test', '--activate-only', '--browser-repo', '/repo', '--runner', '/runner', '--private-review', '/private', '--smoke-output', '/output']
        with patch('sys.argv', args), patch.object(deploy, 'remote') as remote, patch.object(deploy.subprocess, 'run', side_effect=subprocess.CalledProcessError(75, 'smoke')):
            with self.assertRaises(subprocess.CalledProcessError):
                deploy.main()
            remote.assert_not_called()

    def test_browser_admission_is_a_whole_line_before_any_activation(self):
        self.assertFalse(smoke.browser_ready('{"argv":["console.log(BROWSER_ADMITTED)"]}'))
        activation = smoke.ReviewActivation('a' * 40, 'review-test', 'controls')
        with patch.object(smoke, 'load_deployment', return_value=deploy), patch.object(deploy, 'remote') as remote:
            activation.rollback()
            remote.assert_not_called()
            self.assertTrue(smoke.browser_ready('metadata\nBROWSER_ADMITTED\n'))
            activation.start()
            activation.rollback()
            self.assertEqual([call.args[0] for call in remote.call_args_list], [deploy.ACTIVATE, deploy.ROLLBACK])

    def test_frozen_cutoff_accepts_main_ancestry_when_the_tip_advances(self):
        cutoff, current = 'a' * 40, 'b' * 40
        with patch.object(build.subprocess, 'check_output', return_value=current + '\trefs/heads/main\n'), patch.object(build.subprocess, 'run') as run:
            build.require_current_main(Path('/checkout'), cutoff, frozen=True)
            run.assert_called_with(['git', '-C', '/checkout', 'merge-base', '--is-ancestor', cutoff, 'origin/main'], check=True)

    def test_frozen_cutoff_never_accepts_a_commit_outside_main(self):
        with patch.object(build.subprocess, 'check_output', return_value='b' * 40 + '\trefs/heads/main\n'), patch.object(build.subprocess, 'run', side_effect=[None, subprocess.CalledProcessError(1, 'git')]):
            with self.assertRaises(subprocess.CalledProcessError):
                build.require_current_main(Path('/checkout'), 'a' * 40, frozen=True)

    def test_activation_retry_preserves_the_same_state_and_separate_backup(self):
        with patch.object(smoke, 'load_deployment', return_value=deploy), patch.object(deploy, 'remote') as remote:
            smoke.ReviewActivation('a' * 40, 'review-test', 'controls').start()
            self.assertEqual(remote.call_args.args[2], '/var/lib/portico-review-test')
            self.assertEqual(remote.call_args.args[-1], 'review-test-controls')

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
                 'lib/arm64-v8a/libmpv.so': b'engine',
                 'assets/PorticoReader.bundle/index.html': b'production Reader entry',
                 'assets/PorticoReader.bundle/licences.json': b'{}',
                 'assets/PorticoReader.bundle/assets/pdf.worker-fixture.mjs': b'PDF worker'}
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

    def test_reader_entry_and_worker_must_be_packaged(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'review.apk'
            for missing in ['assets/PorticoReader.bundle/index.html',
                            'assets/PorticoReader.bundle/assets/pdf.worker-fixture.mjs']:
                with self.subTest(missing=missing):
                    self.make_apk(path, omit=[missing])
                    with self.assertRaises(ValueError):
                        apk.validate_apk(path)
