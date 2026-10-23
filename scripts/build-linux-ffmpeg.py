#!/usr/bin/env python3
"""Repeat the Linux source build with one sealed dependency/compiler input."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work', required=True, type=Path)
    parser.add_argument('--source', required=True, type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    lock = json.loads((root / 'third_party/ffmpeg/sources.lock.json').read_text())
    work = args.work.resolve()
    deps = work / 'linux-deps'
    prefix = deps / 'prefix'
    files = json.loads((deps / 'prefix-files.json').read_text())
    actual = {str(p.relative_to(prefix)): digest(p) for p in sorted(prefix.rglob('*')) if p.is_file()}
    if actual != files:
        raise ValueError('Dependency prefix changed after qualification')
    # ENV values were emitted by the exact recipe generator, never evaluated as
    # shell text on the host. Pass them as environment entries to the compiler.
    dockerfile = (work / 'recipe/Dockerfile').read_text().replace('\\\n', ' ')
    options = {}
    for line in dockerfile.splitlines():
        if line.startswith('ENV '):
            for entry in shlex.split(line[4:]):
                if '=' in entry:
                    key, value = entry.split('=', 1)
                    if key.startswith('FF_'):
                        options[key] = value
    if 'FF_CONFIGURE' not in options:
        raise ValueError('Generated dependency configure flags missing')
    common = ['--enable-gpl', '--enable-version3', '--disable-nonfree', '--disable-libfdk-aac',
              '--disable-debug', '--disable-doc', '--disable-ffplay', '--enable-libdav1d',
              '--enable-libsvtav1', '--enable-libx264', '--enable-libx265', '--enable-libzimg',
              '--enable-libass', '--enable-fontconfig', '--enable-libfreetype', '--enable-libfribidi',
              '--enable-libharfbuzz', '--enable-libsoxr', '--enable-libbluray', '--enable-libdvdnav',
              '--enable-libdvdread', '--enable-libplacebo', '--enable-libopus', '--enable-libvpx',
              '--enable-libzvbi', '--enable-vaapi', '--enable-libdrm', '--enable-libvpl',
              '--enable-opencl', '--enable-vulkan', '--enable-ffnvcodec', '--enable-cuda-llvm',
              '--enable-nvenc', '--enable-nvdec', '--enable-amf']
    options['FF_CONFIGURE'] += ' ' + ' '.join(common)
    source_sha = digest(args.source)
    # Snapshot the small job inputs before either build. Switching the lane's
    # branch to publish an independent checked component cannot change a
    # running container's helper, compiler capture or source lock.
    inputs = {'build-linux-ffmpeg.py': Path(__file__).resolve(),
              'build-ffmpeg.sh': root / 'scripts/build-ffmpeg.sh',
              'configure-ffmpeg-recipe.py': root / 'scripts/configure-ffmpeg-recipe.py',
              'build-portico-disc.py': root / 'scripts/build-portico-disc.py',
              'capture-compiler.py': root / 'scripts/capture-ffmpeg-compiler.py',
              'sources.lock.json': root / 'third_party/ffmpeg/sources.lock.json',
              'portico-disc.c': work / 'recipe/portico-disc/portico-disc.c',
              'LICENSE': work / 'recipe/portico-disc/LICENSE',
              'Dockerfile': work / 'recipe/Dockerfile'}
    hashes = {name: digest(path) for name, path in inputs.items()}
    key = hashlib.sha256(json.dumps(hashes, sort_keys=True).encode()).hexdigest()
    frozen = work / 'linux-build-inputs' / key
    frozen.mkdir(parents=True, exist_ok=True)
    for name, source in inputs.items():
        destination = frozen / name
        if destination.exists():
            if digest(destination) != hashes[name]:
                raise ValueError('Frozen build input changed: ' + name)
        else:
            shutil.copyfile(source, destination)
        if digest(destination) != hashes[name]:
            raise ValueError('Build input changed during capture: ' + name)
    (work / 'linux-build-inputs.json').write_text(json.dumps({'files': hashes,
        'sourceArchiveSHA256': source_sha, 'compilerImage': lock['linuxSourceBuild']['compilerImage'],
        'dependencyPrefixFilesSHA256': digest(deps / 'prefix-files.json'),
        'nativeBuildTools': {'glslc': digest(deps / 'extra/glslc')}}, indent=2) + '\n')
    results = []
    for index in (1, 2):
        build = work / 'linux-compile'
        if build.exists():
            shutil.rmtree(build)
        build.mkdir()
        command = build / 'build.sh'
        command.write_text('''#!/bin/bash
set -euo pipefail
export LC_ALL=C.UTF-8 TZ=UTC ZERO_AR_DATE=1
mkdir -p /ffbuild/source
tar -xf /source.tar.xz -C /ffbuild/source --strip-components=1
cd /ffbuild/source
export CFLAGS="$CFLAGS -ffile-prefix-map=/ffbuild/source=/usr/src/ffmpeg"
export CXXFLAGS="$CXXFLAGS -ffile-prefix-map=/ffbuild/source=/usr/src/ffmpeg"
./configure --prefix=/ffbuild/prefix --pkg-config-flags=--static $FFBUILD_TARGET_FLAGS $FF_CONFIGURE \\
 --extra-cflags="$FF_CFLAGS" --extra-cxxflags="$FF_CXXFLAGS" --extra-libs="$FF_LIBS" \\
 --extra-ldflags="$FF_LDFLAGS" --extra-ldexeflags="$FF_LDEXEFLAGS" \\
 --cc="$CC" --cxx="$CXX" --ar="$AR" --ranlib="$RANLIB" --nm="$NM" \\
 --extra-version=Portico-Jellyfin || { cat ffbuild/config.log; exit 1; }
make -j''' + str(int(os.environ.get('PORTICO_BUILD_JOBS', '2'))) + '''
make install
python3 /capture-compiler.py --lock /sources.lock.json --output /ffbuild/prefix
python3 /build-portico-disc.py --source /disc/portico-disc.c --source-commit "$PORTICO_DISC_SOURCE_COMMIT" \\
 --prefix /opt/ffbuild --output /ffbuild/prefix/bin/portico-disc \\
 --metadata /ffbuild/prefix/portico-disc-build.json --target linux-x64
cp /disc/LICENSE /ffbuild/prefix/portico-disc-LICENSE
cp COPYING.GPLv3 /ffbuild/prefix/COPYING.GPLv3
''')
        argv = ['docker', 'run', '--rm', '--name', 'portico-hardware-r29-ffmpeg',
                '--cpus', '2', '--memory', '4g', '--memory-swap', '4g', '--network', 'none',
                '--label', 'portico.lane=hardware-r29',
                '-v', str(build) + ':/ffbuild', '-v', str(prefix) + ':/opt/ffbuild:ro',
                '-v', str(args.source.resolve()) + ':/source.tar.xz:ro',
                '-v', str(frozen) + ':/disc:ro',
                '-v', str(frozen / 'build-portico-disc.py') + ':/build-portico-disc.py:ro',
                '-v', str(frozen / 'capture-compiler.py') + ':/capture-compiler.py:ro',
                '-v', str(frozen / 'sources.lock.json') + ':/sources.lock.json:ro',
                '-e', 'SOURCE_DATE_EPOCH=' + str(lock['sourceDateEpoch']),
                '-e', 'TMPDIR=/ffbuild/tmp',
                '-e', 'PORTICO_DISC_SOURCE_COMMIT=' + os.environ['PORTICO_DISC_SOURCE_COMMIT']]
        (build / 'tmp').mkdir()
        if (deps / 'extra/glslc').exists():
            argv += ['-v', str(deps / 'extra/glslc') + ':/usr/bin/glslc:ro']
        for key, value in options.items():
            argv += ['-e', key + '=' + value]
        argv += [lock['linuxSourceBuild']['compilerImage'], 'bash', '/ffbuild/build.sh']
        subprocess.run(argv, check=True)
        binaries = {name: digest(build / 'prefix/bin' / name) for name in ('ffmpeg', 'ffprobe', 'portico-disc')}
        results.append({'round': index, 'sourceArchiveSHA256': source_sha, 'binaries': binaries})
        (work / 'repeat-build.json').write_text(json.dumps(results, indent=2) + '\n')
        if index == 1:
            output = work / 'linux-output'
            if output.exists():
                raise ValueError('Bundle output already exists; preserve it explicitly before rebuilding')
            shutil.copytree(build / 'prefix', output)
            shutil.copytree(deps / 'licenses', output / 'LICENSES/dependencies')
            shutil.copyfile(deps / 'dependency-inputs.json', output / 'dependency-inputs.json')
            shutil.copyfile(deps / 'license-inventory.json', output / 'license-inventory.json')
            shutil.copyfile(deps / 'compiler.txt', output / 'compiler.txt')
        elif binaries != results[0]['binaries']:
            raise ValueError('Source-built executable bytes differ between repeat builds')
    shutil.copyfile(work / 'repeat-build.json', work / 'linux-output/repeat-build.json')
    shutil.copyfile(work / 'linux-build-inputs.json', work / 'linux-output/linux-build-inputs.json')
    shutil.rmtree(work / 'linux-compile')
    print('Two source builds produced identical ffmpeg, ffprobe and portico-disc executable hashes')


if __name__ == '__main__':
    main()
