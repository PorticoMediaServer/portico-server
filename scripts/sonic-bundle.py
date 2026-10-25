#!/usr/bin/env python3
"""Build, verify and fetch the pinned Linux Sonic distribution (no installation)."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import struct
import subprocess
import tarfile
import tempfile
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
MAX_ARCHIVE = 128 << 20
MAX_CONTENT = 256 << 20
MAX_MODEL = 64 << 20
# These remain the distribution's ordinary OS runtime, as in the FFmpeg bundle.
SYSTEM = {'libc.so.6', 'libm.so.6', 'libdl.so.2', 'libpthread.so.0', 'librt.so.1',
          'libstdc++.so.6', 'libgcc_s.so.1', 'ld-linux-x86-64.so.2', 'ld-linux-aarch64.so.1'}
TARGETS = {'linux-x64': 62, 'linux-arm64': 183}


def locks():
    return (json.loads((ROOT / 'third_party/portico-sonic/runtime.lock.json').read_text()),
            json.loads((ROOT / 'third_party/musicnn/model.lock.json').read_text()))


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def safe_path(value):
    path = PurePosixPath(value)
    if not value or path.is_absolute() or '..' in path.parts or str(path) != value or '\\' in value:
        raise ValueError('invalid bundle member path')
    return path


def elf(path, target):
    with path.open('rb') as stream:
        header = stream.read(64)
    if (len(header) != 64 or header[:6] != b'\x7fELF\x02\x01'
            or struct.unpack_from('<H', header, 18)[0] != TARGETS[target]):
        raise ValueError('wrong helper/library ELF target')


def model_identity(directory, lock):
    descriptor = json.loads((directory / 'model.json').read_text())
    expected = {'id': lock['modelId'], 'version': lock['onnx']['version'], 'licence': 'ISC',
                'sourceCommit': lock['sourceCommit'], 'format': 'onnx',
                'sha256': lock['onnx']['sha256'], 'bytes': lock['onnx']['bytes'],
                'preprocessing': 'musicnn-logmel-1', 'input': 'log_mel',
                'embedding': 'embedding', 'tags': 'tags', 'frames': 187, 'mels': 96,
                'dimensions': 200, 'tagCount': 50}
    for key, value in expected.items():
        if type(descriptor.get(key)) is not type(value) or descriptor[key] != value:
            raise ValueError('model descriptor identity differs from approved model')
    for name in ('model.onnx', 'model.json', 'LICENSE.md'):
        file = directory / name
        if file.is_symlink() or not file.is_file():
            raise ValueError('model member is not a regular file')
    if ((directory / 'model.onnx').stat().st_size != expected['bytes']
            or digest(directory / 'model.onnx') != expected['sha256']
            or digest(directory / 'LICENSE.md') != lock['licenseSHA256']):
        raise ValueError('model or original ISC licence bytes differ')
    dependencies = descriptor.get('dependencies')
    approved = {'tensorflow-cpu': '2.15.1', 'tf2onnx': '1.16.1', 'onnx': '1.16.2',
                'onnxruntime': '1.20.1', 'numpy': '1.26.4'}
    if dependencies != approved:
        raise ValueError('unapproved model converter provenance')
    return descriptor


def needed(file):
    result = subprocess.run(['readelf', '-d', str(file)], check=True, capture_output=True, text=True)
    return re.findall(r'\(NEEDED\).*?\[([^\]]+)\]', result.stdout)


def package(helper, sdk, model, target, output, source_revision):
    runtime, model_lock = locks()
    if not re.fullmatch(r'[0-9a-f]{40}', source_revision):
        raise ValueError('full source revision required')
    if output.exists():
        raise ValueError('refusing to replace a published bundle')
    if helper.is_symlink() or not helper.is_file():
        raise ValueError('helper must be a regular file')
    elf(helper, target)
    identity = model_identity(model, model_lock)
    sdk_lib = (sdk / 'lib').resolve(strict=True)
    with tempfile.TemporaryDirectory(prefix='sonic-package-', dir=output.parent) as temporary:
        stage = Path(temporary) / 'bundle'
        stage.mkdir()
        shutil.copy2(helper, stage / 'portico-sonic')
        (stage / 'portico-sonic').chmod(0o755)
        external, visited = set(), set()
        pending = needed(helper)
        while pending:
            name = pending.pop()
            if name in visited:
                continue
            visited.add(name)
            if name in SYSTEM:
                external.add(name)
                continue
            if not re.fullmatch(r'libonnxruntime\.so(?:\.[0-9]+)*', name):
                raise ValueError('unapproved non-system runtime dependency')
            file = (sdk_lib / name).resolve(strict=True)
            if file.parent != sdk_lib or not file.is_file():
                raise ValueError('runtime dependency escapes verified SDK')
            elf(file, target)
            lib = stage / 'lib'
            lib.mkdir(exist_ok=True)
            shutil.copy2(file, lib / file.name)
            (lib / file.name).chmod(0o755)
            # Keep only aliases of a library actually in the NEEDED closure.
            for alias in sorted(sdk_lib.iterdir()):
                if alias.is_symlink() and alias.resolve(strict=True) == file:
                    if not re.fullmatch(r'libonnxruntime\.so(?:\.[0-9]+)*', alias.name):
                        raise ValueError('unexpected SDK library alias')
                    (lib / alias.name).symlink_to(file.name)
            pending.extend(needed(file))
        if not (stage / 'lib').is_dir():
            raise ValueError('helper does not use the verified ONNX runtime')
        shutil.copytree(model, stage / 'musicnn-mtt', ignore=shutil.ignore_patterns('*conformance*'))
        if {p.name for p in (stage / 'musicnn-mtt').iterdir()} != {'model.onnx', 'model.json', 'LICENSE.md'}:
            raise ValueError('unexpected model package member')
        legal = stage / 'licenses/sonic'
        legal.mkdir(parents=True)
        for source, name in [(sdk / 'LICENSE', 'onnxruntime-LICENSE'),
                             (sdk / 'ThirdPartyNotices.txt', 'onnxruntime-ThirdPartyNotices.txt'),
                             (ROOT / 'LICENSE', 'portico-GPLv3'),
                             (ROOT / 'THIRD-PARTY-NOTICES.md', 'THIRD-PARTY-NOTICES.md')]:
            shutil.copyfile(source, legal / name)
        if b'Permission is hereby granted, free of charge' not in (legal / 'onnxruntime-LICENSE').read_bytes():
            raise ValueError('original ONNX MIT licence missing')
        sources = ['third_party/portico-sonic/main.cpp', 'third_party/portico-sonic/CMakeLists.txt',
                   'third_party/portico-sonic/README.md', 'third_party/portico-sonic/runtime.lock.json',
                   'scripts/prepare-sonic-runtime.py', 'scripts/build-sonic-runtime.sh']
        for name in sources:
            dest = stage / 'source/portico-sonic' / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / name, dest)
        entries = []
        for file in sorted(stage.rglob('*')):
            if file.is_symlink():
                entries.append({'path': file.relative_to(stage).as_posix(), 'link': os.readlink(file)})
            elif file.is_file():
                # tarfile's safe data filter strips group/other write bits.
                # Seal portable modes before hashing, independent of umask.
                executable = file.name == 'portico-sonic' or file.parent.name == 'lib' or file.name == 'build-sonic-runtime.sh'
                file.chmod(0o755 if executable else 0o644)
                entries.append({'path': file.relative_to(stage).as_posix(), 'bytes': file.stat().st_size,
                                'sha256': digest(file), 'mode': stat.S_IMODE(file.stat().st_mode)})
        manifest = {'schema': 1, 'target': target, 'releaseTag': runtime['releaseTag'],
                    'sourceRevision': source_revision, 'runtimeVersion': runtime['version'],
                    'model': {k: identity[k] for k in ('id', 'version', 'sha256', 'bytes')},
                    'externalSystemLibraries': sorted(external), 'files': entries}
        (stage / 'sonic-manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
        verify(stage, target)
        epoch = int(os.environ.get('SOURCE_DATE_EPOCH', '0'))
        with tarfile.open(output, 'w:xz') as archive:
            for file in sorted(stage.rglob('*')):
                info = archive.gettarinfo(str(file), file.relative_to(stage).as_posix())
                info.uid = info.gid = 0
                info.uname = info.gname = ''
                info.mtime = epoch
                if info.isfile():
                    with file.open('rb') as stream:
                        archive.addfile(info, stream)
                else:
                    archive.addfile(info)


def verify(directory, target, additional_files=()):
    runtime, model_lock = locks()
    manifest_file = directory / 'sonic-manifest.json'
    if manifest_file.is_symlink() or manifest_file.stat().st_size > 1 << 20:
        raise ValueError('invalid Sonic manifest')
    manifest = json.loads(manifest_file.read_text())
    if (manifest.get('schema') != 1 or manifest.get('target') != target
            or manifest.get('releaseTag') != runtime['releaseTag']
            or manifest.get('runtimeVersion') != runtime['version']
            or not re.fullmatch(r'[0-9a-f]{40}', manifest.get('sourceRevision', ''))
            or not set(manifest.get('externalSystemLibraries', [])) <= SYSTEM):
        raise ValueError('wrong Sonic bundle identity')
    paths = set()
    for entry in manifest['files']:
        relative = safe_path(entry['path'])
        if str(relative) in paths:
            raise ValueError('duplicate manifest member')
        paths.add(str(relative))
        file = directory / relative
        # Only local library aliases are permitted; all parents are real dirs.
        if any((directory / PurePosixPath(*relative.parts[:i])).is_symlink() for i in range(1, len(relative.parts))):
            raise ValueError('symlinked bundle parent')
        if 'link' in entry:
            link = entry['link']
            if (relative.parent != PurePosixPath('lib') or not re.fullmatch(r'libonnxruntime\.so(?:\.[0-9]+)*', link)
                    or not file.is_symlink() or os.readlink(file) != link or not file.resolve().is_file()
                    or file.resolve().parent != (directory / 'lib').resolve()):
                raise ValueError('invalid runtime library alias')
        elif (file.is_symlink() or not file.is_file() or file.stat().st_size != entry['bytes']
              or digest(file) != entry['sha256'] or stat.S_IMODE(file.stat().st_mode) != entry['mode']):
            raise ValueError('bundle member changed')
    actual = {p.relative_to(directory).as_posix() for p in directory.rglob('*') if p.is_file() or p.is_symlink()}
    if paths | {'sonic-manifest.json'} | set(additional_files) != actual:
        raise ValueError('unlisted or missing bundle members')
    identity = model_identity(directory / 'musicnn-mtt', model_lock)
    if manifest['model'] != {k: identity[k] for k in ('id', 'version', 'sha256', 'bytes')}:
        raise ValueError('model manifest mismatch')
    elf(directory / 'portico-sonic', target)
    if not os.access(directory / 'portico-sonic', os.X_OK):
        raise ValueError('helper not executable')
    libraries = list((directory / 'lib').glob('*'))
    if not libraries:
        raise ValueError('runtime closure missing')
    for file in libraries:
        if not file.is_symlink():
            elf(file, target)
    for name in ('onnxruntime-LICENSE', 'onnxruntime-ThirdPartyNotices.txt', 'portico-GPLv3', 'THIRD-PARTY-NOTICES.md'):
        if not (directory / 'licenses/sonic' / name).is_file():
            raise ValueError('required legal notice missing')
    return manifest


def verify_installed(ffmpeg, target):
    """Verify both sealed inventories; never edit/extend the FFmpeg manifest.

    Sonic lives beside bin/ffmpeg. A regular hardlink projection lets the unchanged
    standalone FFmpeg verifier check its original inventory, while the final
    exact union below rejects any undeclared composite-package member.
    """
    original = json.loads((ffmpeg / 'toolchain-manifest.json').read_text())
    if original.get('target') != target:
        raise ValueError('Sonic and FFmpeg targets differ')
    names = set(original['files']) | {'toolchain-manifest.json'}
    extra_bin = []
    for name in names:
        safe_path(name)
        if name.startswith('bin/'):
            extra_bin.append(name[4:])
    manifest = verify(ffmpeg / 'bin', target, extra_bin)
    sonic_names = {'bin/' + entry['path'] for entry in manifest['files']} | {'bin/sonic-manifest.json'}
    if names & sonic_names:
        raise ValueError('Sonic collides with sealed FFmpeg members')
    actual = {p.relative_to(ffmpeg).as_posix() for p in ffmpeg.rglob('*') if p.is_file() or p.is_symlink()}
    if actual != names | sonic_names:
        raise ValueError('composite toolchain inventory mismatch')
    with tempfile.TemporaryDirectory(prefix='ffmpeg-inventory-', dir=ffmpeg.parent) as temporary:
        projection = Path(temporary)
        for name in names:
            source = ffmpeg / name
            relative = safe_path(name)
            if (source.is_symlink() or not source.is_file()
                    or any((ffmpeg / PurePosixPath(*relative.parts[:i])).is_symlink() for i in range(1, len(relative.parts)))):
                raise ValueError('FFmpeg member missing or symlinked')
            dest = projection / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            os.link(source, dest, follow_symlinks=False)
        subprocess.run(['python3', str(ROOT / 'scripts/verify-ffmpeg-manifest.py'), str(projection)], check=True,
                       stdout=subprocess.DEVNULL)
    return manifest


def install(bundle, ffmpeg, target):
    manifest = verify(bundle, target)
    if json.loads((ffmpeg / 'toolchain-manifest.json').read_text()).get('target') != target:
        raise ValueError('Sonic and FFmpeg targets differ')
    subprocess.run(['python3', str(ROOT / 'scripts/verify-ffmpeg-manifest.py'), str(ffmpeg)], check=True,
                   stdout=subprocess.DEVNULL)
    files = [entry['path'] for entry in manifest['files']] + ['sonic-manifest.json']
    for name in files:
        dest = ffmpeg / 'bin' / name
        if dest.exists() or dest.is_symlink():
            raise ValueError('Sonic installation would replace an existing member')
    for name in files:
        source, dest = bundle / name, ffmpeg / 'bin' / name
        dest.parent.mkdir(parents=True, exist_ok=True)
        if source.is_symlink():
            dest.symlink_to(os.readlink(source))
        else:
            shutil.copy2(source, dest)
    verify_installed(ffmpeg, target)


def extract(archive, output, target):
    if output.exists() or output.is_symlink():
        raise ValueError('output already exists')
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='sonic-extract-', dir=output.parent) as temporary:
        stage = Path(temporary) / 'bundle'
        stage.mkdir()
        with tarfile.open(archive) as source:
            members = source.getmembers()
            if len(members) > 512 or sum(m.size for m in members) > MAX_CONTENT:
                raise ValueError('unbounded Sonic archive')
            seen = set()
            for member in members:
                path = safe_path(member.name)
                if member.name in seen or not (member.isfile() or member.isdir() or member.issym()):
                    raise ValueError('duplicate or unsupported Sonic archive member')
                seen.add(member.name)
                if member.issym() and (path.parent != PurePosixPath('lib') or not re.fullmatch(r'libonnxruntime\.so(?:\.[0-9]+)*', member.linkname)):
                    raise ValueError('escaping Sonic archive alias')
            source.extractall(stage, filter='data')
        verify(stage, target)
        if output.exists() or output.is_symlink():
            raise ValueError('output appeared during verification')
        stage.rename(output)


def fetch(output, target):
    runtime, _ = locks()
    tag = runtime['releaseTag']
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,100}', tag):
        raise ValueError('invalid release tag')
    name = f'portico-sonic-{tag}-{target}.tar.xz'
    base = 'https://github.com/PorticoMediaServer/portico-server/releases/download/' + tag + '/'
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='sonic-fetch-', dir=output.parent) as temporary:
        with urllib.request.urlopen(base + 'checksums.sha256', timeout=60) as response:
            sums = response.read((64 << 10) + 1).decode('ascii')
        if len(sums) > 64 << 10:
            raise ValueError('unbounded checksum list')
        matches = [line.split()[0] for line in sums.splitlines()
                   if len(line.split()) == 2 and line.split()[1].removeprefix('./') == name]
        if len(matches) != 1 or not re.fullmatch('[0-9a-f]{64}', matches[0]):
            raise ValueError('missing unique bundle checksum')
        archive = Path(temporary) / name
        size = 0
        with urllib.request.urlopen(base + name, timeout=120) as response, archive.open('wb') as stream:
            while block := response.read(1 << 20):
                size += len(block)
                if size > MAX_ARCHIVE:
                    raise ValueError('unbounded Sonic download')
                stream.write(block)
        if digest(archive) != matches[0]:
            raise ValueError('Sonic download checksum mismatch')
        extract(archive, output, target)


def model_release(model_lock):
    tag = model_lock.get('releaseTag')
    if not isinstance(tag, str) or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,100}', tag):
        raise ValueError('invalid model release tag')
    return tag


def model_name(model_lock):
    version = model_lock['onnx']['version']
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,100}', version):
        raise ValueError('invalid model version')
    return 'musicnn-mtt-' + version + '.tar.xz'


def model_reference(model_lock, path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 64 << 10:
        raise ValueError('invalid model conformance reference')
    reference = json.loads(path.read_text())
    if not isinstance(reference, dict) or reference.get('modelSHA256') != model_lock['onnx']['sha256']:
        raise ValueError('conformance reference describes different model bytes')
    return reference


def model_package(output, model, conformance):
    """Publish the exact reviewed model trio once, with its reference and pin."""
    _, model_lock = locks()
    if output.exists() or output.is_symlink():
        raise ValueError('refusing to replace a published model export')
    model_identity(model, model_lock)
    model_reference(model_lock, conformance)
    members = [(f'musicnn-mtt/{name}', model / name, 0o644) for name in ('model.onnx', 'model.json', 'LICENSE.md')]
    members += [('conformance.json', conformance, 0o644), ('model.lock.json', None, 0o644)]
    epoch = int(os.environ.get('SOURCE_DATE_EPOCH', '0'))
    output.parent.mkdir(parents=True, exist_ok=True)
    with tarfile.open(output, 'w:xz') as archive:
        for name, source, mode in members:
            if source is None:
                stream = io.BytesIO(json.dumps(model_lock, indent=2).encode() + b'\n')
                size = stream.getbuffer().nbytes
            else:
                stream, size = source.open('rb'), source.stat().st_size
            info = tarfile.TarInfo(name)
            info.size, info.mode, info.mtime = size, mode, epoch
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            with stream:
                archive.addfile(info, stream)


def model_extract(archive, output):
    """Verify and atomically publish one pinned model export; never install or infer."""
    _, model_lock = locks()
    if output.exists() or output.is_symlink():
        raise ValueError('model export output already exists')
    expected = {f'musicnn-mtt/{name}' for name in ('model.onnx', 'model.json', 'LICENSE.md')}
    expected |= {'conformance.json', 'model.lock.json'}
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='sonic-model-', dir=output.parent) as temporary:
        stage = Path(temporary) / 'export'
        stage.mkdir()
        with tarfile.open(archive) as source:
            members = source.getmembers()
            if len(members) > 16 or sum(member.size for member in members) > MAX_MODEL:
                raise ValueError('unbounded model export archive')
            names = [member.name for member in members]
            if len(names) != len(set(names)) or not all(member.isfile() for member in members):
                raise ValueError('duplicate or unsupported model export member')
            for member in members:
                safe_path(member.name)
            if set(names) != expected:
                raise ValueError('unexpected model export members')
            source.extractall(stage, filter='data')
        model_identity(stage / 'musicnn-mtt', model_lock)
        if json.loads((stage / 'model.lock.json').read_text()) != model_lock:
            raise ValueError('model export provenance differs from the reviewed pin')
        model_reference(model_lock, stage / 'conformance.json')
        if output.exists() or output.is_symlink():
            raise ValueError('model export output appeared during verification')
        stage.rename(output)


def model_fetch(output, base=None):
    """Fetch the durable reviewed model export from its own pinned, non-latest release."""
    _, model_lock = locks()
    tag, name = model_release(model_lock), model_name(model_lock)
    base = (base or 'https://github.com/PorticoMediaServer/portico-server/releases/download/' + tag).rstrip('/') + '/'
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='sonic-model-fetch-', dir=output.parent) as temporary:
        try:
            with urllib.request.urlopen(base + 'checksums.sha256', timeout=60) as response:
                sums = response.read((64 << 10) + 1).decode('ascii')
        except urllib.error.HTTPError as error:
            if error.code == 404:
                raise ValueError('model export is not published yet') from None
            raise
        if len(sums) > 64 << 10:
            raise ValueError('unbounded model checksum list')
        matches = [line.split()[0] for line in sums.splitlines()
                   if len(line.split()) == 2 and line.split()[1].removeprefix('./') == name]
        if len(matches) != 1 or not re.fullmatch('[0-9a-f]{64}', matches[0]):
            raise ValueError('missing unique model export checksum')
        archive = Path(temporary) / name
        size = 0
        with urllib.request.urlopen(base + name, timeout=120) as response, archive.open('wb') as stream:
            while block := response.read(1 << 20):
                size += len(block)
                if size > MAX_MODEL:
                    raise ValueError('unbounded model export download')
                stream.write(block)
        if digest(archive) != matches[0]:
            raise ValueError('model export download checksum mismatch')
        model_extract(archive, output)


def model_verify(directory):
    _, model_lock = locks()
    return model_identity(directory, model_lock)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    for name in ('verify', 'verify-installed', 'install', 'fetch', 'extract', 'package'):
        command = commands.add_parser(name)
        command.add_argument('output', type=Path)
        command.add_argument('--target', choices=TARGETS, required=True)
        if name == 'extract':
            command.add_argument('--archive', type=Path, required=True)
        if name == 'install':
            command.add_argument('--bundle', type=Path, required=True)
        if name == 'package':
            for key in ('helper', 'sdk', 'model'):
                command.add_argument('--' + key, type=Path, required=True)
            command.add_argument('--source-revision', required=True)
    for name in ('model-package', 'model-fetch', 'model-verify'):
        command = commands.add_parser(name)
        command.add_argument('output', type=Path)
        if name == 'model-package':
            command.add_argument('--model', type=Path, required=True)
            command.add_argument('--conformance', type=Path, required=True)
    args = parser.parse_args()
    try:
        if args.command == 'model-package':
            model_package(args.output, args.model, args.conformance)
        elif args.command == 'model-fetch':
            model_fetch(args.output)
        elif args.command == 'model-verify':
            model_verify(args.output)
        elif args.command == 'package':
            args.output.parent.mkdir(parents=True, exist_ok=True)
            package(args.helper, args.sdk, args.model, args.target, args.output, args.source_revision)
        elif args.command == 'extract':
            extract(args.archive, args.output, args.target)
        elif args.command == 'fetch':
            fetch(args.output, args.target)
        elif args.command == 'install':
            install(args.bundle, args.output, args.target)
        elif args.command == 'verify-installed':
            verify_installed(args.output, args.target)
        else:
            verify(args.output, args.target)
    except ValueError as error:
        parser.exit(1, 'Sonic bundle verification failed: ' + str(error) + '\n')
    except (OSError, KeyError, tarfile.TarError, subprocess.CalledProcessError):
        parser.exit(1, 'Sonic bundle verification failed; no bundle published.\n')
    if args.command.startswith('model-'):
        print('Sonic model export ' + args.command.removeprefix('model-') + ' passed.')
    else:
        print('Sonic bundle ' + args.command + ' passed (' + args.target + ').')


if __name__ == '__main__':
    main()
