import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
import zipfile

from verify_release import verify


class ReleaseVerificationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="mxr-verify-", dir="/tmp")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.version = "0.1.0-beta.1-dev"
        self.prefix = f"terraform-provider-mxroute_{self.version}"
        self.assets = []
        for system in ("darwin", "linux", "windows"):
            for arch in ("amd64", "arm64"):
                path = self.root / f"{self.prefix}_{system}_{arch}.zip"
                with zipfile.ZipFile(path, "w") as archive:
                    archive.writestr(f"terraform-provider-mxroute_v{self.version}" + (".exe" if system == "windows" else ""), b"synthetic binary")
                    archive.writestr("LICENSE", "MIT License\nSynthetic fixture attribution")
                self.assets.append(path)
        manifest = self.root / (self.prefix + "_manifest.json")
        manifest.write_text(json.dumps({"version": 1, "metadata": {"protocol_versions": ["6.0"]}}))
        self.assets.append(manifest)
        self.sums = self.root / (self.prefix + "_SHA256SUMS")
        self.checksum()

    def checksum(self):
        self.sums.write_text("".join(f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n" for p in self.assets))

    def test_exact_archive_and_manifest_coverage(self):
        verify(self.root, self.version, unsigned=True)
        self.assets.pop()
        self.checksum()
        with self.assertRaisesRegex(ValueError, "coverage"):
            verify(self.root, self.version, unsigned=True)

    def test_tampered_archive_and_missing_platform_are_rejected(self):
        self.assets[0].write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            verify(self.root, self.version, unsigned=True)
        self.assets.pop(0)
        self.checksum()
        with self.assertRaisesRegex(ValueError, "coverage"):
            verify(self.root, self.version, unsigned=True)

    def test_embedded_runtime_files_and_wrong_manifest_are_rejected(self):
        with zipfile.ZipFile(self.assets[0], "a") as archive:
            archive.writestr("../terraform.tfstate", "{}")
        self.checksum()
        with self.assertRaisesRegex(ValueError, "exactly"):
            verify(self.root, self.version, unsigned=True)
        self.assets[-1].write_text('{"version":1,"metadata":{"protocol_versions":["5.0"]}}')
        self.checksum()
        with self.assertRaisesRegex(ValueError, "manifest"):
            verify(self.root, self.version, unsigned=True)

    def test_release_never_silently_skips_signatures(self):
        with self.assertRaisesRegex(ValueError, "trusted public key"):
            verify(self.root, self.version)
        with self.assertRaisesRegex(ValueError, "development snapshots"):
            # Match all filenames first, then confirm an ordinary version cannot
            # opt out of signature verification.
            for path in list(self.root.iterdir()):
                path.rename(path.with_name(path.name.replace(self.version, "0.1.0-beta.1")))
            self.assets = [self.root / p.name.replace(self.version, "0.1.0-beta.1") for p in self.assets]
            for path in self.assets:
                if path.suffix == ".zip":
                    with zipfile.ZipFile(path) as old:
                        contents = [(n.replace(self.version, "0.1.0-beta.1"), old.read(n)) for n in old.namelist()]
                    with zipfile.ZipFile(path, "w") as new:
                        for name, content in contents:
                            new.writestr(name, content)
            self.sums = self.root / self.sums.name.replace(self.version, "0.1.0-beta.1")
            self.checksum()
            verify(self.root, "0.1.0-beta.1", unsigned=True)

    @unittest.skipUnless(shutil.which("gpg"), "GPG is required for signature integration coverage")
    def test_signature_requires_the_expected_key(self):
        home = self.root / "gpg"
        home.mkdir(mode=0o700)
        def gpg(*args):
            result = subprocess.run(["gpg", "--homedir", str(home), "--batch", *args], capture_output=True, check=True)
            return result.stdout
        gpg("--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key", "Disposable Offline Release Fixture", "rsa2048", "sign", "1d")
        if shutil.which("gpgconf"):
            self.addCleanup(subprocess.run, ["gpgconf", "--homedir", str(home), "--kill", "gpg-agent"], capture_output=True)
        fingerprint = next(line.split(b":")[9].decode() for line in gpg("--with-colons", "--list-keys").splitlines() if line.startswith(b"fpr:"))
        public = self.root / "public.asc"
        public.write_bytes(gpg("--armor", "--export", fingerprint))
        gpg("--output", str(self.sums) + ".sig", "--detach-sign", str(self.sums))
        verify(self.root, self.version, public, fingerprint)
        with self.assertRaisesRegex(ValueError, "trusted signing key"):
            verify(self.root, self.version, public, "0" * 40)
        self.sums.write_text(self.sums.read_text() + "\n")
        with self.assertRaises(ValueError):
            verify(self.root, self.version, public, fingerprint)


if __name__ == "__main__":
    unittest.main()
