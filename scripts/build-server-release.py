#!/usr/bin/env python3
"""Build only from an isolated source snapshot; never seal a supplied executable."""
import argparse,datetime,hashlib,json,os,pathlib,shutil,subprocess,uuid,shlex,re,base64
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey,Ed25519PublicKey
ROOT=pathlib.Path(__file__).resolve().parent.parent

def official_root():
    path=pathlib.Path(os.environ.get('PORTICO_ROOT_PUBLIC_KEY_FILE',str(ROOT/'trust/hosted-root-public-key.pem')))
    if not path.is_file():raise SystemExit('Official root public key file required for release')
    raw=path.read_bytes();key=serialization.load_pem_public_key(raw)
    if not isinstance(key,Ed25519PublicKey):raise SystemExit('Official root must be Ed25519')
    encoded=base64.urlsafe_b64encode(key.public_bytes_raw()).decode().rstrip('=')
    if encoded in (ROOT/'server/internal/hostedtrust/development_root.go').read_text():raise SystemExit('Development root is forbidden in release builds')
    return encoded,raw

def snapshot_digest(source):
    _,files=digest_source(source)
    # Every local module the build compiles (go.mod replaces apikit with ../apikit).
    for name in ('apikit',):
        dependency=source.parent/name
        if dependency.is_dir():
            _,rows=digest_source(dependency)
            files.extend({'path':'../'+name+'/'+r['path'],'sha256':r['sha256']} for r in rows)
    public=source.parent/'root-public-key.pem'
    if public.is_file():files.append({'path':'../root-public-key.pem','sha256':hashlib.sha256(public.read_bytes()).hexdigest()})
    return hashlib.sha256(json.dumps(files,separators=(',',':'),sort_keys=True).encode()).hexdigest(),files

def controlled_env():
    env=os.environ.copy()
    for key in ('GOOS','GOARCH','GOEXPERIMENT','GOROOT','GOTOOLDIR','GOTMPDIR','CC','CXX','CGO_CFLAGS','CGO_CPPFLAGS','CGO_CXXFLAGS','CGO_LDFLAGS'):env.pop(key,None)
    env.update({'GOFLAGS':'','GOWORK':'off','GOENV':'off','GOTOOLCHAIN':'local','GOPROXY':'https://proxy.golang.org','GOSUMDB':'sum.golang.org','GOPRIVATE':'','GONOSUMDB':'','GONOPROXY':''})
    return env

def signing_key():
    directory=ROOT/'runtime'/'release-signing';directory.mkdir(mode=0o700,parents=True,exist_ok=True);directory.chmod(0o700);path=directory/'private-key'
    if not path.exists():
        key=Ed25519PrivateKey.generate()
        try:
            with os.fdopen(os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600),'wb') as output:output.write(key.private_bytes_raw());output.flush();os.fsync(output.fileno())
        except FileExistsError:pass
    if path.is_symlink() or path.stat().st_mode&0o077:raise ValueError('Release signing key permissions are unsafe')
    key=Ed25519PrivateKey.from_private_bytes(path.read_bytes());(directory/'public-key').write_bytes(key.public_key().public_bytes_raw());return key

def source_files(source):
    # The whole module tree: Go source, embedded files (migrations, fonts, the
    # Cast receiver), protocol vectors and test data. Filtering by extension
    # left embedded files out, and the snapshot could not compile.
    return sorted(p for p in source.rglob('*') if p.is_file() and p.name!='.DS_Store' and not any(x in ('runtime','bin','vendor','.scratch','node_modules') for x in p.relative_to(source).parts))

def digest_source(source):
    files=[{'path':str(p.relative_to(source)), 'sha256':hashlib.sha256(p.read_bytes()).hexdigest()} for p in source_files(source)]
    digest=hashlib.sha256(json.dumps(files,separators=(',',':'),sort_keys=True).encode()).hexdigest()
    return digest,files

def artifact_identity(artifact):
    info=json.loads(subprocess.check_output([str(ROOT/'scripts/env.sh'),'go','version','-m','-json',str(artifact)],text=True,env=controlled_env()))
    settings={entry['Key']:entry.get('Value','') for entry in info.get('Settings',[])}
    if any(key in settings for key in ('-overlay','-toolexec','-modfile','-pkgdir')):raise ValueError('Unexpected compiler input override')
    args=shlex.split(settings.get('-ldflags',''));embedded={}
    for index,arg in enumerate(args):
        if arg=='-X' and index+1<len(args):
            key,separator,value=args[index+1].partition('=')
            if separator:embedded[key]=value
    proofs=set(re.findall(rb'PORTICO_SOURCE_V1:([a-f0-9]{64}):([A-Za-z0-9_-]+):END',artifact.read_bytes()))
    module=info.get('Path','').split('/cmd/')[0]
    if module not in ('portico.local/server','portico.local/hosted'):raise ValueError('Unexpected application module')
    prefix=module+'/internal/buildinfo.'
    expected=(embedded.get(prefix+'SourceDigest','').encode(),embedded.get(prefix+'BuildID','').encode())
    if proofs and proofs!={expected}:raise ValueError('Proof marker disagrees with compiler build identity')
    return embedded,settings

