"""Build an exact published server commit and its web client on the runner host.

The build is release-tagged with the official Hosted root, refuses the
development root, requires zero official API/schema generation drift and
records the exact binary/web hashes. Build output stays in a private log.
"""
from __future__ import annotations

import base64
import hashlib
import json
import os
import re
from dataclasses import dataclass
from pathlib import Path

from .config import (BUILD_TIMEOUT, GOMAXPROCS, GOMEMLIMIT, NODE_OPTIONS, NPM_TIMEOUT,
                     now_iso)
from .process import capture, run as run_process
from .receipt import write_json
from .source import SourceError, generate

class BuildError(Exception):
    """A closed build refusal."""

@dataclass(frozen=True)
class Build:
    commit: str
    directory: Path
    binary: Path
    web: Path
    binary_sha256: str
    web_sha256: str
    official_root_sha256: str
    generated_clean: bool
    generated_files: int
    generation_patch_sha256: str
    schema_digest: str
    fixtures_manifest_sha256: str
    runtime_sha256: str
    built_at: str
    manifest: Path

def _run(argv, cwd, log, timeout, env):
    return run_process(argv, cwd, log, timeout, env)

def _file_hash(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as stream:
        while True:
            block = stream.read(1024 * 1024)
            if not block:
                break
            digest.update(block)
    return digest.hexdigest()

def _tree_hash(root):
    root = Path(root)
    digest = hashlib.sha256()
    for path in sorted(root.rglob('*')):
        if path.is_symlink():
            raise BuildError('web-symlink')
        if not path.is_file():
            continue
        digest.update(str(path.relative_to(root)).encode() + b'\0')
        digest.update(_file_hash(path).encode() + b'\n')
    return digest.hexdigest()

_SCHEMA = (
    'server/api/portico.openapi.json',
    'server/internal/persistence/known_schema.go',
    'server/internal/persistence/known_columns.go',
    'server/internal/persistence/schema_test.go',
    'server/internal/httpapi/route_lanes.go',
    'packages/contracts/src',
)

def _schema_digest(tree):
    tree = Path(tree)
    digest = hashlib.sha256()
    for name in _SCHEMA:
        path = tree / name
        if path.is_dir():
            for file in sorted(path.rglob('*')):
                if file.is_file() and not file.is_symlink():
                    digest.update(str(file.relative_to(tree)).encode() + b'\0')
                    digest.update(_file_hash(file).encode() + b'\n')
        elif path.is_file():
            digest.update(name.encode() + b'\0')
            digest.update(_file_hash(path).encode() + b'\n')
    return digest.hexdigest()

def _official_root(tree):
    pem = (Path(tree) / 'trust/hosted-root-public-key.pem').read_bytes()
    body = ''.join(line for line in pem.decode().splitlines() if not line.startswith('---'))
    try:
        der = base64.b64decode(body, validate=True)
    except ValueError:
        raise BuildError('official-root-format') from None
    if der[:12] != bytes.fromhex('302a300506032b6570032100') or len(der) != 44:
        raise BuildError('official-root-format')
    key = base64.urlsafe_b64encode(der[-32:]).decode().rstrip('=')
    if key in (Path(tree) / 'server/internal/hostedtrust/development_root.go').read_text():
        raise BuildError('development-root-refused')
    return key, hashlib.sha256(pem).hexdigest()

def _go_version(tree):
    text = (Path(tree) / 'server/go.mod').read_text()
    match = re.search(r'^go (\d+\.\d+\.\d+)$', text, re.M)
    if not match:
        raise BuildError('go-version')
    return match.group(1)

def _node_version(tree):
    value = (Path(tree) / '.node-version').read_text().strip()
    if not re.fullmatch(r'\d+\.\d+\.\d+', value):
        raise BuildError('node-version')
    return value

def _node(tree, log):
    version = _node_version(tree)
    env = dict(os.environ, GIT_TERMINAL_PROMPT='0')
    code, output = capture(['npm', 'exec', '--yes', '--package=node@' + version, '--',
                            'node', '-p', 'process.execPath'], tree, log, 180, env)
    if code:
        raise BuildError('node-toolchain')
    path = Path(output.decode().strip())
    code, output = capture([str(path), '--version'], tree, log, 30, env)
    if code or output.decode().strip() != 'v' + version:
        raise BuildError('node-version')
    return str(path), version

def build(source, build_dir, fixtures, paths):
    tree = Path(source.tree)
    build_dir = Path(build_dir)
    build_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    log = build_dir / 'build.log'
    log.touch(mode=0o600, exist_ok=True)
    key, root_sha = _official_root(tree)
    try:
        generation = generate(tree, log, build_dir / 'generation.patch')
    except SourceError as error:
        raise BuildError(error.args[0] if error.args else 'generation-failed') from None
    go_version = _go_version(tree)
    schema_digest = _schema_digest(tree)
    env = dict(os.environ, GOMAXPROCS=GOMAXPROCS, GOMEMLIMIT=GOMEMLIMIT,
               GOTOOLCHAIN='go' + go_version, GIT_TERMINAL_PROMPT='0')
    if _run(['python3', 'scripts/sonic-bundle.py', 'verify-installed', str(fixtures.runtime),
             '--target', 'linux-x64'], tree, log, 180, env):
        raise BuildError('runtime-unverified')
    flags = ('-X portico.local/server/internal/buildinfo.Version=0.1.0-walk+' + source.commit[:8]
             + ' -X portico.local/server/internal/buildinfo.BuildID=' + source.commit
             + ' -X portico.local/server/internal/hostedtrust.OfficialRoot=' + key)
    binary = build_dir / 'portico-server'
    if _run(['go', 'build', '-p=2', '-tags', 'release', '-trimpath', '-buildvcs=false',
             '-ldflags', flags, '-o', str(binary), './cmd/server'],
            tree / 'server', log, BUILD_TIMEOUT, env):
        raise BuildError('server-build')
    if _run([str(binary), '--version'], build_dir, log, 30, env):
        raise BuildError('server-version')
    node, node_version = _node(tree, log)
    node_env = dict(env, PATH=str(Path(node).parent) + os.pathsep + env.get('PATH', ''),
                    NODE_OPTIONS=NODE_OPTIONS)
    if _run(['npm', 'ci', '--no-audit', '--no-fund'], tree, log, NPM_TIMEOUT, node_env):
        raise BuildError('npm-install')
    web = build_dir / 'web'
    if _run(['npm', 'run', 'build', '--workspace', 'portico-web-next', '--',
             '--outDir', str(web), '--emptyOutDir'], tree, log, NPM_TIMEOUT, node_env):
        raise BuildError('web-build')
    if not (web / 'index.html').is_file():
        raise BuildError('web-incomplete')
    write_json(web / 'portico-build.json',
               {'commit': source.commit, 'client': 'web-next', 'purpose': 'walk'}, 0o644)
    binary_sha = _file_hash(binary)
    web_sha = _tree_hash(web)
    manifest = {
        'commit': source.commit,
        'builtAt': now_iso(),
        'binarySHA256': binary_sha,
        'webSHA256': web_sha,
        'officialRootSHA256': root_sha,
        'generatedClean': generation.clean,
        'generatedFiles': generation.changed,
        'generationPatchSHA256': generation.patch_sha256,
        'generationSource': 'official-apigen+schema-sync',
        'generationPatch': 'generation.patch' if generation.patch_path is not None else '',
        'schemaDigest': schema_digest,
        'fixturesManifestSHA256': fixtures.manifest_sha256,
        'runtimeSHA256': fixtures.runtime_sha256,
        'goVersion': go_version,
        'nodeVersion': node_version,
        'state': 'built',
        'runtimeVerified': False,
        'usable': False,
        'handoff': 'built but not yet running; not a usable walk server',
    }
    manifest_path = build_dir / 'manifest.json'
    write_json(manifest_path, manifest)
    return Build(commit=source.commit, directory=build_dir, binary=binary, web=web,
                 binary_sha256=binary_sha, web_sha256=web_sha, official_root_sha256=root_sha,
                 generated_clean=generation.clean, generated_files=generation.changed,
                 generation_patch_sha256=generation.patch_sha256,
                 schema_digest=schema_digest,
                 fixtures_manifest_sha256=fixtures.manifest_sha256,
                 runtime_sha256=fixtures.runtime_sha256, built_at=manifest['builtAt'],
                 manifest=manifest_path)

def load(directory):
    directory = Path(directory)
    document = json.loads((directory / 'manifest.json').read_text())
    if not isinstance(document, dict):
        raise BuildError('manifest-missing')
    binary = directory / 'portico-server'
    web = directory / 'web'
    if not binary.is_file() or not (web / 'index.html').is_file():
        raise BuildError('build-incomplete')
    binary_hash = _file_hash(binary)
    web_hash = _tree_hash(web)
    if binary_hash != document.get('binarySHA256') or web_hash != document.get('webSHA256'):
        raise BuildError('build-hash-mismatch')
    schema = document.get('schemaDigest')
    if not isinstance(schema, str) or len(schema) != 64:
        raise BuildError('build-schema-missing')
    patch_name = document.get('generationPatch')
    patch_hash = document.get('generationPatchSHA256', '')
    if patch_name:
        if patch_name != 'generation.patch':
            raise BuildError('generation-patch-mismatch')
        patch = directory / patch_name
        if not patch.is_file() or _file_hash(patch) != patch_hash:
            raise BuildError('generation-patch-mismatch')
    elif patch_hash:
        raise BuildError('generation-patch-mismatch')
    return Build(commit=document['commit'], directory=directory, binary=binary, web=web,
                 binary_sha256=binary_hash, web_sha256=web_hash,
                 official_root_sha256=document['officialRootSHA256'],
                 generated_clean=bool(document.get('generatedClean')),
                 generated_files=int(document.get('generatedFiles') or 0),
                 generation_patch_sha256=document.get('generationPatchSHA256', ''),
                 schema_digest=schema,
                 fixtures_manifest_sha256=document.get('fixturesManifestSHA256', ''),
                 runtime_sha256=document.get('runtimeSHA256', ''),
                 built_at=document.get('builtAt', ''), manifest=directory / 'manifest.json')
