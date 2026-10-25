#!/usr/bin/env python3
"""Build one published native main with the exact immutable Reader producer."""
import argparse,datetime,fcntl,hashlib,importlib.util,json,os,signal,subprocess,tarfile,tempfile,zipfile
from pathlib import Path,PurePosixPath
ROOT=Path('/home/justin/projects/Portico');LANE=ROOT/'.worktrees/ui-2026/v3-release';PROGRAM=ROOT/'.worktrees/ui-2026/_program'
REPORT=None;RUN_OUT=None
class Unavailable(Exception):pass
def run(argv,log,cwd=None,env=None,timeout=1800):
    with log.open('ab') as output:
        p=subprocess.Popen(argv,cwd=cwd,env=env,stdout=output,stderr=subprocess.STDOUT,start_new_session=True)
        try:code=p.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            os.killpg(p.pid,signal.SIGTERM)
            try:p.wait(timeout=10)
            except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
            raise RuntimeError('bounded-build-timeout')
        finally:
            if p.poll() is None:
                os.killpg(p.pid,signal.SIGTERM)
                try:p.wait(timeout=8)
                except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
    if code:raise RuntimeError('build-command-failed-'+str(code))
def guest(source,private,out):
    destination=ROOT/'.artifacts/v3-release'/('reader-'+source[:12]);receipt=destination/'source-receipt.json'
    if receipt.exists():
        d=json.loads(receipt.read_text())
        if d.get('serverMain')==source and all((destination/'guest'/name).is_file() and hashlib.sha256((destination/'guest'/name).read_bytes()).hexdigest()==digest for name,digest in d['files'].items()):return destination/'guest'
    stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dt%H%M%Sz').lower();rid='native-reader-'+stamp;output=out/'reader-build';verified=all(subprocess.run(['git','-C',str(LANE/'server'),'merge-base','--is-ancestor',pin,source],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0 for pin in ['153c4a79088a00a15d5673db301d6a6872f2c7fc','7d3d088ac255828347b062598786664a0431d7ca'])
    code="""import runpy,os,types,sys
m=runpy.run_path(os.environ['PORTICO_RUNNER_SECONDARY_SOURCE']+'/deployment/testing/checkpoints/runner.py');a=types.SimpleNamespace(server=sys.argv[1],private=sys.argv[2],surface='web-server',run_id=sys.argv[3],reader_source_verified=sys.argv[4]=='1');j=m['Job'](a)
try:
 n=j.node();j.run('npm',['npm','ci','--no-audit','--no-fund']);j.reader(n);j.report['flows'].append({'surface':'native-reader','id':'immutable-producer','status':'pass'})
except m['StepFailure'] as e:j.report['flows'].append({'surface':'native-reader','id':e.step,'status':'fail'})
finally:j.finish()
raise SystemExit(int(any(f['status']=='fail' for f in j.report['flows'])))
"""
    args=['python3',str(PROGRAM/'runner-tools/run-remote.py'),'--host','netcup','--repo',str(LANE/'server'),'--commit',source,'--secondary-repo',str(LANE/'internal'),'--secondary-commit',private,'--lane',rid,'--output',str(output),'--','python3','-c',code,source,private,rid,str(int(verified))]
    run(args,out/'reader-coordinator.log',timeout=1200)
    spec=importlib.util.spec_from_file_location('coordinator',Path(__file__).with_name('run.py'));module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    if not module.receive('portico-runner-g12','/home/runner/work/artifacts/portico-v3-checkpoints/'+rid,out/'reader-output'):raise RuntimeError('reader-evidence-unavailable')
    data=json.loads((out/'reader-output/reader-guest-receipt.log').read_text());assert data['serverMain']==source and data['containsRequiredReaderSource']
    destination.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='guest-',dir=destination) as tmp:
        stage=Path(tmp)
        with tarfile.open(out/'reader-output/reader-guest.tar') as archive:
            members=archive.getmembers()
            if len(members)!=len(data['files']) or sum(m.size for m in members)>64*1024*1024:raise RuntimeError('reader-budget')
            for member in members:
                path=PurePosixPath(member.name)
                if not member.isfile() or path.is_absolute() or '..' in path.parts or member.name not in data['files']:raise RuntimeError('reader-member')
                raw=archive.extractfile(member).read()
                if hashlib.sha256(raw).hexdigest()!=data['files'][member.name]:raise RuntimeError('reader-hash')
                file=stage/member.name;file.parent.mkdir(parents=True,exist_ok=True);file.write_bytes(raw)
        if (destination/'guest').exists():raise RuntimeError('existing-reader-differs')
        stage.rename(destination/'guest')
    receipt.write_text(json.dumps(data,indent=2)+'\n');return destination/'guest'