def verify_manifest(directory,expected_source,trusted_public_key):
    manifest_bytes=(directory/'manifest.json').read_bytes()
    Ed25519PublicKey.from_public_bytes(trusted_public_key).verify((directory/'manifest.sig').read_bytes(),manifest_bytes)
    manifest=json.loads(manifest_bytes)
    if manifest['sourceDigest']!=expected_source:raise ValueError('Release source does not match expected source')
    component=manifest.get('component','server')
    if component not in ('server','hosted'):raise ValueError('Invalid release component')
    artifact=directory/manifest['artifact']['path']
    if artifact.parent!=directory or artifact.name!='portico-'+component+('.exe' if manifest['target'].startswith('windows-') else ''):raise ValueError('Invalid artifact path')
    if hashlib.sha256(artifact.read_bytes()).hexdigest()!=manifest['artifact']['sha256']:raise ValueError('Release artifact was replaced or modified')
    source_digest,_=snapshot_digest(directory/'source')
    if source_digest!=expected_source:raise ValueError('Build snapshot changed')
    embedded,settings=artifact_identity(artifact)
    trust_symbol='portico.local/server/internal/hostedtrust.OfficialRoot' if component=='server' else 'portico.local/hosted/trust.OfficialRoot'
    if settings.get('-tags')!='release' or embedded.get(trust_symbol)!=manifest.get('officialRoot'):raise ValueError('Release lacks the certified official trust root')
    prefix='portico.local/'+component+'/internal/buildinfo.'
    if embedded.get(prefix+'SourceDigest')!=expected_source or embedded.get(prefix+'BuildID')!=manifest['buildId']:raise ValueError('Embedded build identity does not match release source')
    if manifest.get('version') is not None and embedded.get(prefix+'Version')!=manifest['version']:raise ValueError('Embedded version does not match release manifest')
    if settings.get('GOOS','')+'-'+settings.get('GOARCH','')!=manifest['target']:raise ValueError('Embedded target does not match release target')
    if component=='server':
        bundle=directory/'third_party/ffmpeg'
        subprocess.run(['python3',str(ROOT/'scripts/verify-ffmpeg-manifest.py'),str(bundle)],check=True)
        if hashlib.sha256((bundle/'toolchain-manifest.json').read_bytes()).hexdigest()!=manifest.get('toolchainManifestSha256'):raise ValueError('Toolchain manifest changed')
    return manifest

