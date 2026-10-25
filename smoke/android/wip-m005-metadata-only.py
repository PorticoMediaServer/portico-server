# PAUSED WIP: preserved source only. Requires a new explicit Release metadata-only window.
# Never load credentials or submit sign-in; this source has not been executed.
import subprocess,json,re,time,pathlib,os
os.umask(0o077)
p=pathlib.Path('/home/justin/projects/Portico/.artifacts/ds-m0-smoke/m005-metadata-only');b=['/home/justin/.t3/userdata/device/bin/agent-device'];f=['--platform','android','--serial','emulator-5554','--config','/home/justin/.t3/userdata/device/hosts/25bf8e1a2393f1108d37029b.json','--session','t3-1caf96ba9643646bf6eb2948'];n=0
# No credential files, username/password values, screenshots or submit action.
def call(*args):
 global n
 n+=1;r=subprocess.run(b+list(args)+f,capture_output=True,text=True,timeout=25);(p/f'raw-{n:02d}.log').write_text(r.stdout+'\n'+r.stderr)
 if r.returncode:raise RuntimeError('supported CLI operation failed')
 return r.stdout

def ref(pattern):
 m=re.search(pattern,call('snapshot','-i'))
 if not m:raise RuntimeError('public UI target absent')
 return m[1]
call('click',ref(r'(@e\d+) \[button\] "Use a local server"'))
call('fill',ref(r'(@e\d+) \[text-field\] "Server address"'),'https://demo.getportico.tv')
call('click',ref(r'(@e\d+) \[button\] "Continue"(?! \[disabled\])'))
time.sleep(3)
s=json.loads(call('snapshot','--json'));(p/'empty-form.json').write_text(json.dumps(s))
d=s.get('data',{});nodes=d.get('nodes',[])
secure=[x for x in nodes if x.get('editable') is True and x.get('password') is True and x.get('bundleId')=='tv.getportico.portico']
summary={'credentialFileLoaded':False,'credentialTyped':False,'submitted':False,'schemaKeys':sorted(set().union(*(x.keys() for x in nodes))-{'label','value','placeholder','contentDescription','identifier'}),'secureCandidateCount':len(secure)}
if len(secure)!=1:(p/'safe-report.json').write_text(json.dumps(summary));raise RuntimeError('native secure candidate missing or ambiguous; stop')
call('click','@'+secure[0]['ref'])
s=json.loads(call('snapshot','--json'));(p/'focused-secure.json').write_text(json.dumps(s))
d=s.get('data',{});focused=[x for x in d.get('nodes',[]) if x.get('focused') is True and x.get('editable') is True]
summary.update({'refsGeneration':d.get('refsGeneration'),'truncated':d.get('truncated'),'focusedEditableCount':len(focused),'nativeFacts':[{k:x.get(k) for k in ['password','focused','editable','enabled','visibleToUser','hittable','interactionBlocked']} for x in focused]})
(p/'safe-report.json').write_text(json.dumps(summary,indent=2));print('Metadata capture complete; reduced report saved')
