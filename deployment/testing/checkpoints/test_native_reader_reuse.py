"""Synthetic immutable Reader reuse tests; no builder, network or live cache."""
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('native_reader_build',Path(__file__).with_name('native-build.py'))
build=importlib.util.module_from_spec(spec);spec.loader.exec_module(build)
SOURCE='a'*40

class ReaderReuse(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory(dir=Path.home(),prefix='.reader-reuse-')
        self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name)
        self.destination=self.root/'.artifacts/v3-release'/('reader-'+SOURCE[:12])
        self.tree=self.destination/'guest';self.tree.mkdir(parents=True)
        (self.tree/'index.html').write_bytes(b'original immutable guest')
        self.files={'index.html':hashlib.sha256((self.tree/'index.html').read_bytes()).hexdigest()}
        self.data={'serverMain':SOURCE,'containsRequiredReaderSource':True,'files':self.files}
        self.legacy=self.destination/'source-receipt.json'
        self.legacy.write_text(json.dumps({'source':SOURCE,'files':[],'treeDigest':'legacy'}))
        self.before=self.legacy.read_bytes()

    def test_legacy_bytes_are_preserved_and_next_guest_reuses_without_builder(self):
        self.assertEqual(build.remember_reader(self.destination,SOURCE,self.data),self.tree)
        self.assertEqual(self.legacy.read_bytes(),self.before)
        with patch.object(build,'ROOT',self.root),patch.object(build.subprocess,'run',side_effect=AssertionError('no job')):
            self.assertEqual(build.guest(SOURCE,'private',self.root/'unused'),self.tree)
        self.assertEqual(self.legacy.read_bytes(),self.before)

    def test_changed_missing_extra_or_symlink_guest_never_gets_admitted(self):
        for mode in ('changed','missing','extra','symlink'):
            with self.subTest(mode=mode):
                p=self.tree/'index.html'
                if p.exists() or p.is_symlink():p.unlink()
                extra=self.tree/'extra'
                if extra.exists():extra.unlink()
                p.write_bytes(b'original immutable guest')
                if mode=='changed':p.write_bytes(b'changed')
                if mode=='missing':p.unlink()
                if mode=='extra':extra.write_bytes(b'extra')
                if mode=='symlink':p.unlink();p.symlink_to(self.legacy)
                with self.assertRaises(RuntimeError):build.remember_reader(self.destination,SOURCE,self.data)
                self.assertFalse((self.destination/'producer-receipt.json').exists())
                self.assertEqual(self.legacy.read_bytes(),self.before)

    def test_wrong_source_false_proof_empty_or_unsafe_inventory_refuses(self):
        for update in ({'serverMain':'b'*40},{'containsRequiredReaderSource':False},{'files':{}},
                       {'files':{'../outside':'a'*64}},{'files':{'index.html':'not-a-digest'}}):
            data=dict(self.data,**update)
            with self.subTest(update=update),self.assertRaises(RuntimeError):build.remember_reader(self.destination,SOURCE,data)
        self.assertEqual(self.legacy.read_bytes(),self.before)

    def test_oversize_refuses_before_hashing(self):
        with (self.tree/'index.html').open('wb') as stream:stream.truncate(build.READER_LIMIT+1)
        with self.assertRaisesRegex(RuntimeError,'reader-budget'):build.remember_reader(self.destination,SOURCE,self.data)

    def test_new_cache_keeps_standard_source_receipt(self):
        self.legacy.unlink()
        build.remember_reader(self.destination,SOURCE,self.data)
        self.assertEqual(json.loads(self.legacy.read_text()),self.data)
        self.assertFalse((self.destination/'producer-receipt.json').exists())

if __name__=='__main__':unittest.main()
