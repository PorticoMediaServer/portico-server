#!/usr/bin/env python3
import hashlib
import json
import os
import pathlib
import runpy
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

ROOT = pathlib.Path(__file__).resolve().parent.parent
RCLONE = runpy.run_path(str(ROOT / "scripts/rclone-bundle.py"))


class PortablePackagingTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.stage = self.root / "stage"
        self.web = self.root / "web"
        self.out = self.root / "out"
        tools = self.root / "tools"
        (self.stage / "third_party/ffmpeg").mkdir(parents=True)
        self.web.mkdir()
        tools.mkdir()
        for name in ["third_party/ffmpeg/toolchain-manifest.json"]:
            (self.stage / name).write_text("fixture")
        (self.web / "index.html").write_text("fixture web")
        binary = self.stage / "portico-server"
        binary.write_text('#!/bin/sh\nprintf "%s\\n" "$PORTICO_WEB_DIR" "$PORTICO_DATA_DIR" "$1"\n')
        binary.chmod(0o700)
        (self.stage / "portico-server.exe").write_text("fixture executable")
        for name in ["nfpm", "makensis"]:
            tool = tools / name
            if name == "makensis":
                tool.write_text("#!/usr/bin/env python3\nimport pathlib,sys\nstage=pathlib.Path(next(a.split('=',1)[1] for a in sys.argv if a.startswith('-DSTAGE_DIR=')))\nassert not (stage/'README.txt').exists(), 'portable README reached installer'\nassert not (stage/'start-portico.cmd').exists(), 'portable launcher reached installer'\n")
            else:
                tool.write_text("#!/bin/sh\nexit 0\n")
            tool.chmod(0o700)
        self.env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ["PATH"])
        self.env.pop("PORTICO_WEB_DIR", None)
        self.env.pop("PORTICO_DATA_DIR", None)
        self.key = Ed25519PrivateKey.generate()
        public = self.root / "pinned-public-key"
        public.write_bytes(self.key.public_key().public_bytes_raw())
        self.env["PORTICO_RELEASE_PUBLIC_KEY_FILE"] = str(public)

    def package(self, platform):
        target = platform + "-amd64"
        rclone = self.stage / "third_party/rclone"
        RCLONE["prepare"](rclone, target, pathlib.Path(
            os.environ.get("PORTICO_RCLONE_TEST_CACHE", ROOT / ".cache/rclone")))
        executable = self.stage / ("portico-server.exe" if platform == "windows" else "portico-server")
        manifest = {"component": "server", "target": target,
                    "artifact": {"path": executable.name,
                                 "sha256": hashlib.sha256(executable.read_bytes()).hexdigest()},
                    "rcloneManifestSha256": hashlib.sha256((rclone / "bundle-manifest.json").read_bytes()).hexdigest()}
        raw = json.dumps(manifest).encode()
        (self.stage / "manifest.json").write_bytes(raw)
        (self.stage / "manifest.sig").write_bytes(self.key.sign(raw))
        subprocess.run(["bash", str(ROOT / "scripts/package-release.sh"), platform, "amd64",
                        "0.1.0", str(self.stage), str(self.web), str(self.out), "1"],
                       env=self.env, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def test_linux_archive_launches_from_unrelated_working_directory(self):
        self.package("linux")
        unpacked = self.root / "unpacked media server"
        unpacked.mkdir()
        with tarfile.open(self.out / "portico-media-server-linux-x64.tar.gz") as archive:
            archive.extractall(unpacked, filter="data")
        launcher = unpacked / "start-portico.sh"
        self.assertTrue(launcher.is_file(), "archive must supply a configured launcher")
        output = subprocess.check_output([str(launcher), "argument"], cwd=self.root,
                                         env=self.env, text=True).splitlines()
        self.assertEqual(output, [str(unpacked / "web"), "", "argument"])
        override = dict(self.env, PORTICO_DATA_DIR=str(self.root / "owner-state"),
                        PORTICO_WEB_DIR=str(self.root / "owner-web"))
        output = subprocess.check_output([str(launcher), "argument"], cwd="/tmp",
                                         env=override, text=True).splitlines()
        self.assertEqual(output, [override["PORTICO_WEB_DIR"], override["PORTICO_DATA_DIR"], "argument"])
        self.assertTrue((unpacked / "README.txt").is_file())

    @unittest.skipUnless(shutil.which("zip"), "zip is unavailable")
    def test_windows_archive_supplies_launcher_and_instructions(self):
        self.package("windows")
        with zipfile.ZipFile(self.out / "Portico-Media-Server-Windows-x64-Portable.zip") as archive:
            self.assertIn("start-portico.cmd", archive.namelist())
            self.assertIn("README.txt", archive.namelist())


if __name__ == "__main__":
    unittest.main()
