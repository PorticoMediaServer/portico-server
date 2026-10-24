import hashlib
import importlib.util
import io
import json
from pathlib import Path
import struct
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('sonic_bundle', Path(__file__).with_name('sonic-bundle.py'))
bundle = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bundle)


class SonicBundleTests(unittest.TestCase):
    def fixture(self, root, target='linux-x64'):
        runtime, model_lock = bundle.locks()
        model = root / 'model'
        model.mkdir()
        (model / 'model.onnx').write_bytes(b'unit fixture, never executed as a model')
        (model / 'LICENSE.md').write_bytes((bundle.ROOT / 'third_party/musicnn/LICENSE.md').read_bytes())
        model_lock['onnx']['bytes'] = (model / 'model.onnx').stat().st_size
        model_lock['onnx']['sha256'] = bundle.digest(model / 'model.onnx')
        descriptor = dict(id=model_lock['modelId'], version=model_lock['onnx']['version'], licence='ISC',
                          sourceCommit=model_lock['sourceCommit'], format='onnx', sha256=model_lock['onnx']['sha256'],
                          bytes=model_lock['onnx']['bytes'], preprocessing='musicnn-logmel-1', input='log_mel',
                          embedding='embedding', tags='tags', frames=187, mels=96, dimensions=200, tagCount=50,
                          dependencies={'tensorflow-cpu': '2.15.1', 'tf2onnx': '1.16.1', 'onnx': '1.16.2',
                                        'onnxruntime': '1.20.1', 'numpy': '1.26.4'})
        (model / 'model.json').write_text(json.dumps(descriptor))
        sdk = root / 'sdk'
        (sdk / 'lib').mkdir(parents=True)
        header = bytearray(64)
        header[:6] = b'\x7fELF\x02\x01'
        struct.pack_into('<H', header, 18, bundle.TARGETS[target])
        helper = root / 'portico-sonic'
        helper.write_bytes(header)
        real = sdk / 'lib/libonnxruntime.so.1.20.1'
        real.write_bytes(header)
        (sdk / 'lib/libonnxruntime.so.1').symlink_to(real.name)
        (sdk / 'lib/libonnxruntime.so').symlink_to(real.name)
        (sdk / 'LICENSE').write_text('Permission is hereby granted, free of charge')
        (sdk / 'ThirdPartyNotices.txt').write_text('original fixture notices')
        lock_patch = patch.object(bundle, 'locks', return_value=(runtime, model_lock))
        lock_patch.start()
        self.addCleanup(lock_patch.stop)
        def needed(file):
            return ['libonnxruntime.so.1', 'libc.so.6'] if file == helper else ['libstdc++.so.6', 'libm.so.6']
        return helper, sdk, model, needed

    def package(self, root, target='linux-x64'):
        helper, sdk, model, needed = self.fixture(root, target)
        archive = root / 'bundle.tar.xz'
        with patch.object(bundle, 'needed', side_effect=needed):
            bundle.package(helper, sdk, model, target, archive, 'a' * 40)
        output = root / 'extracted'
        bundle.extract(archive, output, target)
        return archive, output

    def test_actual_needed_closure_only_and_original_notices_survive_round_trip(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            _, output = self.package(root)
            manifest = bundle.verify(output, 'linux-x64')
            self.assertEqual(manifest['externalSystemLibraries'], ['libc.so.6', 'libm.so.6', 'libstdc++.so.6'])
            self.assertEqual((output / 'licenses/sonic/onnxruntime-ThirdPartyNotices.txt').read_bytes(), b'original fixture notices')
            self.assertEqual((output / 'lib/libonnxruntime.so.1').resolve(), output / 'lib/libonnxruntime.so.1.20.1')
            self.assertTrue((output / 'source/portico-sonic/third_party/portico-sonic/main.cpp').is_file())
            self.assertFalse((output / 'include').exists())

    def test_changed_helper_model_alias_and_unlisted_members_are_refused(self):
        for change in ('helper', 'model', 'alias', 'unlisted', 'target'):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as temporary:
                _, output = self.package(Path(temporary))
                if change == 'helper':
                    (output / 'portico-sonic').write_bytes(b'changed')
                elif change == 'model':
                    (output / 'musicnn-mtt/model.onnx').write_bytes(b'changed')
                elif change == 'alias':
                    alias = output / 'lib/libonnxruntime.so.1'
                    alias.unlink()
                    alias.symlink_to('/etc/passwd')
                elif change == 'unlisted':
                    (output / 'unlisted').write_bytes(b'extra')
                with self.assertRaises(ValueError):
                    bundle.verify(output, 'linux-arm64' if change == 'target' else 'linux-x64')

    def test_closed_descriptor_uses_exact_numeric_types_and_original_licence(self):
        for field, value in [('frames', True), ('bytes', 1.0), ('dimensions', 201), ('licence', 'NC'), ('dependencies', {})]:
            with self.subTest(field=field), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                _, _, model, _ = self.fixture(root)
                descriptor = json.loads((model / 'model.json').read_text())
                descriptor[field] = value
                (model / 'model.json').write_text(json.dumps(descriptor))
                with self.assertRaises(ValueError):
                    bundle.model_identity(model, bundle.locks()[1])

    def test_archive_escape_links_duplicates_and_special_files_never_publish(self):
        for kind in ('escape', 'link', 'duplicate', 'device'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                archive = root / 'hostile.tar.xz'
                with tarfile.open(archive, 'w:xz') as stream:
                    member = tarfile.TarInfo('../escape' if kind == 'escape' else 'entry')
                    member.size = 1
                    if kind == 'link':
                        member.type, member.linkname, member.size = tarfile.SYMTYPE, '/etc/passwd', 0
                    elif kind == 'device':
                        member.type, member.size = tarfile.CHRTYPE, 0
                    stream.addfile(member, io.BytesIO(b'x') if member.isfile() else None)
                    if kind == 'duplicate':
                        stream.addfile(member, io.BytesIO(b'x'))
                with self.assertRaises(ValueError):
                    bundle.extract(archive, root / 'out', 'linux-x64')
                self.assertFalse((root / 'out').exists())
                self.assertFalse((root.parent / 'escape').exists())

    def test_unknown_dependency_and_wrong_architecture_never_package(self):
        for kind in ('dependency', 'architecture'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                helper, sdk, model, _ = self.fixture(root)
                with patch.object(bundle, 'needed', return_value=['libunapproved.so']):
                    with self.assertRaises(ValueError):
                        bundle.package(helper, sdk, model, 'linux-arm64' if kind == 'architecture' else 'linux-x64', root / 'out.tar.xz', 'a' * 40)
                self.assertFalse((root / 'out.tar.xz').exists())

    def test_existing_output_and_changed_download_leave_original_untouched(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive, output = self.package(root)
            original = (output / 'portico-sonic').read_bytes()
            with self.assertRaises(ValueError):
                bundle.extract(archive, output, 'linux-x64')
            self.assertEqual((output / 'portico-sonic').read_bytes(), original)
            tag = bundle.locks()[0]['releaseTag']
            sums = ('0' * 64 + '  portico-sonic-' + tag + '-linux-x64.tar.xz\n').encode()
            with patch.object(bundle.urllib.request, 'urlopen', side_effect=[io.BytesIO(sums), io.BytesIO(b'changed archive')]):
                with self.assertRaises(ValueError):
                    bundle.fetch(root / 'new', 'linux-x64')
            self.assertFalse((root / 'new').exists())

    def test_installed_union_preserves_original_ffmpeg_guard_and_rejects_changes(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            _, output = self.package(root)
            ffmpeg = root / 'ffmpeg'
            (ffmpeg / 'bin').mkdir(parents=True)
            (ffmpeg / 'bin/ffmpeg').write_bytes(b'original FFmpeg fixture')
            original = {'target': 'linux-x64', 'files': {'bin/ffmpeg': bundle.digest(ffmpeg / 'bin/ffmpeg')}}
            (ffmpeg / 'toolchain-manifest.json').write_text(json.dumps(original))
            calls = []
            def original_verifier(command, **kwargs):
                self.assertEqual(command[:2], ['python3', str(bundle.ROOT / 'scripts/verify-ffmpeg-manifest.py')])
                verified = Path(command[2])
                paths = {p.relative_to(verified).as_posix() for p in verified.rglob('*') if p.is_file()}
                self.assertEqual(paths, {'bin/ffmpeg', 'toolchain-manifest.json'})
                self.assertEqual(json.loads((verified / 'toolchain-manifest.json').read_text()), original)
                if bundle.digest(verified / 'bin/ffmpeg') != original['files']['bin/ffmpeg']:
                    raise ValueError('original FFmpeg verifier refused changed bytes')
                calls.append(verified)
            with patch.object(bundle.subprocess, 'run', side_effect=original_verifier):
                bundle.install(output, ffmpeg, 'linux-x64')
                self.assertEqual(len(calls), 2)
                bundle.verify_installed(ffmpeg, 'linux-x64')
                (ffmpeg / 'bin/extra').write_bytes(b'undeclared')
                with self.assertRaises(ValueError):
                    bundle.verify_installed(ffmpeg, 'linux-x64')
                (ffmpeg / 'bin/extra').unlink()
                (ffmpeg / 'bin/ffmpeg').write_bytes(b'changed')
                with self.assertRaises(ValueError):
                    bundle.verify_installed(ffmpeg, 'linux-x64')

    def test_install_collision_is_refused_before_any_sonic_file_is_copied(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            _, output = self.package(root)
            ffmpeg = root / 'ffmpeg'
            (ffmpeg / 'bin').mkdir(parents=True)
            original = ffmpeg / 'bin/portico-sonic'
            original.write_bytes(b'keep existing')
            (ffmpeg / 'toolchain-manifest.json').write_text(json.dumps({'target': 'linux-x64'}))
            with patch.object(bundle.subprocess, 'run'):
                with self.assertRaises(ValueError):
                    bundle.install(output, ffmpeg, 'linux-x64')
            self.assertEqual(original.read_bytes(), b'keep existing')
            self.assertEqual([p.name for p in (ffmpeg / 'bin').iterdir()], ['portico-sonic'])

    def test_genuine_ffmpeg_verifier_accepts_composition_and_refuses_tamper_extra_and_symlinks(self):
        # This uses the real unchanged verifier, not the mocked boundary above.
        # Tiny ARM inventory bytes exercise that verifier's structural/hash
        # contract; they do not claim real media or native ARM conformance.
        for change in ('none', 'tamper', 'extra', 'member-link', 'parent-link'):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                _, output = self.package(root, 'linux-arm64')
                ffmpeg = root / 'ffmpeg'
                ffmpeg.mkdir()
                names = ['bin/ffmpeg', 'bin/ffprobe', 'bin/portico-disc', 'corresponding-source.tar.xz',
                         'NOTICE.md', 'sources.lock.json', 'requirements.v1.json', 'portico-dovi-profile.v1.json',
                         'portico-disc-build.json', 'LICENSES/portico-disc.txt']
                for name in names:
                    file = ffmpeg / name
                    file.parent.mkdir(parents=True, exist_ok=True)
                    file.write_bytes(b'synthetic structural inventory fixture: ' + name.encode())
                manifest = {'schemaVersion': 1, 'licenseMode': 'gpl3', 'target': 'linux-arm64',
                            'files': {name: bundle.digest(ffmpeg / name) for name in names}}
                (ffmpeg / 'toolchain-manifest.json').write_text(json.dumps(manifest))
                bundle.install(output, ffmpeg, 'linux-arm64')
                if change == 'none':
                    bundle.verify_installed(ffmpeg, 'linux-arm64')
                    continue
                if change == 'tamper':
                    (ffmpeg / 'bin/ffmpeg').write_bytes(b'changed')
                elif change == 'extra':
                    (ffmpeg / 'extra').write_bytes(b'unlisted')
                elif change == 'member-link':
                    file = ffmpeg / 'NOTICE.md'
                    outside = root / 'same-notice'
                    file.rename(outside)
                    file.symlink_to(outside)
                elif change == 'parent-link':
                    directory = ffmpeg / 'LICENSES'
                    outside = root / 'same-licences'
                    directory.rename(outside)
                    directory.symlink_to(outside, target_is_directory=True)
                with self.assertRaises((ValueError, bundle.subprocess.CalledProcessError)):
                    bundle.verify_installed(ffmpeg, 'linux-arm64')


if __name__ == '__main__':
    unittest.main()
