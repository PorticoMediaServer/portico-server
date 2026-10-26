"""Artifact capability receipt/reuse checks; no build, runtime or network."""
import importlib.util
import json
from pathlib import Path
import runpy
import sys
import tempfile
import unittest

ROOT = Path(__file__).parent
spec = importlib.util.spec_from_file_location('walks_server', ROOT/'__init__.py', submodule_search_locations=[str(ROOT)])
module = importlib.util.module_from_spec(spec); sys.modules['walks_server'] = module; spec.loader.exec_module(module)
from walks_server import build
coordinator = runpy.run_path(str(ROOT.parent/'server.py'))

class Capabilities(unittest.TestCase):
    def test_rollback_loads_old_but_current_reuse_requires_tags(self):
        with tempfile.TemporaryDirectory(prefix='.walk-build-tags-', dir=Path.home()) as tmp:
            root=Path(tmp);candidate=root/'candidate';candidate.mkdir()
            (candidate/'portico-server').write_bytes(b'synthetic artifact')
            web=candidate/'web';web.mkdir();(web/'index.html').write_text('synthetic')
            doc={'commit':'a'*40,'binarySHA256':build._file_hash(candidate/'portico-server'),
                 'webSHA256':build._tree_hash(web),'schemaDigest':'b'*64,'officialRootSHA256':'c'*64}
            manifest=candidate/'manifest.json';manifest.write_text(json.dumps(doc))
            self.assertEqual(build.load(candidate).build_tags, ())
            paths=type('Paths',(),{'builds':root})()
            self.assertIsNone(coordinator['_existing_build'](paths,'a'*40))
            doc['buildTags']=['release','portico_cb7'];manifest.write_text(json.dumps(doc))
            self.assertEqual(build.load(candidate).build_tags, ('release','portico_cb7'))
            self.assertIsNone(coordinator['_existing_build'](paths,'a'*40))
            self.assertEqual(build.BUILD_TAGS, ('release','portico_cb7','portico_rar7z'))
            doc['buildTags']=list(build.BUILD_TAGS);manifest.write_text(json.dumps(doc))
            self.assertEqual(coordinator['_existing_build'](paths,'a'*40).build_tags,build.BUILD_TAGS)
            for wrong in ('release', ['release'], ['release','unknown'], ['release','portico_rar7z'], [1,2], [['release']]):
                doc['buildTags']=wrong;manifest.write_text(json.dumps(doc))
                with self.assertRaises(build.BuildError): build.load(candidate)

if __name__=='__main__':unittest.main()
