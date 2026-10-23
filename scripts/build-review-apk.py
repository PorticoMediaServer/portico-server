#!/usr/bin/env python3
"""Package a normal embedded-JavaScript arm64 review APK under the Android slot."""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import zipfile


def validate_apk(path):
    with zipfile.ZipFile(path) as apk:
        names = set(apk.namelist())
        arches = {name.split('/')[1] for name in names if name.startswith('lib/') and name.endswith('.so')}
        required = {'assets/index.android.bundle', 'assets/media-engine/THIRD-PARTY-NOTICES.txt',
                    'lib/arm64-v8a/libporticompv.so', 'lib/arm64-v8a/libmpv.so'}
        if arches != {'arm64-v8a'} or not required <= names:
            raise ValueError('arm64 APK must contain its JS, current JNI bridge, mpv and notices')
        if apk.getinfo('assets/index.android.bundle').file_size < 1024:
            raise ValueError('embedded JavaScript bundle is empty')


def prepare_dependencies(project, donor):
    lock = project / 'package-lock.json'
    if hashlib.sha256(lock.read_bytes()).digest() != hashlib.sha256((donor.parent / 'package-lock.json').read_bytes()).digest():
        raise ValueError('native dependency seed lock differs')
    target = project / 'node_modules'
    if target.exists() or target.is_symlink():
        raise FileExistsError('preserve existing dependencies; use a fresh owned release checkout')
    # A physical hard-linked tree keeps Metro's ordinary project roots. An external
    # symlink needs a different watch configuration and must not alter product setup.
    subprocess.run(['cp', '-al', str(donor), str(target)], check=True)
    for base, dirs, _ in os.walk(target, followlinks=False):
        parent = Path(base)
        for name in list(dirs):
            child = parent / name
            generated = name == 'build' and (parent.name in ('android', 'ReactAndroid')
                         or (parent / 'build.gradle').is_file() or (parent / 'build.gradle.kts').is_file())
            if not child.is_symlink() and (name == '.cache' or generated):
                shutil.rmtree(child)
                dirs.remove(name)
    # Postinstall patches may write this package; break only our links first.
    for file in (target / '@react-native-community/blur').rglob('*'):
        if file.is_file() and not file.is_symlink():
            owned = file.with_name(file.name + '.review-copy')
            shutil.copy2(file, owned)
            os.replace(owned, file)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--repo', type=Path, required=True)
    p.add_argument('--seed', type=Path, help='optional dependency seed with the exact native lockfile')
    p.add_argument('--engine', type=Path, required=True)
    p.add_argument('--scratch', type=Path, required=True)
    p.add_argument('--artifacts', type=Path, required=True)
    p.add_argument('--admitted', action='store_true', help=argparse.SUPPRESS)
    a = p.parse_args()
    if not a.admitted:
        slot = Path.home() / '.cache/portico-heavy/run.py'
        result = subprocess.run(['python3', str(slot), '--gradle', '--label', 'release-arm64', '--',
                                 sys.executable, str(Path(__file__).resolve()), *sys.argv[1:], '--admitted'])
        raise SystemExit(result.returncode)
    repo = a.repo.resolve()
    sha = subprocess.check_output(['git', '-C', str(repo), 'rev-parse', 'HEAD'], text=True).strip()
    if subprocess.check_output(['git', '-C', str(repo), 'status', '--porcelain']).strip():
        raise ValueError('native release source must be clean')
    subprocess.run(['git', '-C', str(repo), 'merge-base', '--is-ancestor', sha, 'origin/main'], check=True)
    a.scratch.mkdir(parents=True, exist_ok=True)
    os.environ.update(TMPDIR=str(a.scratch), JAVA_HOME='/usr/lib/jvm/java-17-openjdk-amd64',
                      ANDROID_HOME='/home/justin/Android/Sdk', ANDROID_SDK_ROOT='/home/justin/Android/Sdk')
    # Bound Metro as well as Gradle: Node observes this affinity when sizing workers.
    os.sched_setaffinity(0, sorted(os.sched_getaffinity(0))[:2])
    project = repo / 'apps/native'
    if a.seed:
        prepare_dependencies(project, a.seed.resolve())
    else:
        target = project / 'node_modules'
        if target.exists() or target.is_symlink():
            raise FileExistsError('preserve existing dependencies; use a fresh owned release checkout')
        subprocess.run(['npm', 'ci', '--no-audit', '--no-fund', '--legacy-peer-deps'], cwd=project, check=True)
    for patch in ['patch-apple-blur.mjs', 'patch-android-blur.mjs']:
        subprocess.run(['node', 'scripts/' + patch], cwd=project, check=True)
    subprocess.run(['python3', 'third_party/media-engine/verify.py', 'android', '--out', str(a.engine.parent),
                    '--abis', 'arm64-v8a'], cwd=repo, check=True)
    subprocess.run(['./gradlew', '--no-daemon', '--max-workers=2',
                    '-Dorg.gradle.jvmargs=-Xmx2048m -XX:MaxMetaspaceSize=768m -XX:ActiveProcessorCount=2',
                    '-Dkotlin.compiler.execution.strategy=in-process', '-PreactNativeArchitectures=arm64-v8a',
                    '-PporticoAbiSplits=true',
                    '-PporticoLibmpvDir=' + str(a.engine), ':app:assembleGoogleRelease'], cwd=project / 'android', check=True)
    built = project / 'android/app/build/outputs/apk/google/release/app-google-arm64-v8a-release.apk'
    validate_apk(built)
    a.artifacts.mkdir(parents=True, exist_ok=True)
    destination = a.artifacts / ('portico-arm64-' + sha[:8] + '.apk')
    if destination.exists():
        raise FileExistsError('existing review APK is retained')
    shutil.copy2(built, destination)
    print('Embedded-JavaScript arm64 review APK:', destination)


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode)
