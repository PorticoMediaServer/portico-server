#!/usr/bin/env python3
import hashlib,json,pathlib,re,sys
root=pathlib.Path(sys.argv[1]).resolve();m=json.loads((root/'toolchain-manifest.json').read_text())
if m.get('schemaVersion')!=1 or m.get('licenseMode')!='gpl3':raise SystemExit('Invalid manifest')
files=m['files']
actual={str(p.relative_to(root)) for p in root.rglob('*') if p.is_file() and p.name!='toolchain-manifest.json'}
if actual!=set(files):raise SystemExit('Bundle file inventory mismatch')
def digest_file(path):
 with path.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
for required in ['corresponding-source.tar.xz','NOTICE.md','sources.lock.json','requirements.v1.json','portico-disc-build.json','LICENSES/portico-disc.txt']:
 if required not in files:raise SystemExit('Missing provenance: '+required)
if m['target']=='linux-x64':
 for required in ['dependency-inputs.json','compiler.txt','compiler-linked-inputs.json','license-inventory.json','linux-build-inputs.json','repeat-build.json']:
  if required not in files:raise SystemExit('Missing Linux build provenance: '+required)
if not any(x.startswith('LICENSES/') for x in files):raise SystemExit('Missing licenses')
for name,digest in files.items():
 path=root/name
 if not path.resolve().is_relative_to(root) or path.is_symlink() or not path.is_file() or digest_file(path)!=digest:raise SystemExit('Bundle verification failed: '+name)
for tool in ['ffmpeg','ffprobe','portico-disc']:
 name='bin/'+tool+('.exe' if m['target'].startswith('windows-') else '')
 if name not in files:raise SystemExit('Missing executable: '+name)
if m['target']=='linux-x64':
 rounds=json.loads((root/'repeat-build.json').read_text())
 inputs=json.loads((root/'linux-build-inputs.json').read_text())
 expected={tool:files['bin/'+tool] for tool in ['ffmpeg','ffprobe','portico-disc']}
 if (len(rounds)!=2 or [r.get('round') for r in rounds]!=[1,2] or
     not all(r.get('binaries')==expected and r.get('sourceArchiveSHA256')==inputs.get('sourceArchiveSHA256') for r in rounds)):
  raise SystemExit('Repeated executable builds do not match this bundle')
 if not all(isinstance(inputs.get(key),str) and re.fullmatch(r'[0-9a-f]{64}',inputs[key]) for key in ['sourceArchiveSHA256','dependencyPrefixFilesSHA256']):
  raise SystemExit('Missing sealed Linux source/dependency identity')
 if not re.fullmatch(r'[0-9a-f]{64}',str(inputs.get('nativeBuildTools',{}).get('glslc',''))):
  raise SystemExit('Missing source-built native shader compiler identity')
print('Verified bundle hashes for '+m['target'])
