#!/usr/bin/env python3
"""Install only checksum-pinned official Intel current and legacy OpenCL ICDs."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import urllib.request

lock = json.loads(Path(sys.argv[1]).read_text())
arch = subprocess.check_output(['dpkg', '--print-architecture'], text=True).strip()
if arch != lock['architecture']:
    sys.exit(0)
with tempfile.TemporaryDirectory(prefix='portico-intel-opencl-') as folder:
    packages = []
    names = []
    for item in lock['packages']:
        path = Path(folder) / item['name']
        urllib.request.urlretrieve(item['url'], path)
        with path.open('rb') as stream:
            if hashlib.file_digest(stream, 'sha256').hexdigest() != item['sha256']:
                raise ValueError('Intel driver checksum mismatch: ' + item['name'])
        packages.append(str(path))
        names.append(subprocess.check_output(['dpkg-deb', '--field', str(path), 'Package'], text=True).strip())
    subprocess.run(['apt-get', 'install', '--yes', '--no-install-recommends', *packages], check=True)
    notices = Path('/usr/share/doc/portico/licenses/runtime-drivers')
    notices.mkdir(parents=True, exist_ok=True)
    for name in names:
        copyright = Path('/usr/share/doc') / name / 'copyright'
        if not copyright.is_file() or copyright.stat().st_size == 0:
            raise ValueError('Intel driver copyright missing: ' + name)
        shutil.copyfile(copyright, notices / (name + '.txt'))
