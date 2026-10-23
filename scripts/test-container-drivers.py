#!/usr/bin/env python3
"""Build and load the exact runtime-driver stage without compiling the product."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[1]

# Loading userspace libraries is not evidence of a physical device or codec.
PROBE = r'''
import ctypes, json, pathlib, subprocess
required = ['libva2', 'libva-drm2', 'mesa-va-drivers', 'mesa-vulkan-drivers',
            'mesa-opencl-icd', 'libvulkan1', 'ocl-icd-libopencl1',
            'intel-media-va-driver-non-free', 'i965-va-driver', 'libvpl2',
            'libmfx1', 'libmfx-gen1.2']
lock = json.loads(pathlib.Path('/usr/share/doc/portico/intel-drivers.lock.json').read_text())
for item in lock['packages']:
    # The checksum-pinned archive filename has an exact Debian package name.
    required.append(item['name'].split('_', 1)[0])
required = sorted(set(required))
for package in required:
    status = subprocess.check_output(['dpkg-query', '-W', '-f=${Status}', package], text=True)
    assert status == 'install ok installed', (package, status)
    assert (pathlib.Path('/usr/share/doc') / package / 'copyright').stat().st_size > 0, package
for package in ['intel-media-va-driver-non-free'] + [i['name'].split('_', 1)[0] for i in lock['packages']]:
    assert (pathlib.Path('/usr/share/doc/portico/licenses/runtime-drivers') / (package + '.txt')).stat().st_size > 0, package
sources = pathlib.Path('/etc/apt/sources.list.d/debian.sources').read_text()
assert 'Components: main non-free\n' in sources
assert 'contrib' not in sources and 'non-free-firmware' not in sources
for name in ['libva.so.2', 'libva-drm.so.2', 'libvulkan.so.1', 'libOpenCL.so.1',
             'libvpl.so.2', 'libmfx.so.1', 'libmfxhw64.so.1', 'libmfx-gen.so.1.2']:
    ctypes.CDLL(name)
icds = list(pathlib.Path('/etc/OpenCL/vendors').glob('*.icd'))
assert len(icds) >= 3, [str(i) for i in icds]
for icd in icds:
    for name in icd.read_text().splitlines():
        if name.strip() and not name.lstrip().startswith('#'):
            ctypes.CDLL(name.strip())
for icd in pathlib.Path('/usr/share/vulkan/icd.d').glob('*.json'):
    library = json.loads(icd.read_text())['ICD']['library_path']
    ctypes.CDLL(library)
for package in ['intel-media-va-driver-non-free', 'i965-va-driver', 'mesa-va-drivers']:
    paths = subprocess.check_output(['dpkg-query', '-L', package], text=True).splitlines()
    drivers = [p for p in paths if p.endswith('_drv_video.so')]
    assert drivers, package
    for driver in drivers:
        ctypes.CDLL(driver)
print(json.dumps({'installedPackages': len(required), 'openclIcds': len(icds),
                  'userspaceLibrariesLoad': True, 'physicalGpuTested': False}))
'''


def main():
    stage = re.search(r'(?ms)^FROM [^\n]+ AS runtime-drivers\n.*?(?=^FROM runtime-drivers\n)',
                      (ROOT / 'Dockerfile').read_text())
    if not stage:
        raise ValueError('runtime-driver stage missing')
    name = 'portico-hardware-drivers-' + uuid.uuid4().hex[:12]
    label = 'portico.test=' + name
    image = name + ':test'
    with tempfile.TemporaryDirectory(prefix='drivers-', dir=os.environ['TMPDIR']) as temporary:
        context = Path(temporary)
        for relative in ['scripts/install-intel-opencl.py', 'third_party/ffmpeg/intel-drivers.lock.json']:
            destination = context / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / relative, destination)
        lines = stage.group().splitlines()
        lines.insert(1, 'LABEL portico.lane=hardware-r29 ' + label)
        (context / 'Dockerfile').write_text('\n'.join(lines) + '\n')
        try:
            environment = dict(os.environ, DOCKER_BUILDKIT='0')
            subprocess.run(['docker', 'build', '--rm', '--force-rm', '--memory', '1536m',
                            '--memory-swap', '1536m', '--cpu-period', '100000', '--cpu-quota', '200000',
                            '--build-arg', 'TARGETARCH=amd64', '-t', image, str(context)],
                           env=environment, check=True, timeout=1200)
            subprocess.run(['docker', 'run', '--rm', '--name', name, '--label', label,
                            '--label', 'portico.lane=hardware-r29', '--network', 'none',
                            '--cpus', '1', '--memory', '512m', '--memory-swap', '512m',
                            image, 'python3', '-c', PROBE], check=True, timeout=90)
        finally:
            containers = subprocess.check_output(['docker', 'ps', '-aq', '--filter', 'label=' + label], text=True).split()
            if containers:
                subprocess.run(['docker', 'rm', '-f', *containers], check=True)
            images = subprocess.check_output(['docker', 'images', '-aq', '--filter', 'label=' + label], text=True).split()
            if images:
                subprocess.run(['docker', 'rmi', '--no-prune', '-f', *dict.fromkeys(images)], check=True)


if __name__ == '__main__':
    main()
