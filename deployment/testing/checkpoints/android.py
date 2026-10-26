#!/usr/bin/env python3
"""Bounded native M0 flows, reusing v2 Home/media/focus helpers with private JSON custody.

Runs only against an exact already allocated device opened by Release. Never
clears data, adopts another AVD, uses XML credentials, or records raw auth output.
"""
import argparse,contextlib,hashlib,importlib.util,json,os,re,signal,subprocess,time
from pathlib import Path
ROOT=Path('/home/justin/projects/Portico');PACKAGE='tv.getportico.portico';ALLOWED={'phone':'Portico_Phone_API36','tv':'Portico_TV_API36'}
class Refusal(Exception):pass
def load(name,path):
 spec=importlib.util.spec_from_file_location(name,path);module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module
class Device:
 def __init__(self,target,mode,out,private,server):
  self.target=target;self.mode=mode;self.out=out;self.private=private;self.server=server;self.sequence=0;self.frames=[];self.custody=load('custody',Path(__file__).parent.parent/'visual-qa/auth-driver/private_driver.py');self.credentials=self.custody.reload_credentials(ROOT/'.private/demo-testing/active-credentials.json');self.serial=target['targetArgs'][target['targetArgs'].index('--serial')+1]
  if not re.fullmatch('emulator-[0-9]{4,5}',self.serial):raise Refusal('non-emulator-target')
  name=subprocess.check_output(['adb','-s',self.serial,'emu','avd','name'],text=True,timeout=10).splitlines()[0]
  if name!=ALLOWED[mode]:raise Refusal('wrong-allocated-avd')
 def command(self,args):
  self.sequence+=1;code,out,err=self.custody.private_capture([self.target['command'],*args,*self.target['targetArgs']],self.private,'command-'+str(self.sequence),timeout=30)
  if code:
   try:error=json.loads(out.read_text()).get('error',{})
   except (ValueError,OSError):error={}
   if args[0]=='snapshot' and error.get('code')=='COMMAND_FAILED' and error.get('message')=='Android snapshot helper returned only non-application windows':raise Refusal('authoritative-tree-unavailable')
   raise Refusal('device-command-refused')
  return out
 def snapshot(self):
  d=json.loads(self.command(['snapshot','-i','--json']).read_text());data=d.get('data',{})
  if d.get('success') is not True or not any(n.get('bundleId')==PACKAGE for n in data.get('nodes',[])) or any(n.get('bundleId') not in (None,'',PACKAGE) for n in data.get('nodes',[])) or data.get('truncated') is not False or not data.get('nodes'):raise Refusal('authoritative-tree-unavailable')
  return data
 def nodes(self):
  deadline=time.monotonic()+20
  while True:
   try:return self.snapshot()['nodes']
   except Refusal as error:
    if str(error)!='authoritative-tree-unavailable' or time.monotonic()>=deadline:raise
    time.sleep(1)
 def text(self):
  nodes=self.nodes()
  if any(x.get('password') is True for x in nodes):raise Refusal('credential-screen')
  return '\n'.join(str(x.get('label','')) for x in nodes if x.get('editable') is not True)
 def key(self,key):subprocess.run(['adb','-s',self.serial,'shell','input','keyevent',key],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=10)
 def tap(self,pattern):
  nodes=self.nodes();candidates=[x for x in nodes if re.fullmatch(pattern,str(x.get('label',''))) and x.get('hittable') is True and x.get('enabled') is not False and (x.get('type','').endswith('Button') or str(x.get('identifier','')).startswith('phone-tab-'))]
  if len(candidates)!=1:raise Refusal('unique-visible-control-unavailable')
  ref=candidates[0].get('ref','')
  if not re.fullmatch('e[0-9]+',ref):raise Refusal('invalid-control-reference')
  self.command(['click','@'+ref]);return True
 def screenshot(self,name):
  nodes=self.nodes();values=[self.credentials['username'],self.credentials['password']]
  if any(n.get('password') is True or n.get('editable') is True or any(v and v in str(n.get('label','')) for v in values) for n in nodes):raise Refusal('private-screenshot-withheld')
  path=self.out/('flow-'+name+'.png')
  with path.open('xb') as output:subprocess.run(['adb','-s',self.serial,'exec-out','screencap','-p'],stdout=output,stderr=subprocess.DEVNULL,check=True,timeout=15)
  if not path.read_bytes().startswith(b'\x89PNG\r\n\x1a\n'):raise Refusal('invalid-screenshot')
  return str(path)
 def wait_text(self,pattern,timeout=20):
  until=time.monotonic()+timeout
  while time.monotonic()<until:
   try:text=self.text()
   except Refusal as error:
    if str(error)!='authoritative-tree-unavailable':raise
    time.sleep(1);continue
   if re.search(pattern,text,re.I):return True
   time.sleep(1)
  raise Refusal('expected-ui-timeout')
