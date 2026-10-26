#!/usr/bin/env python3
"""One isolated, admitted runner job. Main source in; per-flow evidence out."""
from __future__ import annotations
import argparse,base64,contextlib,email,hashlib,http.server,importlib.util,json,os,re,secrets,shutil,signal,socket,ssl,subprocess,sys,tarfile,threading,time,traceback,urllib.error,urllib.request,uuid
from pathlib import Path

class StepFailure(Exception):
    def __init__(self,step):self.step=step

def port():
    with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]

def private_json(path,value):
    with path.open('x') as f:os.chmod(path,0o600);json.dump(value,f)


def _start_ticks(pid):
    try:return int(Path('/proc/%d/stat'%pid).read_text().rpartition(')')[2].split()[19])
    except (OSError,IndexError,ValueError):return None

def _children(pid):
    result=[]
    try:tasks=sorted(Path('/proc/%d/task'%pid).iterdir(),key=lambda p:int(p.name))[:256]
    except OSError:return result
    # Go may spawn its working child from a non-leader OS thread, so children are
    # read per task rather than per process.
    for task in tasks:
        try:values=(task/'children').read_text().split()
        except OSError:continue
        for value in values:
            if value.isdigit() and int(value) not in result:result.append(int(value))
    return result

def _descendants(pid,limit=64):
    result=[pid];pending=[pid]
    while pending and len(result)<limit:
        current=pending.pop(0)
        for child in _children(current):
            if child not in result:
                result.append(child);pending.append(child)
                if len(result)>=limit:break
    return result

def _listener_inodes(port):
    wanted='%04X'%port;inodes=set()
    for name in ['tcp','tcp6']:
        try:lines=Path('/proc/net/'+name).read_text().splitlines()[1:]
        except OSError:continue
        for line in lines:
            fields=line.split()
            if len(fields)<10 or fields[3]!='0A':continue
            local=fields[1]
            if not local.startswith('0100007F:') or local.split(':',1)[1].upper()!=wanted:continue
            if fields[9].isdigit() and fields[9]!='0':inodes.add(fields[9])
    return inodes

def _owns_socket(pid,inodes):
    if not inodes:return False
    try:
        with os.scandir('/proc/%d/fd'%pid) as entries:
            for index,entry in enumerate(entries):
                if index>=4096:break
                try:target=os.readlink(entry.path)
                except OSError:continue
                if target.startswith('socket:[') and target[8:-1] in inodes:return True
    except OSError:return False
    return False

_SMOKE_SETS=None
def api_smoke_sets():
    """The CLI's own closed endpoint-code and reason sets, read from the checkout.

    The sets live beside this file in the secondary checkout. If they cannot be
    read, no code or reason is published at all rather than accepting free text.
    """
    global _SMOKE_SETS
    if _SMOKE_SETS is None:
        codes,reasons=frozenset(),frozenset()
        try:
            directory=str(Path(__file__).resolve().parent)
            if directory not in sys.path:sys.path.insert(0,directory)
            from api_smoke.client import CLOSED_CODES
            from api_smoke.flows import REASONS
            codes,reasons=frozenset(CLOSED_CODES),frozenset(REASONS)
        except Exception:
            pass
        _SMOKE_SETS=(codes,reasons)
    return _SMOKE_SETS

# The closed set of operation ids the reviewed API smoke may publish.
API_SMOKE_FLOWS=frozenset('health-live health-ready home-real-rows library-real-content detail-real-item search-real-title reading-list reading-session-open reading-resource reading-progress-read reading-progress-save reading-progress-confirm reading-progress-stale-guard reading-session-close playback-options playback-queue-create playback-queue-read playback-start-admission playback-preparing playback-current playback-controls-read playback-stop playback-queue-cleanup settings-me settings-downloads settings-console settings-configuration cleanup-reading-session cleanup-playback-session cleanup-queue cleanup-owned-handles'.split())

