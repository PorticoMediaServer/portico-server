#!/usr/bin/env python3
"""Machine native checkpoint: build main, then take only idle allocated AVDs in turn."""
import argparse,contextlib,datetime,fcntl,hashlib,importlib.util,json,os,re,signal,subprocess,time
from pathlib import Path
ROOT=Path('/home/justin/projects/Portico');PRIVATE=ROOT/'.private/v3-checkpoints';NAMES={'phone':'Portico_Phone_API36','tv':'Portico_TV_API36'}
def adb(serial,*args,timeout=15):return subprocess.run(['adb','-s',serial,*args],capture_output=True,text=True,timeout=timeout)
def devices():
 text=subprocess.check_output(['adb','devices'],text=True,timeout=10);return [row.split()[0] for row in text.splitlines()[1:] if row.strip()]
def avd(serial):
 if not re.fullmatch('emulator-[0-9]+',serial):return ''
 try:return adb(serial,'emu','avd','name').stdout.splitlines()[0]
 except (IndexError,subprocess.SubprocessError):return ''
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
  targets=json.loads(targetfile.read_text());statefile=PRIVATE/'native-state.json';state=json.loads(statefile.read_text()) if statefile.exists() else {'history':{},'dropped':[]}
  for mode,flavour in [('phone','google'),('tv','google'),('tv','firetv')]:
   name=NAMES[mode];surface='fire-os-tv' if flavour=='firetv' else 'android-'+mode;package=packages.get(flavour)
   if not package:rows.append({'surface':surface,'id':'core-flows','status':'unavailable','dependency':'flavour build failed or not in frozen candidate'});continue
   apk=Path(package['apk'])
   if not apk.is_file() or hashlib.sha256(apk.read_bytes()).hexdigest()!=package['sha256']:raise RuntimeError('candidate-artifact-hash')
   if mode not in targets:rows.append({'surface':surface,'id':'core-flows','status':'unavailable','dependency':'device target unqualified'});continue
   target=targets[mode];flags=target['targetArgs'];serial=flags[flags.index('--serial')+1];attached=devices()
   if serial in attached or len(attached)>=2 or any(avd(d)==name for d in attached):rows.append({'surface':surface,'id':'core-flows','status':'unavailable','dependency':'allocated device or emulator capacity busy'});continue
   if not re.fullmatch('emulator-[0-9]{4,5}',serial):raise RuntimeError('invalid-allocated-serial')
   port=int(serial.split('-')[1]);emulator=Path.home()/'Android/Sdk/emulator/emulator';private=PRIVATE/(out.name+'-'+mode+'-'+flavour);private.mkdir(mode=0o700);targetpath=private/'target.json';targetpath.write_text(json.dumps(target));log=(private/'emulator.log').open('wb');process=None
   try:
    command=[str(emulator),'-avd',name,'-port',str(port),'-no-window','-no-audio','-no-boot-anim','-no-snapshot','-gpu','swiftshader_indirect','-memory','2048','-cores','2'];process=subprocess.Popen(command,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
    (PRIVATE/'android-custody.json').write_text(json.dumps({'run':out.name,'mode':mode,'flavour':flavour,'avd':name,'pid':process.pid,'state':'checkpoint','maxSeconds':600})+'\n')
    until=time.monotonic()+180
    while time.monotonic()<until:
     if process.poll() is not None:raise RuntimeError('owned-emulator-exited')
     if serial in devices() and avd(serial)==name and adb(serial,'shell','getprop','sys.boot_completed').stdout.strip()=='1':break
     time.sleep(2)
    else:raise RuntimeError('emulator-boot-deadline')
    adb(serial,'shell','input','keyevent','KEYCODE_WAKEUP')
    adb(serial,'shell','wm','dismiss-keyguard')
    installed=adb(serial,'install','-r','-d',str(apk),timeout=180)
    if installed.returncode or 'Success' not in installed.stdout:raise RuntimeError('candidate-install-failed')
    flowout=out/(mode if flavour=='google' else 'firetv')
    with (private/'flow-driver.log').open('wb') as logs:
     driver=subprocess.Popen(['python3',str(Path(__file__).with_name('android.py')),'--target',str(targetpath),'--mode',mode,'--flavour',flavour,'--out',str(flowout),'--disabled',json.dumps(state['dropped'])],stdout=logs,stderr=subprocess.STDOUT,start_new_session=True)
     try:driver.wait(timeout=600)
     finally:
      if driver.poll() is None:
       with contextlib.suppress(ProcessLookupError):os.killpg(driver.pid,signal.SIGTERM)
       try:driver.wait(timeout=15)
       except subprocess.TimeoutExpired:
        with contextlib.suppress(ProcessLookupError):os.killpg(driver.pid,signal.SIGKILL)
        driver.wait()
    result=json.loads((flowout/'result.json').read_text());rows+=result['flows']
    for row in result['flows']:
     key=row['surface']+':'+row['id'];history=state['history'].setdefault(key,{'passed':False,'timeouts':0})
     if row['status']=='pass':history['passed']=True
     if row.get('timeout'):history['timeouts']+=1
     if history['passed'] and history['timeouts']>=2 and key not in state['dropped']:state['dropped'].append(key)
   except Exception as error:rows.append({'surface':surface,'id':'core-flows','status':'fail','class':type(error).__name__})
   finally:
    # Only the exact AVD booted by this process is ours. Never clear userdata,
    # auth/protected records, snapshots, another emulator, or CertTest.
    if process is not None:
     if process.poll() is None and avd(serial)==name:
      with contextlib.suppress(Exception):adb(serial,'emu','kill')
     try:process.wait(timeout=12)
     except subprocess.TimeoutExpired:
      with contextlib.suppress(ProcessLookupError):os.killpg(process.pid,signal.SIGTERM)
      try:process.wait(timeout=5)
      except subprocess.TimeoutExpired:
       with contextlib.suppress(ProcessLookupError):os.killpg(process.pid,signal.SIGKILL)
       process.wait()
     until=time.monotonic()+15
     while serial in devices() and time.monotonic()<until:time.sleep(1)
    log.close();(PRIVATE/'android-custody.json').write_text(json.dumps({'run':out.name,'mode':mode,'flavour':flavour,'avd':name,'state':'returned','authState':'preserved'})+'\n')
  statefile.write_text(json.dumps(state,indent=2)+'\n');(out/'result.json').write_text(json.dumps({'nativeMain':candidate['nativeMain'],'serverMirror':candidate['serverMirror'],'apkSHA256s':{name:value['sha256'] for name,value in packages.items() if value},'flows':rows,'dropped':state['dropped'],'cleanup':'all own AVD processes stopped; originals/auth/CertTest preserved'},indent=2)+'\n');print(json.dumps({'nativeMain':candidate['nativeMain'],'flows':rows,'dropped':state['dropped']}));return 0
if __name__=='__main__':
 def stop(*_):raise SystemExit(143)
 signal.signal(signal.SIGTERM,stop);signal.signal(signal.SIGHUP,stop)
 raise SystemExit(main())