def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--config',required=True,type=Path);a=p.parse_args();os.umask(0o077)
    if a.config.resolve()!=ROOT/'.private/v3-checkpoints/config.json':p.error('fixed private config required')
    global REPORT,RUN_OUT
    config=json.loads(a.config.read_text());native=LANE/'native';out=Path(config['evidence'])/('native-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dt%H%M%Sz').lower());out.mkdir(parents=True,exist_ok=False);report={'flows':[]};REPORT=report;RUN_OUT=out
    with (ROOT/'.private/v3-checkpoints/native-build.lock').open('a') as lock:
        try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError:raise Unavailable('native-lane-busy')
        if subprocess.check_output(['git','-C',str(native),'status','--porcelain'],text=True).strip():raise Unavailable('native-source-dirty')
        for repo in [native,LANE/'server',LANE/'internal']:run(['git','-C',str(repo),'fetch','origin','--quiet'],out/'prepare.log',timeout=60)
        if subprocess.check_output(['git','-C',str(native),'rev-list','origin/main..HEAD'],text=True).strip():raise Unavailable('native-unpublished-work')
        run(['git','-C',str(native),'rebase','origin/main'],out/'prepare.log',timeout=60)
        pin=subprocess.check_output(['git','-C',str(native),'rev-parse','HEAD'],text=True).strip();source=json.loads((native/'packages/portico-server.lock.json').read_text())['revision'];private=subprocess.check_output(['git','-C',str(LANE/'internal'),'rev-parse','origin/main'],text=True).strip();report.update(nativeMain=pin,serverMirror=source,privateTools=private)
        reader=guest(source,private,out)
        node=Path('/home/justin/.npm/_npx/538786c08bcb9442/node_modules/node/bin/node');version=(native/'.node-version').read_text().strip()
        if not node.is_file() or subprocess.check_output([str(node),'--version'],text=True).strip()!='v'+version:raise Unavailable('pinned-node-unavailable')
        env=dict(os.environ,PATH=str(node.parent)+os.pathsep+os.environ['PATH'],PORTICO_PACKAGING_LOCKED='1');artifact=ROOT/'.artifacts/v3-release'/('native-'+pin[:12]);artifact.mkdir(exist_ok=True)
        with Path('/tmp/portico-gradle.lock').open('a') as gradle:
            try:fcntl.flock(gradle,fcntl.LOCK_EX|fcntl.LOCK_NB)
            except BlockingIOError:raise Unavailable('gradle-busy')
            run(['npm','ci','--no-audit','--no-fund','--legacy-peer-deps'],out/'dependencies.log',native/'apps/native',env,timeout=240)
            packages={}
            for flavour,variant in [('google','googleDebug'),('firetv','firetvRelease')]:
                destination=artifact/flavour;destination.mkdir(exist_ok=True);log=out/('package-'+flavour+'.log')
                try:
                    run([str(node),str(native/'apps/native/scripts/package-android-emulator.mjs'),'--variant',variant,'--abi','x86_64','--reader-bundle',str(reader),'--libmpv-dir',str(ROOT/'.artifacts/v3-release/engine-r2/android'),'--output',str(destination),'--timeout','1800'],log,native,env,timeout=1900)
                    summary=json.loads(log.read_text().splitlines()[-1]);build=native/'apps/native/android/app/build';bundle=build/'generated/assets/react'/variant/'index.android.bundle';mapping=build/'generated/sourcemaps/react'/variant/'index.android.bundle.map'
                    with zipfile.ZipFile(summary['apk']) as archive:embedded=archive.read('assets/index.android.bundle')
                    if not mapping.is_file() or bundle.read_bytes()!=embedded:raise RuntimeError('matching-source-map-unavailable')
                    (destination/'index.android.bundle').write_bytes(embedded);(destination/'index.android.bundle.map').write_bytes(mapping.read_bytes())
                    proof={'nativeMain':pin,'variant':variant,'bundleSHA256':hashlib.sha256(embedded).hexdigest(),'mapSHA256':hashlib.sha256(mapping.read_bytes()).hexdigest(),'apkSHA256':summary['sha256']};(destination/'source-map-receipt.json').write_text(json.dumps(proof,indent=2)+'\n')
                    if flavour=='firetv':
                        spec=importlib.util.spec_from_file_location('fire_artifact',Path(__file__).with_name('firetv-artifact.py'));module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);proof=module.audit(Path(summary['apk']),native/'apps/native/licences/android-firetv.txt');(destination/'firetv-platform-proof.json').write_text(json.dumps(proof,indent=2)+'\n')
                        report['flows'].append({'surface':'fire-os-tv','id':'no-gms-cast-artifact','status':proof['status']})
                    packages[flavour]=summary;report['flows'].append({'surface':'android-google' if flavour=='google' else 'fire-os-tv','id':'published-main-package','status':'pass','variant':variant})
                except RuntimeError as error:report['flows'].append({'surface':'android-google' if flavour=='google' else 'fire-os-tv','id':'published-main-package','status':'fail','class':type(error).__name__,'variant':variant})
        report.update(artifact=str(artifact),reader=str(reader),packages=packages)
        if 'google' in packages:report['package']=packages['google']
        (out/'result.json').write_text(json.dumps(report,indent=2)+'\n');(ROOT/'.private/v3-checkpoints/native-candidate.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'status':'BUILT' if packages else 'FAILED','nativeMain':pin,'serverMirror':source,'artifact':str(artifact),'reader':str(reader),'flavours':list(packages)}))

if __name__=='__main__':
    def interrupt(*_):raise SystemExit(143)
    signal.signal(signal.SIGTERM,interrupt)
    try:main()
    except BaseException as e:
        status='unavailable' if isinstance(e,(Unavailable,SystemExit,KeyboardInterrupt)) else 'fail'
        receipt={'surface':'android','id':'published-main-package','status':status,'class':type(e).__name__}
        if isinstance(e,Unavailable):receipt['reason']=str(e)
        if REPORT is not None:
            REPORT['flows'].append(receipt);(RUN_OUT/'result.json').write_text(json.dumps(REPORT,indent=2)+'\n')
        print(json.dumps(receipt));raise SystemExit(75 if status=='unavailable' else 1)

