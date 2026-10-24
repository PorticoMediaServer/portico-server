#!/usr/bin/env python3
"""Bind retained Actions model output to reviewed exporter inputs on MAIN."""
import argparse
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parent.parent
INPUTS = ('scripts/prepare-musicnn-model.py', 'scripts/export-musicnn-onnx.py',
          'third_party/musicnn/requirements-export.txt', 'third_party/musicnn/LICENSE.md')


def check(metadata, repository, root=ROOT):
    producer = metadata.get('head_sha', '')
    if (metadata.get('path') != '.github/workflows/sonic-toolchain.yml'
            or metadata.get('event') != 'workflow_dispatch' or metadata.get('head_branch') != 'main'
            or metadata.get('repository', {}).get('full_name') != repository
            or metadata.get('head_repository', {}).get('full_name') != repository
            or not re.fullmatch(r'[0-9a-f]{40}', producer)):
        raise ValueError('retained model must come from this workflow on this repository MAIN')
    ancestry = subprocess.run(['git', 'merge-base', '--is-ancestor', producer, 'HEAD'], cwd=root,
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if ancestry.returncode:
        raise ValueError('retained model producer is not an ancestor of this source')
    for name in INPUTS:
        source = root / name
        if source.is_symlink() or not source.is_file():
            raise ValueError('exporter input is not a regular source file')
        previous = subprocess.run(['git', 'show', producer + ':' + name], cwd=root, check=True,
                                  capture_output=True).stdout
        if previous != source.read_bytes():
            raise ValueError('retained model uses different reviewed exporter inputs')
    name = 'third_party/musicnn/model.lock.json'
    previous = json.loads(subprocess.run(['git', 'show', producer + ':' + name], cwd=root,
                                        check=True, capture_output=True).stdout)
    current = json.loads((root / name).read_text())
    # The only permitted source difference is the explicit output byte pin.
    # Original weights, licence, graph/version and every converter stay fixed.
    for lock in (previous, current):
        lock['onnx'].pop('sha256', None)
        lock['onnx'].pop('bytes', None)
    if previous != current:
        raise ValueError('retained model uses different original weights or graph identity')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('metadata', type=Path)
    parser.add_argument('--repository', required=True)
    args = parser.parse_args()
    try:
        check(json.loads(args.metadata.read_text()), args.repository)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError):
        parser.exit(1, 'Retained Sonic model producer verification failed.\n')
    print('Retained Sonic model producer and original exporter inputs verified.')


if __name__ == '__main__':
    main()
