import hashlib
import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest


spec = importlib.util.spec_from_file_location("prepare_sonic", Path(__file__).with_name("prepare-sonic-runtime.py"))
sdk = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sdk)


class RuntimeStagingTests(unittest.TestCase):
    def fixture(self, root, path="onnxruntime-linux-x64-1.20.1/include/header.h"):
        archive = root / "sdk.tgz"
        with tarfile.open(archive, "w:gz") as target:
            for name, data in [(path, b"synthetic SDK boundary fixture"),
                               ("onnxruntime-linux-x64-1.20.1/LICENSE", b"Permission is hereby granted, free of charge"),
                               ("onnxruntime-linux-x64-1.20.1/ThirdPartyNotices.txt", b"fixture notices")]:
                member = tarfile.TarInfo(name)
                member.size = len(data)
                target.addfile(member, io.BytesIO(data))
        lock = {"version": "1.20.1", "license": "MIT", "packages": {
            "linux-x64": {"sha256": hashlib.sha256(archive.read_bytes()).hexdigest()}}}
        return archive, lock

    def test_changed_unlicensed_or_escaping_sdk_never_publishes(self):
        for kind in ("changed", "licence", "escape"):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                archive, lock = self.fixture(root, "../escape" if kind == "escape" else "onnxruntime-linux-x64-1.20.1/include/header.h")
                if kind == "changed":
                    lock["packages"]["linux-x64"]["sha256"] = "0" * 64
                if kind == "licence":
                    lock["license"] = "non-commercial"
                with self.assertRaises(ValueError):
                    sdk.prepare(archive, root / "installed", "linux-x64", lock)
                self.assertFalse((root / "installed").exists())
                self.assertFalse((root / "escape").exists())

    def test_existing_installation_is_preserved(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive, lock = self.fixture(root)
            output = root / "installed"
            output.mkdir()
            (output / "keep").write_text("original")
            with self.assertRaises(ValueError):
                sdk.prepare(archive, output, "linux-x64", lock)
            self.assertEqual((output / "keep").read_text(), "original")


if __name__ == "__main__":
    unittest.main()
