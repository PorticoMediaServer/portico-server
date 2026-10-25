#!/usr/bin/env python3
"""Release-checkpoint recipe only. No network, installation or PATH discovery.

Inputs: clean pinned upstream checkout, exact musl source archive, qualified
builder/tool manifest, private output parent. Must not run per implementation item.
"""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shlex
import shutil
import subprocess
import sys
import tarfile

HERE = Path(__file__).resolve().parent
PIN = json.loads((HERE / 'inputs.json').read_text())

def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def require(condition, message):
    if not condition:
        raise SystemExit(message)

def main():
    require(len(sys.argv) == 5, 'build.py LIBMOBI_CHECKOUT MUSL_ARCHIVE TOOLCHAIN_MANIFEST OUTPUT')
    source, archive, manifest_path, output = [Path(value).resolve() for value in sys.argv[1:]]
    require(platform.system() == 'Linux' and platform.machine() == 'x86_64', 'unsupported builder target')
    require(not output.exists(), 'output must be new; no overwrite or install')
    tools_manifest = json.loads(manifest_path.read_text())
    require(re.fullmatch(r'sha256:[0-9a-f]{64}', tools_manifest['builderImage']) is not None, 'qualified immutable builder image digest required')
    tools = {}
    for name, version in PIN['toolchain'].items():
        entry = tools_manifest['tools'][name]
        path = Path(entry['path'])
        require(path.is_absolute() and path.is_file() and not path.is_symlink(), 'exact regular tool file required')
        require(digest(path) == entry['sha256'], 'tool hash mismatch: ' + name)
        require(entry['version'] == version, 'tool version mismatch: ' + name)
        tools[name] = str(path)
    # Git is an input-inspection tool in the qualified builder, not a downloader.
    git = Path(tools_manifest['git']['path'])
    require(git.is_absolute() and digest(git) == tools_manifest['git']['sha256'], 'git input hash mismatch')
    require(subprocess.check_output([str(git), '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip() == PIN['libmobi']['commit'], 'wrong libmobi commit')
    subprocess.run([str(git), '-C', str(source), 'diff', '--exit-code', 'HEAD', '--'], check=True, stdout=subprocess.DEVNULL)
    require(not subprocess.check_output([str(git), '-C', str(source), 'ls-files', '--others', '--exclude-standard']), 'untracked upstream input refused')
    for name, expected in PIN['libmobi']['files'].items():
        require(digest(source / name) == expected, 'upstream file hash mismatch: ' + name)
    require(digest(archive) == PIN['musl']['sha256'], 'musl archive hash mismatch')
    output.mkdir(mode=0o700)
    work = output / 'work'; work.mkdir(mode=0o700)
    env = {'PATH': '/usr/bin:/bin', 'LC_ALL': 'C', 'TZ': 'UTC', 'SOURCE_DATE_EPOCH': '1711929600', 'ZERO_AR_DATE': '1'}
    # All shell/configure utilities come from the recorded immutable Release
    # builder, never fetched by this recipe. The compiler/linker/archiver are exact.
    def run(argv, cwd=None, extra=None):
        subprocess.run(argv, cwd=cwd, env=env | (extra or {}), check=True)
    with tarfile.open(archive) as package:
        require(all(member.name.startswith('musl-1.2.6/') for member in package), 'unexpected musl archive root')
        package.extractall(work, filter='data')
    musl_source = work / 'musl-1.2.6'; musl_build = work / 'musl-build'; musl_build.mkdir()
    musl_prefix = work / 'musl-prefix'
    compiler = shlex.join([tools['clang'], '--target=x86_64-linux-musl', '-fuse-ld=' + tools['ld.lld']])
    run([str(musl_source / 'configure'), '--target=x86_64-linux-musl', '--disable-shared', '--prefix=' + str(musl_prefix)], cwd=musl_build,
        extra={'CC': compiler, 'AR': tools['llvm-ar'], 'RANLIB': tools['llvm-ranlib']})
    run([tools['make'], '-j1'], cwd=musl_build)
    run([tools['make'], 'install'], cwd=musl_build) # private prefix only
    build = work / 'worker-build'
    run([tools['cmake'], '-S', str(HERE), '-B', str(build), '-G', 'Unix Makefiles',
         '-DCMAKE_MAKE_PROGRAM=' + tools['make'], '-DCMAKE_C_COMPILER=' + tools['clang'],
         '-DCMAKE_C_COMPILER_TARGET=x86_64-linux-musl', '-DCMAKE_TRY_COMPILE_TARGET_TYPE=STATIC_LIBRARY',
         '-DCMAKE_AR=' + tools['llvm-ar'], '-DCMAKE_RANLIB=' + tools['llvm-ranlib'],
         '-DLIBMOBI_SOURCE=' + str(source), '-DMUSL_PREFIX=' + str(musl_prefix), '-DPORTICO_LLD=' + tools['ld.lld']])
    run([tools['cmake'], '--build', str(build), '--target', 'portico-mobi-worker', '--parallel', '1'])
    worker = build / 'portico-mobi-worker'
    elf = subprocess.check_output([tools['llvm-readelf'], '--program-headers', '--dynamic-table', str(worker)], text=True)
    require('INTERP' not in elf and '(NEEDED)' not in elf and 'Shared library:' not in elf, 'worker is not a static closure')
    delivery = output / 'delivery'; delivery.mkdir()
    shutil.copy2(worker, delivery / 'worker')
    # Sandbox is an existing Release-qualified Linux helper, not built/downloaded here.
    sandbox = tools_manifest['sandbox']; sandbox_path = Path(sandbox['path'])
    require(sandbox_path.is_absolute() and not sandbox_path.is_symlink() and digest(sandbox_path) == sandbox['sha256'], 'sandbox hash mismatch')
    shutil.copy2(sandbox_path, delivery / 'bwrap')
    bundle = {'schema': 1, 'target': PIN['target'], 'converter': 'libmobi:' + PIN['libmobi']['commit'],
              'options': 'portico-epub-v1;unencrypted-only;no-replica;default-kf8',
              'workerSHA256': digest(delivery / 'worker'), 'sandboxSHA256': digest(delivery / 'bwrap')}
    (delivery / 'manifest.json').write_text(json.dumps(bundle, indent=2) + '\n')
    (delivery / 'manifest.sha256').write_text(digest(delivery / 'manifest.json') + '\n')
    # Matching source and all application objects allow independent rebuild/relink.
    sources = delivery / 'sources'; sources.mkdir()
    with (sources / 'libmobi.tar').open('wb') as stream:
        subprocess.run([str(git), '-C', str(source), 'archive', PIN['libmobi']['commit']], stdout=stream, check=True)
    shutil.copy2(archive, sources / 'musl-1.2.6.tar.gz')
    shutil.copytree(HERE, sources / 'portico-mobi')
    objects = delivery / 'relink-objects'; objects.mkdir()
    for index, obj in enumerate(sorted(build.rglob('*.o'))):
        shutil.copy2(obj, objects / (str(index) + '-' + obj.name))
    for name in ('libc.a', 'crt1.o', 'crti.o', 'crtn.o'):
        shutil.copy2(musl_prefix / 'lib' / name, objects / name)
    shutil.copy2(manifest_path, delivery / 'builder-inputs.json')
    shutil.copy2(musl_source / 'COPYRIGHT', delivery / 'MUSL-COPYRIGHT')
    shutil.copy2(source / 'COPYING', delivery / 'LIBMOBI-COPYING')
    shutil.copy2(source / 'src/miniz.c', delivery / 'MINIZ-NOTICE-AND-SOURCE.c')
    (delivery / 'static-elf.txt').write_text(elf)
    # No version/help/parser process is run by qualification in this recipe.
    # Release independently qualifies the sandbox/worker at its checkpoint.

if __name__ == '__main__':
    main()
