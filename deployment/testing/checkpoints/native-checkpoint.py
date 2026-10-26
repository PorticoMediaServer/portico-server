#!/usr/bin/env python3
"""Machine native checkpoint: build main, then take only idle allocated AVDs in turn."""
import argparse,contextlib,datetime,fcntl,hashlib,importlib.util,json,os,re,signal,subprocess,time,tempfile,zipfile
from pathlib import Path
ROOT=Path('/home/justin/projects/Portico');PRIVATE=ROOT/'.private/v3-checkpoints';NAMES={'phone':'Portico_Phone_API36','tv':'Portico_TV_API36'}
import sys
sys.path.insert(0,str(Path(__file__).resolve().parent))
import device_runtime
PACKAGE='tv.getportico.portico'
def adb(serial,*args,timeout=15):return subprocess.run(['adb','-s',serial,*args],capture_output=True,text=True,timeout=timeout)
def devices():
 text=subprocess.check_output(['adb','devices'],text=True,timeout=10);return [row.split()[0] for row in text.splitlines()[1:] if row.strip()]
def avd(serial):
 if not re.fullmatch('emulator-[0-9]+',serial):return ''
 try:return adb(serial,'emu','avd','name').stdout.splitlines()[0]
 except (IndexError,subprocess.SubprocessError):return ''
def installed_proof(serial,apk,private):
 paths=adb(serial,'shell','pm','path','tv.getportico.portico').stdout.strip().splitlines()
 if len(paths)!=1 or not re.fullmatch(r'package:/data/app/[A-Za-z0-9._~+=/-]+/base.apk',paths[0]) or '..' in paths[0].split('/'):raise RuntimeError('installed-package-path-refused')
 expected=hashlib.sha256(apk.read_bytes()).hexdigest()
 with tempfile.TemporaryFile(dir=private) as copy:
  result=subprocess.run(['adb','-s',serial,'exec-out','cat',paths[0][8:]],stdout=copy,stderr=subprocess.DEVNULL,timeout=30)
  if result.returncode or copy.tell()!=apk.stat().st_size:raise RuntimeError('installed-package-readback-refused')
  copy.seek(0);actual=hashlib.file_digest(copy,'sha256').hexdigest();copy.seek(0)
  with zipfile.ZipFile(copy) as archive:
   entry=archive.getinfo('assets/index.android.bundle')
   if entry.file_size>64*1024*1024:raise RuntimeError('installed-bundle-budget')
   hbc=hashlib.sha256(archive.read(entry)).hexdigest()
 return {'expectedAPK':expected,'installedAPK':actual,'installedHBC':hbc,'matchesCandidate':actual==expected,'scope':'this exact boot only; persisted base/snapshot/overlay state after shutdown is not claimed'}
@contextlib.contextmanager
def allocation(name):
 with Path('/tmp/portico-avd-'+name+'.lock').open('a') as held:
  try:fcntl.flock(held,fcntl.LOCK_EX|fcntl.LOCK_NB)
  except BlockingIOError:
   yield False;return
  yield True

def admission_refusal(surfaces):
    '''The production device guard: rows when no admitted lease exists, else None.'''
    try:device_runtime.require_lease()
    except device_runtime.RuntimeUnavailable:return device_runtime.unavailable_rows(surfaces)
    return None
def dropped_from_state(statefile):
 '''Dropped ids recorded by earlier runs; read only, never rewritten here.'''
 try:state=json.loads(Path(statefile).read_text())
 except (OSError,ValueError):return []
 dropped=state.get('dropped') if isinstance(state,dict) else None
 return list(dropped) if isinstance(dropped,list) else []
def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--config',type=Path,required=True);parser.add_argument('--skip-build',action='store_true',help='qualify a previously built exact candidate');a=parser.parse_args();os.umask(0o077)
 if a.config.resolve()!=PRIVATE/'config.json':parser.error('fixed private campaign config required')
 config=json.loads(a.config.read_text());out=Path(config['evidence'])/('android-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dt%H%M%Sz').lower());out.mkdir(parents=True,exist_ok=False);rows=[]
 with (PRIVATE/'android.lock').open('a') as lock:
  try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
  except BlockingIOError:print('Native checkpoint already active; untouched.');return 0
  if not a.skip_build:
   with (out/'build-coordinator.log').open('wb') as log:code=subprocess.run(['python3',str(Path(__file__).with_name('native-build.py')),'--config',str(a.config)],stdout=log,stderr=subprocess.STDOUT,timeout=5400).returncode
   if code:
    rows=[{'surface':name,'id':'core-flows','status':'unavailable','dependency':'native build failed or busy'} for name in ['android-phone','android-tv','fire-os-tv']];(out/'result.json').write_text(json.dumps({'flows':rows},indent=2)+'\n');return 0
  candidate=json.loads((PRIVATE/'native-candidate.json').read_text());packages=candidate.get('packages',{'google':candidate.get('package')})
  targetfile=PRIVATE/'android-targets.json'
  if not targetfile.exists():
   rows=[{'surface':name,'id':'core-flows','status':'unavailable','dependency':'Release target qualification pending'} for name in ['android-phone','android-tv','fire-os-tv']];(out/'result.json').write_text(json.dumps({'flows':rows},indent=2)+'\n');return 0
  if targetfile.is_symlink() or targetfile.stat().st_mode&0o077:raise RuntimeError('private-target-file-required')
  targets=json.loads(targetfile.read_text())
  # The machine device phase is not implemented: no admitted lease producer exists, and
  # none can be supplied by target data. The build above stands on its own; every device
  # surface records unavailable here, before any probe, boot, clock, reverse, install,
  # launch or adoption.
  surfaces=['android-phone','android-tv','fire-os-tv']
  rows=admission_refusal(surfaces)
  (out/'result.json').write_text(json.dumps({'nativeMain':candidate['nativeMain'],'serverMirror':candidate['serverMirror'],'apkSHA256s':{name:value['sha256'] for name,value in packages.items() if value},'flows':rows,'dropped':dropped_from_state(PRIVATE/'native-state.json'),'cleanup':'no device action; nothing booted or adopted','handbackArtifactScope':'packages built only; machine runtime qualification awaits an admitted lease producer'},indent=2)+'\n')
  print(json.dumps({'nativeMain':candidate['nativeMain'],'flows':rows}));return 0
if __name__=='__main__':
 def stop(*_):raise SystemExit(143)
 signal.signal(signal.SIGTERM,stop);signal.signal(signal.SIGHUP,stop)
 raise SystemExit(main())