class Job:
    def __init__(self,args):
        self.args=args;self.source=Path.cwd();self.tools=Path(os.environ['PORTICO_RUNNER_SECONDARY_SOURCE']);self.out=Path(os.environ['PORTICO_RUNNER_OUTPUT']);self.work=Path(os.environ['TMPDIR'])/'checkpoint';self.work.mkdir(mode=0o700);self.children=[];self.logs=[];self.mailboxes=[];self.process_env=dict(os.environ,GOMAXPROCS='2',GOMEMLIMIT='1400MiB');self.report={'serverMain':args.server,'privateMain':args.private,'surface':args.surface,'flows':[],'generated':[],'runtimeProfile':'isolated loopback QA; no public deployment or production-identity acceptance'}
    def run(self,name,argv,cwd=None,env=None,timeout=900):
        log=self.out/('build-'+name+'.log')
        with log.open('wb') as f:
            process=subprocess.Popen(argv,cwd=cwd or self.source,env=env or self.process_env,stdout=f,stderr=subprocess.STDOUT,start_new_session=True);self.children.append(process)
            try:code=process.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid,signal.SIGTERM)
                try:process.wait(timeout=8)
                except subprocess.TimeoutExpired:os.killpg(process.pid,signal.SIGKILL);process.wait()
                raise StepFailure(name+'-timeout')
        if code:raise StepFailure(name)
    def spawn(self,name,argv,cwd=None,env=None):
        f=(self.work/(name+'.private.log')).open('wb');self.logs.append(f);p=subprocess.Popen(argv,cwd=cwd or self.source,env=env or self.process_env,stdout=f,stderr=subprocess.STDOUT,start_new_session=True);self.children.append(p);return p
    def attempt(self,surface,name,action,kind='build'):
        try:action()
        except StepFailure as error:
            self.report['flows'].append({'surface':surface,'id':name,'status':'fail','kind':kind,'step':error.step});return False
        self.report['flows'].append({'surface':surface,'id':name,'status':'pass','kind':kind});return True
    def ready(self,url,process):
        for _ in range(160):
            if process.poll() is not None:
                log=self.work/'server-runtime.private.log';text=log.read_text(errors='replace') if log.is_file() else ''
                self.report['runtimeExit']={'exitCode':process.returncode,'categories':[code for token,code in [('permission denied','permission'),('Operation not permitted','namespace-denied'),('No such file','missing-file'),('read-only file system','read-only-path'),('/var/cache/portico','service-transcode-path'),('schema','schema'),('baseline','baseline'),('address already in use','address-busy')] if token.lower() in text.lower()]}
                raise StepFailure('runtime-start')
            try:
                with urllib.request.urlopen(url,timeout=1) as r:
                    if r.status==200:return
            except Exception:pass
            time.sleep(.25)
        raise StepFailure('runtime-readiness')
    def json_call(self,base,method,path,body=None,token=None,installation=None,headers=None,expected=(200,)):
        h={'Content-Type':'application/json','Accept':'application/json'}
        if token:h['Authorization']='Bearer '+token
        if installation:h['X-Portico-Installation-Id']=installation
        h.update(headers or {})
        req=urllib.request.Request(base+path,json.dumps(body).encode() if body is not None else None,h,method=method)
        try:r=urllib.request.urlopen(req,timeout=45)
        except urllib.error.HTTPError as e:r=e
        with r:
            raw=r.read(8*1024*1024+1)
            if r.status not in expected or len(raw)>8*1024*1024:raise StepFailure('api-'+method.lower()+'-'+str(r.status))
            return json.loads(raw) if raw else None
    def generation(self):
        roots=[(self.source,['packages/contracts/src','server/api/portico.openapi.json','server/internal/httpapi/route_lanes.go','server/internal/persistence/known_columns.go','server/internal/persistence/known_schema.go','server/internal/persistence/schema_test.go'])] if self.args.surface=='web-server' else [(self.tools,['hosted-services/hosted/api/hosted.openapi.json','hosted-services/hosted/contracts/src/hosted'])]
        before={}
        for root,names in roots:
            for name in names:
                p=root/name
                for f in p.rglob('*') if p.is_dir() else [p]:
                    if f.is_file():before[(root,str(f.relative_to(root)))]=f.read_bytes()
        if self.args.surface=='web-server':
            self.run('api-generation',['go','run','./cmd/apigen','-root','..'],self.source/'server')
            self.run('schema-generation',['python3','scripts/schema-sync.py'])
        else:self.run('hosted-generation',['go','run','./cmd/apigen','-root','../..'],self.tools/'hosted-services/hosted')
        patch=[]
        for (root,name),old in before.items():
            new=(root/name).read_bytes()
            if old!=new:
                self.report['generated'].append(name)
                previous=self.work/'generation-before';previous.write_bytes(old)
                result=subprocess.run(['diff','-u','--label','a/'+name,'--label','b/'+name,str(previous),str(root/name)],capture_output=True,text=True,timeout=20)
                if result.returncode not in [0,1]:raise StepFailure('generation-diff')
                patch.append(result.stdout)
        (self.out/'generation.patch.log').write_text(''.join(patch));self.report['generatedPatchSHA256']=hashlib.sha256(''.join(patch).encode()).hexdigest()
    def node(self):
        version=(self.source/'.node-version').read_text().strip();log=self.work/'node-path.private.log'
        with log.open('wb') as f:r=subprocess.run(['npm','exec','--yes','--package=node@'+version,'--','node','-p','process.execPath'],stdout=f,stderr=subprocess.DEVNULL,timeout=120)
        if r.returncode:raise StepFailure('node-toolchain')
        path=Path(log.read_text().strip());actual=subprocess.check_output([str(path),'--version'],text=True).strip()
        if actual!='v'+version:raise StepFailure('node-version')
        self.process_env['PATH']=str(path.parent)+os.pathsep+self.process_env['PATH'];self.process_env['NODE_OPTIONS']='--max-old-space-size=1536';return str(path)
    def browser(self,node,config):
        payload=self.work/'browser-input.json';private_json(payload,config)
        self.run('browser-deps',['npm','install','--prefix',str(self.work/'browser'),'--no-save','--package-lock=false','--no-audit','--no-fund','playwright@1.58.2'])
        env=dict(self.process_env,PORTICO_CHECKPOINT_INPUT=str(payload),PORTICO_CHECKPOINT_BROWSER=str(self.work/'browser/node_modules/playwright/index.mjs'),PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=self.args.chromium)
        try:self.run('browser',[node,str(self.tools/'deployment/testing/checkpoints/browser.mjs')],env=env,timeout=600)
        finally:
            if (self.out/'browser-results.log').exists():self.report['flows']+=json.loads((self.out/'browser-results.log').read_text())['flows']
    def reader(self,node):
        guest=self.work/'reader-guest'
        self.run('reader-guest',[node,str(self.source/'node_modules/vite/bin/vite.js'),'build','--config',str(self.source/'web-next/reader-native/vite.config.ts')],env=dict(self.process_env,PORTICO_READER_OUT_DIR=str(guest)))
        files={}
        for file in sorted(guest.rglob('*')):
            if file.is_symlink():raise StepFailure('reader-guest-symlink')
            if file.is_file():files[str(file.relative_to(guest))]=hashlib.sha256(file.read_bytes()).hexdigest()
        if not {'index.html','licences.json'}<=files.keys():raise StepFailure('reader-guest-incomplete')
        # Retain the complete producer graph, including any newly introduced dependency.
        source={}
        for directory in ['packages/reader-core','web-next/reader-native','web-next/src/screens/read/native']:
            for file in sorted((self.source/directory).rglob('*')):
                if file.is_file() and 'node_modules' not in file.parts:source[str(file.relative_to(self.source))]=hashlib.sha256(file.read_bytes()).hexdigest()
        receipt={'serverMain':self.args.server,'files':files,'sourceFiles':source,'treeSHA256':hashlib.sha256(''.join(f'{value}  ./{name}\n' for name,value in sorted(files.items())).encode()).hexdigest(),'containsRequiredReaderSource':self.args.reader_source_verified}
        (self.out/'reader-guest-receipt.log').write_text(json.dumps(receipt,indent=2)+'\n')
        with tarfile.open(self.out/'reader-guest.tar','w') as archive:
            for name in files:archive.add(guest/name,arcname=name,recursive=False)
        self.report['readerGuest']={'treeSHA256':receipt['treeSHA256'],'files':len(files),'containsRequiredReaderSource':receipt['containsRequiredReaderSource']}
    def server_build(self):
        pem=self.source/'trust/hosted-root-public-key.pem';raw=pem.read_bytes();body=''.join(x for x in raw.decode().splitlines() if not x.startswith('---'));der=base64.b64decode(body,validate=True)
        if der[:12]!=bytes.fromhex('302a300506032b6570032100') or len(der)!=44:raise StepFailure('official-root-format')
        key=base64.urlsafe_b64encode(der[-32:]).decode().rstrip('=')
        if key in (self.source/'server/internal/hostedtrust/development_root.go').read_text():raise StepFailure('development-root-refused')
        self.report['officialRootPEMSHA256']=hashlib.sha256(raw).hexdigest()
        flags='-X portico.local/server/internal/buildinfo.Version=0.1.0-checkpoint+'+self.args.server[:8]+' -X portico.local/server/internal/buildinfo.BuildID='+self.args.server+' -X portico.local/server/internal/hostedtrust.OfficialRoot='+key
        self.run('server',['go','build','-p=2','-tags','release','-trimpath','-buildvcs=false','-ldflags',flags,'-o',str(self.work/'server'),'./cmd/server'],self.source/'server')
        self.run('server-version',[str(self.work/'server'),'--version'],timeout=10)
    def original_runtime(self,bind,timeout=30):
        """The exact built server process behind the bwrap launcher.

        A candidate must be a bounded same-process descendant of the launcher, run
        the exact executable this job built, report real start ticks, and own the
        admitted loopback listener socket. A bare listener on the port is never
        adopted; more than one match is ambiguous and therefore unusable.
        """
        launcher=getattr(self,'app_pid',None);launcher_start=getattr(self,'app_start',None)
        # A pid alone can be reused: the captured launcher process and its start
        # ticks have to still be the same live process, before and after the walk.
        if launcher is None or launcher_start is None or getattr(self,'app',None) is None:return None
        expected=os.path.realpath(str(self.work/'server'));deadline=time.monotonic()+timeout
        def launcher_original():
            if self.app.poll() is not None:return False
            return _start_ticks(launcher)==launcher_start
        while time.monotonic()<deadline:
            if not launcher_original():return None
            inodes=_listener_inodes(bind)
            if inodes:
                matches=[]
                for candidate in _descendants(launcher):
                    try:exe=os.path.realpath(os.readlink('/proc/%d/exe'%candidate))
                    except OSError:continue
                    ticks=_start_ticks(candidate)
                    if exe!=expected or ticks is None:continue
                    if _owns_socket(candidate,inodes):matches.append((candidate,ticks))
                if len(matches)>1:return None
                if len(matches)==1:return matches[0] if launcher_original() else None
            time.sleep(.25)
        return None
    def api_smoke(self,base,bind,token,installation):
        """Run the reviewed API smoke against this exact disposable runtime.

        The context is private to this job and removed in the finally path; the
        setup grant is still alive so the call uses this run's own token. A missing
        or ambiguous runtime identity, or a CLI that never wrote its receipt,
        publishes only an unavailable API row: the browser follow-on, the setup
        grant retirement and the job result are all unaffected.
        """
        surface='server-api';context=self.work/'api-context.json';result=self.out/'flow-api-smoke.log'
        try:
            runtime=self.original_runtime(bind)
            if runtime is None:
                self.report['flows'].append({'surface':surface,'id':'runtime-identity','status':'unavailable','reason':'original-runtime-not-resolved'});return
            private_json(context,{'origin':base,'accessToken':token,'installationId':installation,'owner':True,'scope':'checkpoint-disposable','runId':self.args.run_id,'runtime':{'pid':runtime[0],'startTimeTicks':runtime[1]}})
            try:self.run('api-smoke',['python3',str(self.tools/'deployment/testing/checkpoints/api-smoke.py'),'--context',str(context),'--origin',base,'--run-id',self.args.run_id,'--output',str(result)],timeout=420)
            except StepFailure:
                if not result.is_file():raise
        except StepFailure:
            self.report['flows'].append({'surface':surface,'id':'api-smoke','status':'unavailable','reason':'api-smoke-not-started'})
        except Exception:
            self.report['flows'].append({'surface':surface,'id':'api-smoke','status':'unavailable','reason':'api-smoke-harness'})
        finally:
            # Nothing here may raise: the browser follow-on and the setup grant
            # retirement continue whatever the smoke did.
            try:context.unlink()
            except FileNotFoundError:pass
            except OSError:self.report['flows'].append({'surface':surface,'id':'context-cleanup','status':'unavailable','reason':'context-not-removed'})
            self.merge_api_smoke(result)
    def merge_api_smoke(self,result):
        """Merge only closed rows from the CLI receipt; a malformed one is unavailable."""
        surface='server-api'
        if not result.is_file():return
        try:
            document=json.loads(result.read_text())
            flows=document['flows']
            if not isinstance(flows,list):raise ValueError('receipt-shape')
            codes,reasons=api_smoke_sets();rows=[]
            for flow in flows:
                if not isinstance(flow,dict):raise ValueError('receipt-row')
                identifier,status=flow.get('id'),flow.get('status')
                if not isinstance(identifier,str):raise ValueError('receipt-row')
                if status not in ('pass','fail','unavailable'):raise ValueError('receipt-row')
                if identifier not in API_SMOKE_FLOWS:continue
                row={'surface':surface,'id':identifier,'status':status}
                if flow.get('kind') in ('api','cleanup'):row['kind']=flow['kind']
                # A real endpoint cause survives: integer status plus the CLI's own
                # closed code and reason sets. Nothing else about the reply is copied.
                http=flow.get('http')
                if isinstance(http,int) and not isinstance(http,bool) and 100<=http<=599:row['http']=http
                code,reason=flow.get('code'),flow.get('reason')
                if isinstance(code,str) and code in codes:row['code']=code
                if isinstance(reason,str) and reason in reasons:row['reason']=reason
                rows.append(row)
            self.report['flows']+=rows
        except (OSError,ValueError,KeyError,TypeError):
            self.report['flows'].append({'surface':surface,'id':'api-smoke-receipt','status':'unavailable','reason':'receipt-unreadable'})
    def web_server(self):
        node=self.node();generated=self.attempt('server','official-generation',self.generation)
        server=self.attempt('server','server-build-and-release-guard',self.server_build)
        npm=self.attempt('web','dependency-install',lambda:self.run('npm',['npm','ci','--no-audit','--no-fund']))
        web=self.work/'web';built=npm and self.attempt('web','web-build',lambda:self.run('web',['npm','run','build','--workspace','portico-web-next','--','--outDir',str(web),'--emptyOutDir']))
        if npm:self.attempt('native-reader','immutable-reader-build',lambda:self.reader(node))
        self.attempt('server','reading-adversarial',lambda:self.run('reading-adversarial',['go','test','-count=1','./internal/reading','-run','EPUB|XHTML|CSS|Srcset|Archive'],self.source/'server',timeout=180),'tests')
        if npm:self.attempt('web','renderer-adversarial',lambda:self.run('renderer-adversarial',['npm','run','test','--workspace','@portico/reader-core'],timeout=180),'tests')
        if not (server and generated):
            self.report['flows'].append({'surface':'web','id':'browser-flows','status':'unavailable','dependency':'server build or official generation failed'});return
        bind=port();base='http://127.0.0.1:'+str(bind);state=self.work/'server-state';state.mkdir(mode=0o700);runtime=self.args.fixtures/'runtime/bundle'
        if not all((runtime/x).is_file() for x in ['toolchain-manifest.json','bin/ffmpeg','bin/ffprobe']):raise StepFailure('qualified-runtime-unavailable')
        self.run('runtime-manifest',['python3',str(self.source/'scripts/sonic-bundle.py'),'verify-installed',str(runtime),'--target','linux-x64'])
        env=dict(self.process_env,PORTICO_BIND='127.0.0.1:'+str(bind),PORTICO_DATA_DIR=str(state),PORTICO_WEB_DIR=str(web),PORTICO_FFMPEG=str(runtime/'bin/ffmpeg'),PORTICO_FFPROBE=str(runtime/'bin/ffprobe'),PORTICO_SERVICE='1',PORTICO_DISCOVERY='0',XDG_CACHE_HOME=str(self.work/'cache'))
        # Exercise the normal Linux service profile, while its fixed transcode
        # path is a private disk-backed bind mount rather than a host write.
        transcode=self.work/'transcode';transcode.mkdir(mode=0o700)
        app=self.spawn('server-runtime',['bwrap','--die-with-parent','--unshare-user','--ro-bind','/','/','--bind',str(self.work),str(self.work),'--tmpfs','/var/cache','--dir','/var/cache/portico','--bind',str(transcode),'/var/cache/portico','--proc','/proc','--dev','/dev','--',str(self.work/'server')],env=env);self.app=app;self.app_pid=app.pid;self.app_start=_start_ticks(app.pid);self.ready(base+'/health/ready',app)
        books=self.work/'book-fixture';books.mkdir(mode=0o700)
        epubs=list((self.args.fixtures/'media/Read/Books').rglob('*.epub'))
        if len(epubs)!=1 or hashlib.sha256(epubs[0].read_bytes()).hexdigest()!='85814417c1a74d2ca7f3b077b47454d0cfc728392d40d846d1c97d44f24c5279':raise StepFailure('unchanged-epub-fixture')
        shutil.copy2(epubs[0],books/epubs[0].name);self.report['epubFixtureSHA256']='85814417c1a74d2ca7f3b077b47454d0cfc728392d40d846d1c97d44f24c5279'
        username='cp'+secrets.token_hex(5);password='Cp9!'+secrets.token_urlsafe(24);installation=secrets.token_urlsafe(32)
        setup=self.json_call(base,'POST','/v1/setup/browser',{},installation=installation,headers={'Origin':base})['setupToken']
        owner=self.json_call(base,'POST','/v1/setup',{'setupToken':setup,'requestId':str(uuid.uuid4()),'name':'Portico checkpoint','authMode':'local','interactive':True,'username':username,'password':password},installation=installation,headers={'Origin':base},expected=(201,));grant=owner.get('session',owner);token=grant['accessToken']
        try:
            ready=self.json_call(base,'GET','/v1/setup/state',token=token,installation=installation);self.json_call(base,'POST','/v1/setup/finish',{'revision':ready['revision']},token,installation)
            self.json_call(base,'POST','/v1/direct/profiles',{'name':'Review','art':'mint'},token,installation,expected=(201,))
            settings=self.json_call(base,'GET','/v1/admin/console/settings',token=token,installation=installation)['data'];values=settings['requested'];values['conversionMode']='all';self.json_call(base,'PATCH','/v1/admin/console/settings',{'expectedRevision':settings['revision'],'idempotencyKey':str(uuid.uuid4()),'values':values},token,installation)
            libraries=[];jobs=[]
            for name,kind,path in [('Movies','movie','Movies'),('Music','music','Music'),('Books','book','Read/Books')]:
                library=self.json_call(base,'POST','/v1/libraries',{'name':name,'kind':kind,'path':str(books if kind=='book' else self.args.fixtures/'media'/path),'metadataAgent':'local'},token,installation,expected=(200,201));library=library.get('library',library);libraries.append(library)
                job=self.json_call(base,'POST','/v1/libraries/'+library['id']+'/scans',{},token,installation,expected=(200,201,202));jobs.append(job.get('jobId',job.get('id')))
            deadline=time.monotonic()+150
            while time.monotonic()<deadline:
                states=[]
                for job in jobs:
                    j=self.json_call(base,'GET','/v1/admin/ingestion/jobs/'+job,token=token,installation=installation);j=j.get('job',j);states.append(j.get('status'))
                if all(x in ['complete','complete_with_warnings','completed','failed','cancelled'] for x in states):break
                time.sleep(2)
            home=self.json_call(base,'GET','/v1/home',token=token,installation=installation);self.report['flows'].append({'surface':'server','id':'real-fixture-ingestion-home','status':'pass' if home.get('rows') else 'fail','rows':len(home.get('rows',[]))})
            if home.get('rows'):self.api_smoke(base,bind,token,installation)
        finally:self.json_call(base,'DELETE','/v1/devices/'+grant['deviceId'],token=token,installation=installation,expected=(204,))
        if built:self.browser(node,{'surface':'web','origin':base,'username':username,'password':password,'profileName':username,'reviewProfile':'Review','disabled':self.args.disabled})
    def hosted(self):
        node=self.node();self.generation();module=self.tools/'hosted-services/hosted';self.run('hosted',['go','build','-tags','devtrust','-p=2','-trimpath','-buildvcs=false','-o',str(self.work/'hosted'),'./cmd/hosted'],module);self.run('hosted-web-host',['go','build','-p=2','-trimpath','-buildvcs=false','-o',str(self.work/'web-host'),'./cmd/web'],module)
        versions=sorted(Path('/usr/lib/postgresql').glob('*/bin/initdb'),reverse=True)
        if not versions:raise StepFailure('postgres-tools-unavailable')
        pg=versions[0].parent;pgroot=self.work/'postgres';pwfile=self.work/'postgres-password';password=secrets.token_urlsafe(32);pwfile.write_text(password);pwfile.chmod(0o600)
        self.run('postgres-init',[str(pg/'initdb'),'-D',str(pgroot),'-U','checkpoint','--auth-local=trust','--auth-host=scram-sha-256','--pwfile',str(pwfile),'--no-locale','--encoding=UTF8']);pgport=port();pgprocess=self.spawn('postgres',[str(pg/'postgres'),'-D',str(pgroot),'-h','127.0.0.1','-p',str(pgport),'-k',str(self.work)])
        for _ in range(100):
            if pgprocess.poll() is not None:raise StepFailure('postgres-start')
            if subprocess.run([str(pg/'pg_isready'),'-h','127.0.0.1','-p',str(pgport)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:break
            time.sleep(.1)
        pg_env=dict(self.process_env,PGPASSWORD=password);self.run('database',[str(pg/'createdb'),'-h','127.0.0.1','-p',str(pgport),'-U','checkpoint','checkpoint'],env=pg_env)
        dsn=self.work/'database-url';dsn.write_text('postgres://checkpoint:'+password+'@127.0.0.1:'+str(pgport)+'/checkpoint?sslmode=disable');dsn.chmod(0o600)
        keys=self.work/'keys';self.run('signing-custody',[str(self.work/'hosted'),'keys','setup','--private-out',str(self.work/'root-private.pem'),'--public-out',str(self.work/'root-public.pem'),'--staging',str(keys)])
        mail=Mailbox(self);api='http://127.0.0.1:'+str(port());web='http://127.0.0.1:'+str(port());oidc=self.work/'oidc.json';private_json(oidc,{'webOrigin':web})
        args=[str(self.work/'hosted'),'--listen',api.removeprefix('http://'),'--database-url-file',str(dsn),'--root-public-key-file',str(self.work/'root-public.pem'),'--key-file',str(keys/'signing.json'),'--origins',web,'--claim-audience','https://checkpoint.m0.getportico.invalid','--tv-issuer','https://checkpoint.m0.getportico.invalid','--device-verification-uri','https://checkpoint.m0.getportico.invalid/device','--allow-loopback-enrollment','--oidc-config-file',str(oidc),'--http-mail-config-file',str(mail.config),'--metrics-listen','127.0.0.1:'+str(port())]
        app=self.spawn('hosted-runtime',args,env=dict(self.process_env,SSL_CERT_FILE=str(mail.cert)));self.ready(api+'/ready',app)
        env=dict(self.process_env,VITE_HOSTED_WEB='true',VITE_ACCOUNT_WORKSPACE='true',VITE_HOSTED_URL=api);self.run('npm',['npm','ci','--ignore-scripts','--no-audit','--no-fund']);webroot=self.work/'services-web';self.run('services-web',[node,'--input-type=module','-e',"import {build} from 'vite'; await build({build:{outDir:"+json.dumps(str(webroot))+",emptyOutDir:true,sourcemap:false}});"],self.source/'web-next',env)
        host=self.spawn('services-static',[str(self.work/'web-host'),'--listen',web.removeprefix('http://'),'--root',str(webroot)]);self.ready(web+'/',host)
        username='cp'+secrets.token_hex(5);password='Cp9!'+secrets.token_urlsafe(24);address=username+'@m0.getportico.invalid';continuation=secrets.token_urlsafe(32);headers={'X-Portico-Continuation':continuation}
        registration=self.json_call(api,'POST','/v1/registrations',{'username':username,'displayName':'Checkpoint','password':password,'contactAddress':address,'sessionMode':'native','requestId':secrets.token_urlsafe(24)},headers=headers,expected=(202,));code=mail.code(address)
        grant=self.json_call(api,'POST','/v1/registrations/'+registration['registrationId']+'/verify',{'code':code,'username':username,'displayName':'Checkpoint'},headers=headers,expected=(201,));self.report['flows'].append({'surface':'hosted','id':'real-registration-mail-verification','status':'pass','delivery':'verified TLS private mail gateway, invalid-domain recipient only'})
        self.json_call(api,'DELETE','/v1/sessions/current',token=grant['accessToken'],expected=(204,200))
        self.browser(node,{'surface':'hosted','origin':web,'apiOrigin':api,'username':username,'password':password,'disabled':self.args.disabled})
    def finish(self):
        for box in self.mailboxes:
            if hasattr(box,'server'):box.server.shutdown();box.server.server_close()
            box.messages.clear()
        for process in reversed(self.children):
            if process.poll() is None:
                with contextlib.suppress(ProcessLookupError):os.killpg(process.pid,signal.SIGTERM)
                try:process.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    with contextlib.suppress(ProcessLookupError):os.killpg(process.pid,signal.SIGKILL)
                    process.wait()
        for log in self.logs:log.close()
        self.report['cleanup']='all owned runtime processes stopped';(self.out/'report.log').write_text(json.dumps(self.report,indent=2)+'\n')
        destination=Path('/home/runner/work/artifacts/portico-v3-checkpoints')/self.args.run_id;destination.mkdir(parents=True,mode=0o700,exist_ok=False)
        # This log exists only for a process that exited before fixture setup,
        # before credentials or account/session state were created. Keep it in
        # private evidence for source-level startup diagnosis; never print it.
        startup=self.work/'server-runtime.private.log'
        if self.report.get('runtimeExit') and startup.is_file():shutil.copyfile(startup,destination/'startup-private.log')
        for file in self.out.iterdir():
            if not file.is_file() or file.is_symlink():continue
            if file.name in ['report.log','generation.patch.log','browser-results.log','reader-guest-receipt.log','reader-guest.tar','reading-layout.log'] or re.fullmatch(r'flow-[a-z0-9-]+\.(?:jpg|log)',file.name) or file.name.startswith('build-') and file.suffix=='.log':shutil.copyfile(file,destination/file.name)
        shutil.rmtree(self.work)

class Mailbox:
    def __init__(self,job):
        job.mailboxes.append(self)
        self.messages=[];self.token=secrets.token_urlsafe(32);self.cert=job.work/'mail-cert.pem';key=job.work/'mail-key.pem';self.lock=threading.Lock()
        job.run('mail-tls',['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(key),'-out',str(self.cert),'-days','1','-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost,IP:127.0.0.1'])
        box=self
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self,*args):pass
            def do_POST(self):
                size=int(self.headers.get('Content-Length','0'))
                if self.path!='/mail' or self.headers.get('Authorization')!='Bearer '+box.token or not 0<size<512*1024:self.send_error(403);return
                try:
                    data=json.loads(self.rfile.read(size));assert data['to'].endswith('@m0.getportico.invalid')
                    with box.lock:
                        assert len(box.messages)<32;box.messages.append((data['to'],data['rawMessage']))
                except Exception:self.send_error(400);return
                self.send_response(204);self.end_headers()
        self.server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);tls.load_cert_chain(str(self.cert),str(key));self.server.socket=tls.wrap_socket(self.server.socket,server_side=True);threading.Thread(target=self.server.serve_forever,daemon=True).start();self.config=job.work/'mail.json';token=job.work/'mail-token';token.write_text(self.token);token.chmod(0o600);private_json(self.config,{'endpoint':'https://localhost:'+str(self.server.server_port)+'/mail','from':'checkpoint@m0.getportico.invalid','tokenFile':str(token)})
    def code(self,address):
        for _ in range(150):
            with self.lock:messages=[m for to,m in self.messages if to==address]
            if messages:
                message=email.message_from_string(messages[-1]);plain='\n'.join(part.get_payload(decode=True).decode('utf-8','replace') for part in message.walk() if part.get_content_type()=='text/plain');match=re.search(r'\b([0-9]{6})\b',plain)
                if match:return match.group(1)
                raise StepFailure('issued-mail-code')
            time.sleep(.2)
        raise StepFailure('mail-delivery-deadline')

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--surface',choices=['web-server','hosted'],required=True);p.add_argument('--server',required=True);p.add_argument('--private',required=True);p.add_argument('--run-id',required=True);p.add_argument('--fixtures',type=Path,required=True);p.add_argument('--chromium',required=True);p.add_argument('--disabled',type=json.loads,default=[]);p.add_argument('--reader-source-verified',action='store_true');a=p.parse_args()
    if not all(re.fullmatch('[0-9a-f]{40}',x) for x in [a.server,a.private]) or not re.fullmatch('[a-z0-9-]{1,96}',a.run_id):p.error('exact sources and safe run id required')
    if a.fixtures!=Path('/home/runner/work/fixtures/portico-v3'):p.error('fixed owned fixture custody required')
    os.umask(0o077);job=Job(a)
    def stop(*_):raise SystemExit(143)
    signal.signal(signal.SIGTERM,stop);signal.signal(signal.SIGHUP,stop)
    deadline=threading.Timer(1800,lambda:os.kill(os.getpid(),signal.SIGTERM));deadline.daemon=True;deadline.start()
    try:job.web_server() if a.surface=='web-server' else job.hosted()
    except StepFailure as error:job.report['flows'].append({'surface':a.surface,'id':error.step,'status':'fail','kind':'build' if error.step.startswith(('server','web','hosted','npm','api-generation','schema-generation')) else 'infrastructure'})
    except Exception as error:job.report['flows'].append({'surface':a.surface,'id':'runner','status':'fail','class':type(error).__name__,'frames':[{'file':Path(f.filename).name,'line':f.lineno} for f in traceback.extract_tb(error.__traceback__)[-6:]]})
    finally:deadline.cancel();job.finish()
    print(json.dumps({'run':a.run_id,'flows':job.report['flows'],'cleanup':job.report['cleanup']}));return int(any(x['status']=='fail' for x in job.report['flows']))
if __name__=='__main__':raise SystemExit(main())
