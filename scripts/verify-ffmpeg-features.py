#!/usr/bin/env python3
"""Qualify actual executable inventories against the bundled target contract."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys

TARGETS = {'linux-x64': 'linux-amd64', 'linux-arm64': 'linux-arm64',
           'windows-x64': 'windows-amd64', 'windows-arm64': 'windows-arm64',
           'macos-x64': 'darwin-amd64', 'macos-arm64': 'darwin-arm64'}


def entries(raw, kind):
    result = set()
    for line in raw.splitlines():
        words = line.split()
        if kind in ('hwaccel', 'bsf', 'protocol'):
            if len(words) == 1 and re.fullmatch(r'[a-zA-Z0-9_]+', words[0]):
                result.add(words[0])
        elif len(words) >= 2 and words[1] != '=' and re.fullmatch(r'[.A-Z|]{1,8}', words[0]):
            result.update(words[1].split(','))
    return result


def missing_features(contract, platform, inventories):
    missing = []
    for feature in contract['features']:
        if not feature['required'] or platform not in feature.get('platforms', [platform]):
            continue
        available = inventories[feature['kind']]
        absent = set(feature['acceptable']) - available
        if feature.get('match') == 'any':
            absent = set(feature['acceptable']) if len(absent) == len(feature['acceptable']) else set()
        missing.extend(feature['id'] + ':' + name for name in sorted(absent))
    return missing


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    manifest = json.loads((args.root / 'toolchain-manifest.json').read_text())
    target = manifest['target']
    if target not in TARGETS:
        parser.error('unknown target')
    tool = args.root / 'bin' / ('ffmpeg.exe' if target.startswith('windows') else 'ffmpeg')
    tool = tool.resolve()
    def run(*flags):
        proc = subprocess.run([str(tool), '-hide_banner', *flags], text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=30, check=True)
        return proc.stdout
    raw = {kind: run('-' + flag) for kind, flag in {
        'encoder': 'encoders', 'decoder': 'decoders', 'filter': 'filters',
        'demuxer': 'demuxers', 'muxer': 'muxers', 'protocol': 'protocols',
        'hwaccel': 'hwaccels', 'bsf': 'bsfs'}.items()}
    config, license_text = run('-buildconf'), run('-L')
    lock = json.loads((args.root / 'sources.lock.json').read_text())
    for flag in lock['requiredConfigure']:
        if flag not in config.split():
            raise ValueError('missing required configure flag: ' + flag)
    for flag in lock['prohibitedConfigure']:
        if flag in config.split():
            raise ValueError('prohibited configure flag: ' + flag)
    if not re.search(r'GPL version 3|GPLv3|GNU General Public License.*?version 3', license_text, re.S | re.I):
        raise ValueError('GPLv3 license required')
    inventories = {kind: entries(value, kind) for kind, value in raw.items()}
    inventories['library'] = set(re.findall(r'--enable-([a-z0-9_-]+)', config))
    contract = json.loads((args.root / 'requirements.v1.json').read_text())
    missing = missing_features(contract, TARGETS[target], inventories)
    # soxr must be selectable, not just a configured but unavailable dependency.
    resample = run('-h', 'filter=aresample')
    if not re.search(r'\bsoxr\b', resample):
        missing.append('audio.resampler:soxr')
    report = {'target': target, 'contractId': contract['contractId'], 'version': run('-version').splitlines()[0],
              'sourceCommit': lock['ffmpeg']['commit'], 'missing': missing,
              'inventories': {k: sorted(v) for k, v in inventories.items()}}
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + '\n')
    if missing:
        raise ValueError('missing target features: ' + ', '.join(missing))
    probe = tool.with_name('ffprobe.exe' if tool.suffix == '.exe' else 'ffprobe')
    subprocess.run([str(probe), '-hide_banner', '-version'], check=True, stdout=subprocess.DEVNULL, timeout=10)
    print('Verified required features for ' + target)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, subprocess.SubprocessError) as exc:
        sys.exit(str(exc))