def safe_diagnostics(raw):
 origins={'home-card','home-assembly','assembly-error','restore-error','route-probe','route-memory','home-registration','artwork-error','native-http','app-render','library-menu'};operations={'report-error','saved-route-summary','bindings','attached','screen','producer','produced','element','dispatch','response','rejected','primary-play','menu'};classes={'Error','TypeError','ReferenceError','RangeError','SyntaxError','AbortError','ApiError','PlaybackApiError','RouteError','ContractError','RequestTimeoutError','unknown'};codes={'other','unoffered_home_playback','native_home_authority_required','stale_home_context','home-visible-registration-limit','invalid_response','invalid_home','invalid_scope','invalid_request','request_failed','network_error','network_unavailable','offline','timeout','cancelled','unauthorized','forbidden','access_refused','identity_mismatch','route_unavailable','certificate_unverified','certificate_untrusted','identity_code_needed','invalid_identity','invalid_signature','invalid_address','clocks_disagree','route_stale','random_unavailable','not_found','unavailable','unsupported','unsupported_media','no_playable_version','capabilities_required','revision_required','stale_revision','conflict','session_expired','lease_expired','stale_selection_context','invalid_playback_target','artwork_pending','rate_limited','server_busy'}
 rows=[]
 for line in raw.splitlines():
  if '[home-play-r25] ' not in line:continue
  try:d=json.loads(line.split('[home-play-r25] ',1)[1])
  except ValueError:continue
  if d.get('origin') not in origins or d.get('operation') not in operations:continue
  row={k:d[k] for k in ['origin','operation']}
  for k in ['v','sequence','request','status','elapsedMs','runtimeRef','menuRef','pairedCount']:
   if isinstance(d.get(k),int) and not isinstance(d[k],bool) and 0<=d[k]<=1000000:row[k]=d[k]
  for k in ['own','enumerable','callable','signedCurrent','signedPrevious','lastGood']:
   if isinstance(d.get(k),bool):row[k]=d[k]
  for k,values in [('code',codes),('class',classes),('routeSource',{'none','local','account'}),('stage',{'enter','guard-refused','unoffered','dispatch'})]:
   if d.get(k) in values:row[k]=d[k]
  route=d.get('route')
  if route in {'get_home','get_home_row','get_me','get_capabilities','prove_network_identity','get_playback_options','put_device_capabilities','get_listening_preferences','create_queue','get_queue','get_queue_window','list_queue_entries','prepare_queue_next','commit_queue_next','advance_queue','start_playback_session','get_playback_session','change_playback_session','stop_playback_session'}:row['route']=route
  for k in ['frames','componentFrames']:
   if isinstance(d.get(k),list):row[k]=[p for p in d[k][:6] if isinstance(p,list) and len(p)==2 and all(isinstance(x,int) and not isinstance(x,bool) and 0<=x<=10**9 for x in p)]
  rows.append(row)
  if len(rows)>=256:break
 return rows
