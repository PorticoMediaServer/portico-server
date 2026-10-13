#!/usr/bin/env python3
import hashlib
import json
import os
import pathlib
import subprocess
import tempfile
import unittest

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

ROOT = pathlib.Path(__file__).resolve().parent.parent


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class MacOSSealTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.payload = self.root / "payload"
        self.tools = self.root / "tools"
        self.tools.mkdir()
        (self.payload / "web").mkdir(parents=True)
        bundle = self.payload / "third_party/ffmpeg"
        (bundle / "bin").mkdir(parents=True)
        (bundle / "LICENSES").mkdir()
        for name in ["bin/ffmpeg", "bin/ffprobe", "corresponding-source.tar.xz", "NOTICE.md",
                     "sources.lock.json", "requirements.v1.json", "LICENSES/GPL.txt"]:
            (bundle / name).write_text("fixture " + name)
        files = {str(p.relative_to(bundle)): digest(p) for p in bundle.rglob("*") if p.is_file()}
        (bundle / "toolchain-manifest.json").write_text(json.dumps({
            "schemaVersion": 1, "licenseMode": "gpl3", "target": "macos-arm64", "files": files}))
        (self.payload / "portico-server").write_text("fixture server")
        self.key = Ed25519PrivateKey.generate()
        self.keyfile = self.root / "release-key"
        self.keyfile.write_bytes(self.key.private_bytes_raw())
        self.keyfile.chmod(0o600)
        manifest = {"schema": 1, "component": "server", "target": "darwin-arm64", "version": "0.1.0",
                    "sourceDigest": "a" * 64, "buildId": "fixture", "officialRoot": "fixture-root",
                    "artifact": {"path": "portico-server", "sha256": digest(self.payload / "portico-server")},
                    "toolchainManifestSha256": digest(bundle / "toolchain-manifest.json")}
        raw = json.dumps(manifest).encode()
        (self.payload / "manifest.json").write_bytes(raw)
        (self.payload / "manifest.sig").write_bytes(self.key.sign(raw))
        self.publicfile = self.root / "pinned-public-key"
        self.publicfile.write_bytes(self.key.public_key().public_bytes_raw())
        self.original_manifest = raw
        self.original_toolchain = (bundle / "toolchain-manifest.json").read_bytes()
        self.sign_log = self.root / "codesign.log"
        codesign = self.tools / "codesign"
        codesign.write_text("""#!/usr/bin/env python3
import os,pathlib,sys
if '--force' in sys.argv:
 p=pathlib.Path(sys.argv[-1])
 with open(os.environ['SIGN_LOG'],'a') as log:log.write(str(p)+'\\n')
 if p.is_file():
  with p.open('ab') as f:f.write(b'OS signature bytes')
 elif os.environ.get('APP_MUTATES_CHILD') == '1':
  with (p/'Contents/Resources/server/portico-server').open('ab') as f:f.write(b'changed after seal')
""")
        codesign.chmod(0o700)
        hdiutil = self.tools / "hdiutil"
        hdiutil.write_text('#!/usr/bin/env python3\nimport pathlib,sys\npathlib.Path(sys.argv[-1]).touch()\n')
        hdiutil.chmod(0o700)
        pkgbuild = self.tools / "pkgbuild"
        pkgbuild.write_text('#!/usr/bin/env python3\nimport pathlib,sys\npathlib.Path(sys.argv[-1]).touch()\n')
        pkgbuild.chmod(0o700)
        prefix = "portico.local/server/internal/buildinfo."
        symbols = {prefix + "SourceDigest": "a" * 64, prefix + "BuildID": "fixture",
                   prefix + "Version": "0.1.0", "portico.local/server/internal/hostedtrust.OfficialRoot": "fixture-root"}
        info = {"Path": "portico.local/server/cmd/server", "Settings": [
            {"Key": "GOOS", "Value": "darwin"}, {"Key": "GOARCH", "Value": "arm64"},
            {"Key": "-tags", "Value": "release"},
            {"Key": "-ldflags", "Value": " ".join("-X " + k + "=" + v for k, v in symbols.items())}]}
        go = self.tools / "go"
        go.write_text("#!/bin/sh\ncat <<'INFO'\n" + json.dumps(info) + "\nINFO\n")
        go.chmod(0o700)
        self.env = dict(os.environ, PATH=str(self.tools) + os.pathsep + os.environ["PATH"],
                        PORTICO_MACOS_SIGNING_IDENTITY="fixture", PORTICO_RELEASE_SIGNING_KEY_FILE=str(self.keyfile), PORTICO_RELEASE_PUBLIC_KEY_FILE=str(self.publicfile),
                        SIGN_LOG=str(self.sign_log))

    def package(self):
        return subprocess.run(["bash", str(ROOT / "scripts/package-macos.sh"), "0.1.0",
                               str(self.payload), str(self.root / "out"), "1"],
                              env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

    def test_codesigned_payload_has_an_authenticated_final_digest(self):
        result = self.package()
        self.assertEqual(result.returncode, 0, result.stderr)
        server = self.root / "out/pkg-root/Applications/Portico Media Server.app/Contents/Resources/server"
        raw = (server / "manifest.json").read_bytes()
        self.key.public_key().verify((server / "manifest.sig").read_bytes(), raw)
        manifest = json.loads(raw)
        self.assertEqual(manifest["artifact"]["sha256"], digest(server / "portico-server"))
        self.assertEqual(manifest["toolchainManifestSha256"], digest(server / "third_party/ffmpeg/toolchain-manifest.json"))
        subprocess.run(["python3", str(ROOT / "scripts/verify-ffmpeg-manifest.py"),
                        str(server / "third_party/ffmpeg")], check=True, stdout=subprocess.PIPE)
        self.assertEqual((server / "source-seal/manifest.json").read_bytes(), self.original_manifest)
        self.assertEqual((server / "source-seal/toolchain-manifest.json").read_bytes(), self.original_toolchain)
        self.key.public_key().verify((server / "source-seal/manifest.sig").read_bytes(), self.original_manifest)

    def test_verification_needs_only_public_material(self):
        result = self.package()
        self.assertEqual(result.returncode, 0, result.stderr)
        public = self.root / "release-public"
        public.write_bytes(self.key.public_key().public_bytes_raw())
        self.keyfile.unlink()
        server = self.root / "out/pkg-root/Applications/Portico Media Server.app/Contents/Resources/server"
        result = subprocess.run(["python3", str(ROOT / "scripts/seal-macos-payload.py"),
                                 str(server), "--public-key", str(public), "--verify"],
                                env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_authenticated_provenance_must_describe_the_same_build(self):
        result = self.package()
        self.assertEqual(result.returncode, 0, result.stderr)
        server = self.root / "out/pkg-root/Applications/Portico Media Server.app/Contents/Resources/server"
        original = server / "source-seal"
        source = json.loads((original / "manifest.json").read_bytes())
        source["buildId"] = "another-authenticated-build"
        raw = json.dumps(source).encode()
        (original / "manifest.json").write_bytes(raw)
        (original / "manifest.sig").write_bytes(self.key.sign(raw))
        final = json.loads((server / "manifest.json").read_bytes())
        for name in ["manifest.json", "manifest.sig"]:
            final["sourceSeal"][name] = digest(original / name)
        raw = json.dumps(final).encode()
        (server / "manifest.json").write_bytes(raw)
        (server / "manifest.sig").write_bytes(self.key.sign(raw))
        result = subprocess.run(["python3", str(ROOT / "scripts/seal-macos-payload.py"),
                                 str(server), "--public-key", str(self.publicfile), "--verify"],
                                env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.assertNotEqual(result.returncode, 0, "a different signed source build was accepted")

    def test_tampered_source_is_rejected_before_codesigning(self):
        (self.payload / "portico-server").write_text("tampered")
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.sign_log.exists(), "unverified input reached the OS signer")

    def test_wrong_release_key_is_rejected_before_codesigning(self):
        self.keyfile.write_bytes(Ed25519PrivateKey.generate().private_bytes_raw())
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.sign_log.exists(), "unknown source seal reached the OS signer")

    def test_app_signing_cannot_change_already_sealed_binaries(self):
        self.env["APP_MUTATES_CHILD"] = "1"
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Server artifact changed", result.stderr)

    def test_final_verification_rejects_changed_provenance(self):
        result = self.package()
        self.assertEqual(result.returncode, 0, result.stderr)
        server = self.root / "out/pkg-root/Applications/Portico Media Server.app/Contents/Resources/server"
        (server / "source-seal/toolchain-manifest.json").write_text("tampered provenance")
        result = subprocess.run(["python3", str(ROOT / "scripts/seal-macos-payload.py"),
                                 str(server), "--public-key", str(self.publicfile), "--verify"],
                                env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Original source seal changed", result.stderr)

    def test_missing_release_key_does_not_generate_a_replacement(self):
        self.keyfile.unlink()
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.keyfile.exists())
        self.assertFalse(self.sign_log.exists())


if __name__ == "__main__":
    unittest.main()
