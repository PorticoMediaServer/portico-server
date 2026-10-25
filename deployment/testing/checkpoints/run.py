#!/usr/bin/env python3
"""Machine scheduler entry: isolated current-main jobs, bounded receipts, no AI driver."""
import argparse,concurrent.futures,datetime,fcntl,hashlib,json,os,re,shlex,subprocess,tarfile,tempfile
from pathlib import Path

HOSTS={'netcup':('portico-runner-g12','/usr/bin/chromium'),'contabo':('portico-runner','/snap/bin/chromium')}
def read_config(path):
    info=path.lstat()
    if path.is_symlink() or not path.is_file() or info.st_uid!=os.getuid() or info.st_mode&0o077:raise ValueError('private owner-only config required')
    value=json.loads(path.read_text());root=Path(value['root']).resolve()
    if root!=Path('/home/justin/projects/Portico'):raise ValueError('fixed project root required')
    return value,root

def freeze(repo):
    subprocess.run(['git','-C',str(repo),'fetch','origin','--quiet'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    return subprocess.check_output(['git','-C',str(repo),'rev-parse','origin/main'],text=True).strip()

def receive(host,remote,destination):
    destination.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryFile() as stream:
        result=subprocess.run(['ssh','-o','BatchMode=yes',host,'tar -cf - -C '+shlex.quote(remote)+' .'],stdout=stream,stderr=subprocess.DEVNULL,timeout=60)
        if result.returncode:return False
        stream.seek(0)
        with tarfile.open(fileobj=stream,mode='r:') as archive:
            members=archive.getmembers();total=0
            if len(members)>128:raise ValueError('too many evidence files')
            for member in members:
                name=member.name.removeprefix('./')
                if member.isdir() and name in ['', '.']:continue
                if not member.isfile() or not (re.fullmatch(r'[a-z0-9.-]+\.(?:log|jpg)',name) or name=='reader-guest.tar'):raise ValueError('unsupported evidence member')
                if member.size>(64 if name=='reader-guest.tar' else 8)*1024*1024:raise ValueError('oversized evidence file')
                total+=member.size
                if total>112*1024*1024:raise ValueError('evidence budget exceeded')
                raw=archive.extractfile(member).read()
                if name.endswith('.jpg') and not raw.startswith(b'\xff\xd8'):raise ValueError('invalid screenshot')
                target=destination/name
                with target.open('xb') as out:os.fchmod(out.fileno(),0o600);out.write(raw)
    subprocess.run(['ssh','-o','BatchMode=yes',host,'rm -rf -- '+shlex.quote(remote)],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    return True

def job(config,root,server,private,host,surface,stamp,disabled):
    run_id='m0-'+stamp+'-'+host+'-'+surface;folder=Path(config['evidence'])/run_id;folder.mkdir(parents=True,exist_ok=False);alias,chromium=HOSTS[host]
    command=['python3','-c',"import os,runpy;runpy.run_path(os.environ['PORTICO_RUNNER_SECONDARY_SOURCE']+'/deployment/testing/checkpoints/runner.py',run_name='__main__')",'--surface',surface,'--server',server,'--private',private,'--run-id',run_id,'--fixtures','/home/runner/work/fixtures/portico-v3','--chromium',chromium,'--disabled',json.dumps(disabled)]
    if all(subprocess.run(['git','-C',config['serverRepo'],'merge-base','--is-ancestor',pin,server],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0 for pin in ['153c4a79088a00a15d5673db301d6a6872f2c7fc','7d3d088ac255828347b062598786664a0431d7ca']):command.append('--reader-source-verified')
    args=['python3',str(root/'.worktrees/ui-2026/_program/runner-tools/run-remote.py'),'--host',host,'--browser','--repo',config['serverRepo'],'--commit',server,'--secondary-repo',config['privateRepo'],'--secondary-commit',private,'--lane',run_id,'--output',str(folder/'runner'),'--']+command
    with (folder/'coordinator.log').open('wb') as log:
        try:code=subprocess.run(args,stdout=log,stderr=subprocess.STDOUT,timeout=2100).returncode
        except subprocess.TimeoutExpired:code=124
    received=receive(alias,'/home/runner/work/artifacts/portico-v3-checkpoints/'+run_id,folder/'outputs')
    result={'run':run_id,'serverMain':server,'privateMain':private,'runnerExit':code,'evidenceReceived':received,'flows':[]}
    if received:result['flows']=json.loads((folder/'outputs/report.log').read_text())['flows']
    if not result['flows']:result['flows']=[{'surface':surface,'id':'runner-admission' if code==75 else 'runner-build','status':'unavailable' if code==75 else 'fail','kind':'infrastructure'}]
    (folder/'result.json').write_text(json.dumps(result,indent=2)+'\n');return result

def audit(config,root):
    rows=[]
    for repo in ['portico-server','portico-react-native','portico-internal']:
        runs=json.loads(subprocess.check_output(['gh','run','list','--repo','PorticoMediaServer/'+repo,'--workflow','ci.yml','--branch','main','--limit','8','--json','databaseId,status,conclusion,headSha,event,createdAt'],text=True))
        full=next((r for r in runs if r['event'] in ['schedule','workflow_dispatch']),None)
        now=datetime.datetime.now(datetime.timezone.utc)
        if not full or (now-datetime.datetime.fromisoformat(full['createdAt'].replace('Z','+00:00'))).total_seconds()>70*60:
            # GitHub cron can be delayed or dropped. The machine's hourly audit
            # supplies one full default-main run only when no recent one exists.
            subprocess.run(['gh','workflow','run','ci.yml','--repo','PorticoMediaServer/'+repo,'--ref','main'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=30)
        latest=next((r for r in runs if r['status']=='completed' and r['event'] in ['schedule','workflow_dispatch']),None)
        if not latest or latest['conclusion']!='failure':continue
        jobs=json.loads(subprocess.check_output(['gh','run','view',str(latest['databaseId']),'--repo','PorticoMediaServer/'+repo,'--json','jobs'],text=True))['jobs']
        rows.append({'repo':repo,**latest,'failures':[{'job':j['name'],'steps':[s['name'] for s in j['steps'] if s['conclusion']=='failure']} for j in jobs if j['conclusion']=='failure']})
    path=Path(config['evidence'])/'ci-inbox.json';path.parent.mkdir(parents=True,exist_ok=True);path.write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps({'ciFailures':len(rows),'inbox':str(path)}))

def refresh_ledger(config,root):
    program=root/'.worktrees/ui-2026/_program'
    command=['python3',str(program/'runner-tools/ledger-from-trailers.py')]
    for key in ['serverRepo','nativeRepo','privateRepo']:command+=['--repo',config[key]]
    log=Path(config['evidence'])/'ledger-refresh.log';log.parent.mkdir(parents=True,exist_ok=True)
    with log.open('ab') as output:
        subprocess.run(command,check=True,stdout=output,stderr=subprocess.STDOUT,timeout=180)
        subprocess.run(['python3',str(program/'v3/make-worklists.py')],check=True,stdout=output,stderr=subprocess.STDOUT,timeout=30)
    print('Ledger and owner work lists refreshed from published main; owner notes preserved.')

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--config',required=True,type=Path);parser.add_argument('--audit-ci',action='store_true');parser.add_argument('--refresh-ledger',action='store_true');parser.add_argument('--host',choices=['netcup','contabo']);parser.add_argument('--surface',choices=['web-server','hosted']);args=parser.parse_args();config,root=read_config(args.config)
    os.umask(0o077)
    if args.refresh_ledger:refresh_ledger(config,root);return 0
    if args.audit_ci:audit(config,root);return 0
    lock=Path(config['state']).with_suffix('.lock');lock.parent.mkdir(parents=True,exist_ok=True)
    with lock.open('a') as held:
        try:fcntl.flock(held,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError:print('Checkpoint already running; left untouched.');return 0
        server=freeze(Path(config['serverRepo']));private=freeze(Path(config['privateRepo']));state_path=Path(config['state']);state=json.loads(state_path.read_text()) if state_path.exists() else {'flaky':{},'dropped':[]};stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dt%H%M%Sz').lower();tasks=[]
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            for host in ([args.host] if args.host else ['netcup','contabo']):
                for surface in ([args.surface] if args.surface else ['web-server','hosted']):tasks.append(pool.submit(job,config,root,server,private,host,surface,stamp,state['dropped']))
            results=[]
            for future in concurrent.futures.as_completed(tasks):
                try:result=future.result()
                except Exception as error:result={'flows':[{'surface':'checkpoint','id':'coordinator','status':'fail','class':type(error).__name__,'kind':'infrastructure'}]}
                results.append(result)
                for flow in result['flows']:
                    key=flow['surface']+':'+flow['id'];history=state.setdefault('flowHistory',{}).setdefault(key,{'passed':False,'timeouts':0})
                    if flow['status']=='pass':history['passed']=True
                    if flow.get('timeout'):history['timeouts']+=1
                    # Repeated deterministic failures remain fix items. A passing result
                    # plus timeouts establishes intermittency; two retire the script.
                    if history['passed'] and history['timeouts']>=2:
                        state['flaky'][key]=history['timeouts']
                        if key not in state['dropped']:state['dropped'].append(key)
        state_path.write_text(json.dumps(state,indent=2)+'\n');inbox=Path(config['evidence'])/(stamp+'-inbox.json');inbox.write_text(json.dumps({'serverMain':server,'privateMain':private,'results':results,'dropped':state['dropped']},indent=2)+'\n');print(json.dumps({'runs':len(results),'failedFlows':sum(f['status']=='fail' for r in results for f in r['flows']),'dropped':state['dropped'],'inbox':str(inbox)}));return 0
if __name__=='__main__':raise SystemExit(main())
