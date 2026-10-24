#!/usr/bin/env python3
"""The narrow compiler cache and protocol gate must fail closed."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


def load(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


cache = load('reuse-ffmpeg-protocol-objects')
protocol = load('verify-ffmpeg-bluray')


class CacheBoundaryTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.base = Path(temporary.name)
        self.old = self.base / 'old'
        self.fresh = self.base / 'fresh'
        for path in (self.old / 'source/libavformat', self.old / 'source/libavcodec',
                     self.old / 'prefix/bin', self.fresh / 'libavformat', self.fresh / 'libavcodec'):
            path.mkdir(parents=True)
        post = [{'path': 'profile'}, {'path': 'join'},
                {'path': 'third_party/ffmpeg/portico-bluray-playlist-minimum.patch'}]
        self.lock = {'ffmpeg': {'commit': 'source', 'sha256': 'archive', 'patches': [],
                                'postSeriesPatches': post}, 'linuxSourceBuild': {'compilerImage': 'compiler'}}
        pair = {}
        for name in ('ffmpeg', 'ffprobe'):
            path = self.old / 'prefix/bin' / name
            path.write_text(name)
            pair[name] = cache.digest(path)
        self.pair = pair
        (self.old / 'prefix/compiler-linked-inputs.json').write_text(json.dumps({'compilerImage': 'compiler'}))
        for root, contents in ((self.old / 'source', 'original'), (self.fresh, 'patched')):
            (root / 'libavformat/bluray.c').write_text(contents)
            (root / 'libavcodec/decode.c').write_text('unchanged')
            (root / 'config.h').write_text('same configure')
            receipt = {'sourceCommit': 'source', 'sourceSha256': 'archive', 'patches': [],
                       'postSeriesPatches': post[:2] if root != self.fresh else post,
                       'files': {name: cache.digest(root / name) for name in
                                 ('libavformat/bluray.c', 'libavcodec/decode.c')}}
            (root / 'portico-source-receipt.json').write_text(json.dumps(receipt))
        for name in ('libavformat/bluray.o', 'libavformat/libavformat.a', 'libavcodec/decode.o'):
            (self.old / 'source' / name).write_text('compiled')
        self.generated = patch.object(cache, 'GENERATED', ('config.h',))
        self.generated.start()
        self.addCleanup(self.generated.stop)
        self.expected = cache.fingerprint(self.old, self.lock, self.pair)

    def reuse(self):
        cache.reuse(self.old, self.fresh, self.lock, self.expected, self.pair)

    def test_only_unchanged_objects_reused_and_original_preserved(self):
        self.reuse()
        self.assertTrue((self.fresh / 'libavcodec/decode.o').is_file())
        self.assertFalse((self.fresh / 'libavformat/bluray.o').exists())
        self.assertFalse((self.fresh / 'libavformat/libavformat.a').exists())
        self.assertEqual(cache.fingerprint(self.old, self.lock, self.pair), self.expected)

    def test_replaced_object_refused(self):
        (self.old / 'source/libavcodec/decode.o').write_text('changed')
        with self.assertRaisesRegex(ValueError, 'changed before reuse'):
            self.reuse()

    def test_different_configure_refused(self):
        (self.fresh / 'config.h').write_text('different configuration')
        with self.assertRaisesRegex(ValueError, 'configure/generated inputs differ'):
            self.reuse()

    def test_second_source_change_refused(self):
        path = self.fresh / 'portico-source-receipt.json'
        receipt = json.loads(path.read_text())
        receipt['files']['libavcodec/decode.c'] = 'changed too'
        path.write_text(json.dumps(receipt))
        with self.assertRaisesRegex(ValueError, 'Only the checked Blu-ray'):
            self.reuse()


class ProtocolGateTests(unittest.TestCase):
    def test_stock_help_cannot_qualify(self):
        with patch.object(protocol, 'run_tool', return_value=(0, 'playlist angle chapter', '')):
            with self.assertRaisesRegex(ValueError, 'lacks min_playlist_length'):
                protocol.qualify(Path('ffmpeg'), {})

    def test_default_must_reject_actual_short_title(self):
        help_text = '-min_playlist_length <int> (from 0 to INT_MAX) (default 180)'
        with patch.object(protocol, 'run_tool', side_effect=[(0, help_text, ''), (0, '', '')]):
            with self.assertRaisesRegex(ValueError, 'Default180 changed'):
                protocol.qualify(Path('ffmpeg'), {protocol.IDS[0]: Path('disc')})

    def test_option_help_without_decoded_frame_cannot_qualify(self):
        help_text = '-min_playlist_length <int> (from 0 to INT_MAX) (default 180)'
        with patch.object(protocol, 'run_tool', side_effect=[(0, help_text, ''),
                (1, '', '0 usable playlists'), (0, '# no frames', '')]):
            with self.assertRaisesRegex(ValueError, 'did not decode'):
                protocol.qualify(Path('ffmpeg'), {protocol.IDS[0]: Path('disc')})


if __name__ == '__main__':
    unittest.main()