def admission_refusal(surface):
 '''The production driver guard: closed unavailable rows while no lease producer exists.'''
 refusal=load('device_runtime',Path(__file__).with_name('device_runtime.py'))
 return refusal.unavailable_rows([surface])
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--target',type=Path,required=True);p.add_argument('--mode',choices=ALLOWED,required=True);p.add_argument('--out',type=Path,required=True);p.add_argument('--flavour',choices=['google','firetv'],default='google');p.add_argument('--disabled',type=json.loads,default=[]);a=p.parse_args();os.umask(0o077)
 if a.target.is_symlink() or a.target.stat().st_mode&0o077 or a.target.stat().st_uid!=os.getuid():p.error('private owned target required')
 surface='fire-os-tv' if a.flavour=='firetv' else 'android-'+a.mode
 if a.flavour=='firetv' and a.mode!='tv':p.error('Fire OS requires TV allocation')
 tv=load('tv_flows',Path(__file__).with_name('tv_flows.py')) if a.mode=='tv' else None
 phone=load('phone_flows',Path(__file__).with_name('phone_flows.py')) if a.mode=='phone' else None
 out=a.out;out.mkdir(parents=True,exist_ok=False);private=ROOT/'.private/v3-checkpoints'/('flow-'+out.parent.name+'-'+out.name);private.mkdir(mode=0o700,exist_ok=False);server=ROOT/'.worktrees/ui-2026/v3-release/server';legacy=load('legacy_android_flows',server/'smoke/android/flows.py');target=json.loads(a.target.read_text())
 rows=admission_refusal(surface)
 if rows:
  # No admitted lease producer exists, so a standalone driver refuses before its device
  # probe, log capture, launch or force-stop. A target file is a description, never
  # admission; a syntactically complete old target is refused the same way.
  (out/'result.json').write_text(json.dumps({'flows':rows,'coverage':'refused before any device action'},indent=2)+'\n');print(json.dumps({'surface':surface,'flows':rows}));return 0
 d=Device(target,a.mode,out,private,server);flows=[];log=(private/'native-logcat.log').open('wb');capture=subprocess.Popen(['adb','-s',d.serial,'logcat','-v','brief','-T','1','ReactNativeJS:W','AndroidRuntime:E','*:S'],stdout=log,stderr=subprocess.STDOUT)
 failures=(Refusal,tv.Failure) if tv else (Refusal,phone.Refusal)
 unavailable_errors=(tv.Unavailable,) if tv else (phone.Unavailable,)
 def flow(name,fn):
  record={'surface':surface,'id':name}
  if record['surface']+':'+name in a.disabled:record.update(status='dropped',manual='Justin hands-on; flaky twice');flows.append(record);return False
  started=time.monotonic()
  try:
   value=fn();record['status']='pass'
   if isinstance(value,dict):record['evidence']=value
  except unavailable_errors as e:record.update(status='unavailable',reason=str(e),dependency='authoritative device observation unavailable')
  except failures as e:record.update(status='fail',reason=str(e),timeout=str(e)=='expected-ui-timeout' or str(e).endswith('-timeout'))
  except Exception as e:record.update({'status':'fail','class':type(e).__name__})
  record['durationMs']=round((time.monotonic()-started)*1000)
  try:record['screenshot']=d.screenshot(name)
  except Exception:record['screenshotWithheld']=True
  flows.append(record);return record['status']=='pass'
 flow.unavailable=lambda name,reason,dependency:flows.append({'surface':surface,'id':name,'status':'unavailable','reason':reason,'dependency':dependency})
 try:
  if a.mode=='tv':tv.run_tv(d,flow,legacy,a.flavour)
  else:
   def launch():
    # A hard stop at the mutation site: there is no adb bypass and no launch without an
    # admitted lease. Behaviour below is unreachable until a real lease producer exists.
    raise Refusal('runtime-admission-unavailable')
   flow('launch-restored-family',launch)
   if not any(row['id']=='launch-restored-family' and row['status']=='pass' for row in flows):
    flow.unavailable('remaining-phone-flows','retained-launch-unavailable','no reset or re-auth fallback')
   else:
    phone.run_phone(d,flow,legacy,launched=True,refusal=Refusal,unavailable=lambda name,code:flow.unavailable(name,code,'authoritative observation or prerequisite unavailable'))
 finally:
  with contextlib.suppress(Exception):d.key('KEYCODE_MEDIA_STOP')
  with contextlib.suppress(Exception):subprocess.run(['adb','-s',d.serial,'shell','am','force-stop',PACKAGE],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=10)
  capture.terminate()
  try:capture.wait(timeout=5)
  except subprocess.TimeoutExpired:capture.kill();capture.wait()
  log.close();(out/'native-receipts.json').write_text(json.dumps(safe_diagnostics((private/'native-logcat.log').read_text(errors='replace')),indent=2)+'\n');(out/'result.json').write_text(json.dumps({'flows':flows,'authState':'retained; never cleared','cleanup':'media Stop requested; app and log capture stopped','coverage':'bounded restored-family core subset; full sign-in/seek/subtitles/Reading/search/profile acceptance remains separate'},indent=2)+'\n')
 print(json.dumps({'surface':surface,'flows':[{k:v for k,v in row.items() if k!='screenshot'} for row in flows]}));return int(any(f['status']=='fail' for f in flows))
if __name__=='__main__':
 def stop(*_):raise SystemExit(143)
 signal.signal(signal.SIGTERM,stop);signal.signal(signal.SIGHUP,stop)
 raise SystemExit(main())
