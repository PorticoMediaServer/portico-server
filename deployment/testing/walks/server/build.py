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

# The sprint exercises the existing bounded local CB7 and RAR5 readers.
# Source leases, decoder limits and format guards remain the server's.
BUILD_TAGS = ('release', 'portico_cb7', 'portico_rar7z')
# Keep prior recorded artifacts readable for rollback; reuse still requires
# exact current BUILD_TAGS in the coordinator. Missing tags imply no capability.
_RECORDED_BUILD_TAGS = ((), ('release', 'portico_cb7'), BUILD_TAGS)

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
    rclone_version: str = ''
    rclone_manifest_sha256: str = ''
    rclone_files: tuple[tuple[str, str], ...] = ()
    build_tags: tuple[str, ...] = ()

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

# The walk server runs on linux/amd64, and the source's bundle lock pins that target.
RCLONE_TARGET = 'linux-amd64'
RCLONE_RELATIVE = 'third_party/rclone'
RCLONE_TIMEOUT = 600
_HELPER_MAX_FILES = 32
_HELPER_MAX_FILE_BYTES = 160 * 1024 * 1024
_HELPER_MAX_TOTAL_BYTES = 256 * 1024 * 1024
_HELPER_NAME = re.compile(r'[A-Za-z0-9][A-Za-z0-9._-]{0,127}\Z')
_HEX40 = re.compile(r'[0-9a-f]{40}\Z')
_HEX64 = re.compile(r'[0-9a-f]{64}\Z')

