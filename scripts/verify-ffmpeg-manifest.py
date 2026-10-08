#!/usr/bin/env python3
import hashlib,json,pathlib,sys
root=pathlib.Path(sys.argv[1]).resolve();m=json.loads((root/'toolchain-manifest.json').read_text())
if m.get('schemaVersion')!=1 or m.get('licenseMode')!='gpl3':raise SystemExit('Invalid manifest')
files=m['files']
actual={str(p.relative_to(root)) for p in root.rglob('*') if p.is_file() and p.name!='toolchain-manifest.json'}
if actual!=set(files):raise SystemExit('Bundle file inventory mismatch')
def digest_file(path):
 with path.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
for required in ['corresponding-source.tar.xz','NOTICE.md','sources.lock.json','requirements.v1.json']:
 if required not in files:raise SystemExit('Missing provenance: '+required)
if not any(x.startswith('LICENSES/') for x in files):raise SystemExit('Missing licenses')
for name,digest in files.items():
 path=root/name
 if not path.resolve().is_relative_to(root) or path.is_symlink() or not path.is_file() or digest_file(path)!=digest:raise SystemExit('Bundle verification failed: '+name)
for tool in ['ffmpeg','ffprobe']:
 name='bin/'+tool+('.exe' if m['target'].startswith('windows-') else '')
 if name not in files:raise SystemExit('Missing executable: '+name)
print('Verified bundle hashes for '+m['target'])
