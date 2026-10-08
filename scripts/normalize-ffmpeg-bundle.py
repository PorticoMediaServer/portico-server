#!/usr/bin/env python3
"""Seal an exact build and corresponding source; never manufacture qualification."""
import argparse,hashlib,json,pathlib,shutil
root=pathlib.Path(__file__).resolve().parent.parent
p=argparse.ArgumentParser();p.add_argument('input',type=pathlib.Path);p.add_argument('target');p.add_argument('output',type=pathlib.Path);p.add_argument('source',type=pathlib.Path);a=p.parse_args()
if a.target not in ['linux-x64','linux-arm64','windows-x64','windows-arm64','macos-x64','macos-arm64']:p.error('unknown target')
if a.output.exists():p.error('output must not exist')
if not a.source.is_file() or a.source.stat().st_size<1024:p.error('corresponding source archive is required')
a.output.mkdir(parents=True);(a.output/'bin').mkdir();(a.output/'LICENSES').mkdir()
for tool in ['ffmpeg','ffprobe']:
 name=tool+('.exe' if a.target.startswith('windows-') else '')
 matches=sorted(x for x in a.input.rglob(name) if x.is_file() and not x.is_symlink())
 if len(matches)!=1:p.error('expected one '+name+' executable')
 shutil.copy2(matches[0],a.output/'bin'/name)
for i,f in enumerate(sorted(a.input.rglob('*'))):
 if f.is_file() and f.name.lower().startswith(('license','copying')):shutil.copy2(f,a.output/'LICENSES'/(str(i)+'-'+f.name))
if not list((a.output/'LICENSES').iterdir()):p.error('dependency license texts missing')
shutil.copy2(a.source,a.output/'corresponding-source.tar.xz')
for name in ['NOTICE.md','sources.lock.json','requirements.v1.json']:shutil.copy2(root/'third_party/ffmpeg'/name,a.output/name)
def digest(path):
 with path.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
files={str(f.relative_to(a.output)):digest(f) for f in sorted(a.output.rglob('*')) if f.is_file()}
lock=json.loads((a.output/'sources.lock.json').read_text())
(a.output/'toolchain-manifest.json').write_text(json.dumps({'schemaVersion':1,'target':a.target,'ffmpegVersion':lock['ffmpeg']['version'],'buildId':lock['releaseTag'],'licenseMode':'gpl3','files':files},indent=2)+'\n')
