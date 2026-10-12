#!/usr/bin/env python3
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent


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
        for name in ["manifest.json", "manifest.sig", "third_party/ffmpeg/toolchain-manifest.json"]:
            (self.stage / name).write_text("fixture")
        (self.web / "index.html").write_text("fixture web")
        binary = self.stage / "portico-server"
        binary.write_text('#!/bin/sh\nprintf "%s\\n" "$PORTICO_WEB_DIR" "$PORTICO_STATE_DIR" "$1"\n')
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
        self.env.pop("PORTICO_STATE_DIR", None)

    def package(self, platform):
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
        self.assertEqual(output, [str(unpacked / "web"), str(unpacked / "state"), "argument"])
        override = dict(self.env, PORTICO_STATE_DIR=str(self.root / "owner-state"),
                        PORTICO_WEB_DIR=str(self.root / "owner-web"))
        output = subprocess.check_output([str(launcher), "argument"], cwd="/tmp",
                                         env=override, text=True).splitlines()
        self.assertEqual(output, [override["PORTICO_WEB_DIR"], override["PORTICO_STATE_DIR"], "argument"])
        self.assertTrue((unpacked / "README.txt").is_file())

    @unittest.skipUnless(shutil.which("zip"), "zip is unavailable")
    def test_windows_archive_supplies_launcher_and_instructions(self):
        self.package("windows")
        with zipfile.ZipFile(self.out / "Portico-Media-Server-Windows-x64-Portable.zip") as archive:
            self.assertIn("start-portico.cmd", archive.namelist())
            self.assertIn("README.txt", archive.namelist())


if __name__ == "__main__":
    unittest.main()