def _helper_inventory(bundle):
    """Every actual file in a prepared helper, as a bounded name->sha256 map.

    The directory and its parent must be real, not links; the walk stops at the
    thirty-third entry before reading it, and each file is size-checked before it
    is hashed so an oversized payload is refused rather than read.
    """
    bundle = Path(bundle)
    if bundle.is_symlink():
        raise BuildError('rclone-inventory')
    parent = bundle.parent
    if parent.is_symlink() or (parent.exists() and not parent.is_dir()):
        raise BuildError('rclone-inventory')
    if not bundle.exists():
        raise BuildError('rclone-missing')
    if not bundle.is_dir():
        raise BuildError('rclone-inventory')
    files = {}
    total = 0
    with os.scandir(bundle) as entries:
        for entry in entries:
            if len(files) >= _HELPER_MAX_FILES:
                raise BuildError('rclone-inventory')
            if not _HELPER_NAME.fullmatch(entry.name):
                raise BuildError('rclone-inventory')
            if entry.is_symlink() or not entry.is_file(follow_symlinks=False):
                raise BuildError('rclone-inventory')
            size = entry.stat(follow_symlinks=False).st_size
            if size > _HELPER_MAX_FILE_BYTES:
                raise BuildError('rclone-inventory')
            total += size
            if total > _HELPER_MAX_TOTAL_BYTES:
                raise BuildError('rclone-inventory')
            files[entry.name] = _file_hash(Path(entry.path))
    if not files:
        raise BuildError('rclone-inventory')
    return files

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
    if _run(['go', 'build', '-p=2', '-tags', ','.join(BUILD_TAGS), '-trimpath', '-buildvcs=false',
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
    # The source needs a newer rclone than the host's; a bare walk build has no
    # system install and no PATH override, so the pinned bundle is prepared from
    # the same source with an owned cache and executed once to prove its version.
    rclone_dir = build_dir / RCLONE_RELATIVE
    if rclone_dir.exists():
        raise BuildError('rclone-present')
    rclone_cache = Path(paths.work) / 'rclone-cache'
    if _run(['python3', 'scripts/rclone-bundle.py', 'prepare', str(rclone_dir),
             '--target', RCLONE_TARGET, '--cache', str(rclone_cache)],
            tree, log, RCLONE_TIMEOUT, env):
        raise BuildError('rclone-prepare')
    if _run(['python3', 'scripts/rclone-bundle.py', 'verify', str(rclone_dir),
             '--target', RCLONE_TARGET, '--execute-version'], tree, log, 120, env):
        raise BuildError('rclone-verify')
    helper_manifest_path = rclone_dir / 'bundle-manifest.json'
    try:
        helper_manifest = json.loads(helper_manifest_path.read_text())
    except (OSError, ValueError):
        raise BuildError('rclone-manifest') from None
    rclone_version = helper_manifest.get('version')
    rclone_revision = helper_manifest.get('revision')
    if (not isinstance(rclone_version, str) or not rclone_version
            or not isinstance(rclone_revision, str) or not rclone_revision
            or helper_manifest.get('target') != RCLONE_TARGET):
        raise BuildError('rclone-manifest')
    rclone_files = _helper_inventory(rclone_dir)
    rclone_manifest_sha = _file_hash(helper_manifest_path)
    binary_sha = _file_hash(binary)
    web_sha = _tree_hash(web)
    manifest = {
        'commit': source.commit,
        'buildTags': list(BUILD_TAGS),
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
        'rclone': {'target': RCLONE_TARGET, 'version': rclone_version,
                   'revision': rclone_revision, 'path': RCLONE_RELATIVE,
                   'manifestSHA256': rclone_manifest_sha, 'files': rclone_files},
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
                 manifest=manifest_path, rclone_version=rclone_version,
                 rclone_manifest_sha256=rclone_manifest_sha,
                 rclone_files=tuple(sorted(rclone_files.items())), build_tags=BUILD_TAGS)

def load(directory):
    directory = Path(directory)
    document = json.loads((directory / 'manifest.json').read_text())
    if not isinstance(document, dict):
        raise BuildError('manifest-missing')
    tags = document.get('buildTags', [])
    if not isinstance(tags, list) or tuple(tags) not in _RECORDED_BUILD_TAGS:
        raise BuildError('build-tags-mismatch')
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
    rclone_version, rclone_manifest_sha, rclone_files = '', '', ()
    recorded = document.get('rclone')
    if recorded is not None:
        # A build that records the helper must still match it exactly. A manifest
        # from before the helper existed simply has no entry and stays that way.
        if not isinstance(recorded, dict):
            raise BuildError('rclone-manifest')
        target, relative = recorded.get('target'), recorded.get('path')
        version, revision = recorded.get('version'), recorded.get('revision')
        manifest_sha, files = recorded.get('manifestSHA256'), recorded.get('files')
        if target != RCLONE_TARGET or relative != RCLONE_RELATIVE:
            raise BuildError('rclone-manifest')
        if not isinstance(version, str) or not version:
            raise BuildError('rclone-manifest')
        if not isinstance(revision, str) or not _HEX40.fullmatch(revision):
            raise BuildError('rclone-manifest')
        if not isinstance(manifest_sha, str) or not _HEX64.fullmatch(manifest_sha):
            raise BuildError('rclone-manifest')
        if not isinstance(files, dict) or not files or len(files) > _HELPER_MAX_FILES:
            raise BuildError('rclone-manifest')
        expected = {}
        for name, digest in files.items():
            # No coercion: a key or digest that is not exactly a safe flat name or
            # a 64-hex string refuses the build instead of being stringified in.
            if not isinstance(name, str) or not _HELPER_NAME.fullmatch(name):
                raise BuildError('rclone-manifest')
            if not isinstance(digest, str) or not _HEX64.fullmatch(digest):
                raise BuildError('rclone-manifest')
            expected[name] = digest
        bundle = directory / relative
        on_disk = _helper_inventory(bundle)
        if _file_hash(bundle / 'bundle-manifest.json') != manifest_sha:
            raise BuildError('rclone-hash-mismatch')
        if on_disk != expected:
            raise BuildError('rclone-hash-mismatch')
        try:
            helper = json.loads((bundle / 'bundle-manifest.json').read_text())
        except (OSError, ValueError):
            raise BuildError('rclone-manifest') from None
        if (not isinstance(helper, dict) or helper.get('target') != RCLONE_TARGET
                or helper.get('version') != version or helper.get('revision') != revision):
            raise BuildError('rclone-manifest')
        rclone_version, rclone_manifest_sha = version, manifest_sha
        rclone_files = tuple(sorted(on_disk.items()))
    return Build(commit=document['commit'], directory=directory, binary=binary, web=web,
                 binary_sha256=binary_hash, web_sha256=web_hash,
                 official_root_sha256=document['officialRootSHA256'],
                 generated_clean=bool(document.get('generatedClean')),
                 generated_files=int(document.get('generatedFiles') or 0),
                 generation_patch_sha256=document.get('generationPatchSHA256', ''),
                 schema_digest=schema,
                 fixtures_manifest_sha256=document.get('fixturesManifestSHA256', ''),
                 runtime_sha256=document.get('runtimeSHA256', ''),
                 built_at=document.get('builtAt', ''), manifest=directory / 'manifest.json',
                 rclone_version=rclone_version, rclone_manifest_sha256=rclone_manifest_sha,
                 rclone_files=rclone_files, build_tags=tuple(tags))