def release_version(value):
    numeric=r"(?:0|[1-9][0-9]*)"
    identifier=r"(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
    pattern=rf"{numeric}\.{numeric}\.{numeric}(?:-{identifier}(?:\.{identifier})*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
    if not re.fullmatch(pattern,value): raise argparse.ArgumentTypeError("Version must be a complete SemVer release version")
    return value

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--version',required=True,type=release_version);parser.add_argument('--os',choices=['linux','darwin','windows'],default='linux');parser.add_argument('--arch',choices=['amd64','arm64'],default='amd64');parser.add_argument('--component',choices=['server'],default='server');parser.add_argument('--ffmpeg-bundle',type=pathlib.Path,default=ROOT/'third_party/ffmpeg/bundle');parser.add_argument('--source-dir',type=pathlib.Path);args=parser.parse_args()
    root_key,root_pem=official_root()
    if args.component=='server':
        bundle=args.ffmpeg_bundle.resolve()
        subprocess.run(['python3',str(ROOT/'scripts/verify-ffmpeg-manifest.py'),str(bundle)],check=True)
        toolchain=json.loads((bundle/'toolchain-manifest.json').read_text())
        target={'darwin':'macos','linux':'linux','windows':'windows'}[args.os]+'-'+{'amd64':'x64','arm64':'arm64'}[args.arch]
        if toolchain['target']!=target:raise SystemExit('FFmpeg bundle target does not match server')
    builder_digest=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
    build_id=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+uuid.uuid4().hex[:8]
    stage=ROOT/'runtime'/'releases'/build_id;source=stage/'source';source.mkdir(parents=True)
    live=(args.source_dir or ROOT/args.component).resolve()
    if not live.is_relative_to(ROOT) or not (live/'go.mod').is_file():raise SystemExit('Source must be an application module inside this repository.')
    if not re.search(r'^module portico\.local/'+args.component+r'\s*$',(live/'go.mod').read_text(),re.M):raise SystemExit('Source module does not match release component.')
    # The source read is checked again after copying. Mixed concurrent edits fail before compiling.
    before,files=digest_source(live)
    for entry in files:
        dst=source/entry['path'];dst.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(live/entry['path'],dst)
    snapshot,copied=digest_source(source)
    if before!=snapshot or digest_source(live)[0]!=before:raise SystemExit('Source changed during capture; retry after edits settle.')
    for name in ('apikit',):
        if name==args.component:continue
        dependency=ROOT/name;initial,rows=digest_source(dependency)
        for row in rows:
            dst=stage/name/row['path'];dst.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(dependency/row['path'],dst)
        if digest_source(stage/name)[0]!=initial or digest_source(dependency)[0]!=initial:raise SystemExit('Dependency changed during capture')
    # Local module replacements and the cross-service policy test refer to siblings.
    (stage/args.component).symlink_to(source.name,target_is_directory=True)
    (stage/'root-public-key.pem').write_bytes(root_pem)
    snapshot,copied=snapshot_digest(source)
    checked_env=controlled_env()
    if args.component=='hosted':
        database_file=pathlib.Path(os.environ.get('PORTICO_TEST_DATABASE_URL_FILE',str(ROOT/'runtime/postgres/test-database-url')))
        if not database_file.is_file():raise SystemExit('Hosted release requires the isolated PostgreSQL integration database.')
        checked_env['PORTICO_TEST_DATABASE_URL_FILE']=str(database_file)
    subprocess.run([str(ROOT/'scripts/env.sh'),'go','mod','verify'],cwd=source,env=checked_env,check=True)
    if args.component=='server':
        helper=stage/'test-helper'
        subprocess.run([str(ROOT/'scripts/env.sh'),'go','build','-mod=readonly','-o',str(helper),'./cmd/server'],cwd=source,env=checked_env,check=True)
        checked_env['PORTICO_PLAYBACK_TEST_HELPER']=str(helper)
    # The 20,000-row metadata boundary test is deliberately exhaustive. Race
    # instrumentation of pure-Go SQLite exceeds Go's default ten-minute limit.
    subprocess.run([str(ROOT/'scripts/env.sh'),'go','test','-race','-timeout=30m','./...'],cwd=source,env=checked_env,check=True)
    subprocess.run([str(ROOT/'scripts/env.sh'),'go','vet','./...'],cwd=source,env=checked_env,check=True)
    built_at=datetime.datetime.now(datetime.timezone.utc).isoformat()
    prefix='portico.local/'+args.component+'/internal/buildinfo.'
    flags=' '.join(f'-X {prefix}{key}={value}' for key,value in {'SourceDigest':snapshot,'BuildID':build_id,'Version':args.version,'BuiltAt':built_at,'Proof':f'PORTICO_SOURCE_V1:{snapshot}:{build_id}:END'}.items())
    flags+=' -X '+('portico.local/server/internal/hostedtrust.OfficialRoot' if args.component=='server' else 'portico.local/hosted/trust.OfficialRoot')+'='+root_key
    env=controlled_env();env.update({'GOOS':args.os,'GOARCH':args.arch,'CGO_ENABLED':'0'})
    subprocess.run([str(ROOT/'scripts/env.sh'),'go','build','-tags','release','-buildvcs=false','-mod=readonly','-ldflags',flags,'-o',str(stage/('portico-'+args.component+('.exe' if args.os=='windows' else ''))),'./cmd/'+args.component],cwd=source,env=env,check=True)
    if snapshot_digest(source)[0]!=snapshot:raise SystemExit('Source snapshot changed during build.')
    if hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()!=builder_digest:raise SystemExit('Builder changed during execution; discard output and retry.')
    binary=stage/('portico-'+args.component+('.exe' if args.os=='windows' else ''));manifest={'schema':1,'version':args.version,'component':args.component,'buildId':build_id,'sourceDigest':snapshot,'officialRoot':root_key,'builtAt':built_at,'target':args.os+'-'+args.arch,'builderSha256':builder_digest,'sourceFiles':copied,'artifact':{'path':binary.name,'sha256':hashlib.sha256(binary.read_bytes()).hexdigest()}}
    if args.component=='server':
        bundle=args.ffmpeg_bundle.resolve()
        subprocess.run(['python3',str(ROOT/'scripts/verify-ffmpeg-manifest.py'),str(bundle)],check=True)
        toolchain=json.loads((bundle/'toolchain-manifest.json').read_text())
        target={'darwin':'macos','linux':'linux','windows':'windows'}[args.os]+'-'+{'amd64':'x64','arm64':'arm64'}[args.arch]
        if toolchain['target']!=target:raise SystemExit('FFmpeg bundle target does not match server')
        shutil.copytree(bundle,stage/'third_party/ffmpeg')
        manifest['toolchainManifestSha256']=hashlib.sha256((bundle/'toolchain-manifest.json').read_bytes()).hexdigest()
    (stage/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n');key=signing_key();(stage/'manifest.sig').write_bytes(key.sign((stage/'manifest.json').read_bytes()));verify_manifest(stage,snapshot,key.public_key().public_bytes_raw())
    print(json.dumps({'directory':str(stage),'sourceDigest':snapshot,'artifactSha256':manifest['artifact']['sha256'],'buildId':build_id}))
if __name__=='__main__':main()
