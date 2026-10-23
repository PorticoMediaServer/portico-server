#!/usr/bin/env python3
"""Build one pinned Linux bundle in the temporary exact-commit remote runner."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-commit', required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    os.environ['PORTICO_RUNNER_SOURCE_COMMIT'] = args.source_commit
    if root.name != 'source' or root.parent.parent.parent.name != 'jobs':
        raise ValueError('Use only the published temporary exact-commit remote runner')
    job_root, host_work = root.parent, root.parents[3]
    scratch = job_root / '.scratch/hardware-r29'
    scratch.mkdir(parents=True)
    lock = json.loads((root / 'third_party/ffmpeg/sources.lock.json').read_text())
    env = dict(os.environ, PORTICO_LANE_SCRATCH=str(scratch), PORTICO_BUILD_JOBS='2',
        PORTICO_DISC_SOURCE_COMMIT=lock['discHelper']['sourceCommit'],
        PORTICO_DISC_FIXTURES=str(root / lock['discHelper']['fixtureManifest']),
        PORTICO_DISC_FIXTURES_SHA256=lock['discHelper']['fixtureManifestSHA256'],
        TMPDIR=str(scratch / 'tmp'), GOTMPDIR=str(scratch / 'go-tmp'))
    for directory in ('tmp', 'go-tmp'):
        (scratch / directory).mkdir()
    fs = os.statvfs(job_root)
    available = int(next(line.split()[1] for line in Path('/proc/meminfo').read_text().splitlines()
                         if line.startswith('MemAvailable:'))) * 1024
    # This is this job's actual phase preflight, in addition to the shared
    # runner. It is not a retry loop or a separate admission mechanism.
    disk_peak, ram_cap = 18 * 2**30, 4 * 2**30
    preflight = {'host': os.uname().nodename, 'sourceCommit': args.source_commit,
        'availableDiskBytes': fs.f_bavail * fs.f_frsize, 'availableRAMBytes': available,
        'additionalDiskPeakBytes': disk_peak, 'containerRAMCapBytes': ram_cap,
        'compilerWorkers': 2, 'scratch': str(scratch),
        'retainedArtifactRoot': str(host_work / 'artifacts/hardware-r29'),
        'scope': 'One source-built Linux-x64 target; no physical GPU or other-target claim'}
    print(json.dumps({'preflight': preflight}, indent=2), flush=True)
    if preflight['availableDiskBytes'] < disk_peak + 20 * 2**30 or available < ram_cap:
        print('REFUSED: this FFmpeg job peak does not fit; continue code-only', flush=True)
        return 75
    subprocess.run(['python3', str(root / 'scripts/check-ffmpeg-source.py'),
                    '--archive-commit', args.source_commit, '--archive-rejections'],
                   cwd=root, env=env, check=True)
    # Verify capability before a lengthy source dependency intake. No sudo,
    # daemon configuration, host package installation or private auth is used.
    subprocess.run(['docker', 'version', '--format', '{{.Server.Version}}'], check=True)
    subprocess.run(['bash', str(root / 'scripts/build-ffmpeg.sh'), 'linux-x64'],
                   cwd=root, env=env, check=True)
    work = scratch / 'ffmpeg-build/linux-x64'
    archive = work / 'portico-ffmpeg-linux-x64.tar.xz'
    manifest = work / 'bundle/toolchain-manifest.json'
    if not archive.is_file() or not manifest.is_file():
        raise ValueError('Successful canonical build did not produce the required bundle')
    artifact_parent = host_work / 'artifacts/hardware-r29' / args.source_commit
    artifact_parent.mkdir(parents=True, exist_ok=True)
    destination = artifact_parent / job_root.name
    staging = destination.with_name(destination.name + '.new')
    staging.mkdir()
    try:
        # This is the requested retained product artifact, not the disposable
        # source/tmp tree. Preserve one payload only and return text by reference.
        archive.rename(staging / archive.name)
        shutil.copyfile(manifest, staging / 'toolchain-manifest.json')
        reports = [work / name for name in ('feature-report-bundle.json', 'disc-report-bundle.json',
                   'runtime-report-bundle.json', 'abi-report-bundle.json', 'repeat-build.json')]
        reports += [work / 'linux-output' / name for name in
                    ('dovi-profile-build.json', 'dovi-video-build.json')]
        for path in reports:
            shutil.copyfile(path, staging / path.name)
        hashes = {p.name: digest(p) for p in sorted(staging.iterdir()) if p.is_file()}
        receipt = {'schema': 1, 'host': os.uname().nodename,
            'at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'buildSourceCommit': args.source_commit, 'ffmpegSourceCommit': lock['ffmpeg']['commit'],
            'artifactPath': str(destination / archive.name), 'files': hashes,
            'qualifiedTargetVerification': True, 'mainConsumerAdmission': False,
            'retention': 'Retain this verified product until explicit consumer release; per-job source/tmp is disposable.',
            'scope': 'One Linux source bundle/generated fixtures/ABI/licences/repeat proof. No physical GPU, real Atmos/JOC, mastered FEL, other targets or full HW1-HW7 completion.',
            'consumerFence': 'This exact product source must be normally merged on MAIN before feature consumers use the artifact.'}
        (staging / 'artifact-receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
        staging.rename(destination)
    except BaseException:
        # A partial artifact was never announced or handed to a consumer.
        shutil.rmtree(staging)
        raise
    print(json.dumps({'retainedVerifiedArtifact': receipt}, indent=2), flush=True)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
