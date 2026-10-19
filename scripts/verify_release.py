#!/usr/bin/env python3
"""Verify exact Registry asset layout, checksums and a separately trusted signer."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile


def verify(directory, version, public_key=None, fingerprint=None, unsigned=False):
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", version):
        raise ValueError("invalid release version")
    prefix = f"terraform-provider-mxroute_{version}"
    archives = {f"{prefix}_{system}_{arch}.zip" for system in ("darwin", "linux", "windows") for arch in ("amd64", "arm64")}
    manifest = prefix + "_manifest.json"
    expected = archives | {manifest}
    sums = directory / (prefix + "_SHA256SUMS")
    listed = {}
    for line in sums.read_text().splitlines():
        match = re.fullmatch(r"([a-f0-9]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] in listed:
            raise ValueError("malformed or duplicate checksum entry")
        listed[match[2]] = match[1]
    if set(listed) != expected:
        raise ValueError("checksum coverage must include all six archives and the versioned manifest")
    for name, digest in listed.items():
        if hashlib.sha256((directory / name).read_bytes()).hexdigest() != digest:
            raise ValueError("release asset checksum mismatch")
    if json.loads((directory / manifest).read_text()) != {"version": 1, "metadata": {"protocol_versions": ["6.0"]}}:
        raise ValueError("unexpected Registry manifest")
    for name in archives:
        binary = f"terraform-provider-mxroute_v{version}" + (".exe" if "_windows_" in name else "")
        with zipfile.ZipFile(directory / name) as archive:
            if sorted(archive.namelist()) != sorted([binary, "LICENSE"]):
                raise ValueError("archive must contain exactly the versioned binary and LICENSE")
            if not archive.getinfo(binary).file_size or not archive.read("LICENSE").startswith(b"MIT License"):
                raise ValueError("empty binary or missing license attribution")
    if unsigned:
        if not version.endswith("-dev"):
            raise ValueError("unsigned verification is restricted to development snapshots")
        return
    if not public_key or not fingerprint or not re.fullmatch(r"[A-Fa-f0-9]{40,64}", fingerprint):
        raise ValueError("a separately trusted public key and full fingerprint are required")
    signature = Path(str(sums) + ".sig")
    if signature.read_bytes().startswith(b"-----BEGIN"):
        raise ValueError("Registry signature must be binary, not ASCII armored")
    # Short paths avoid the UNIX socket length limit in macOS GPG installations.
    with tempfile.TemporaryDirectory(prefix="mxr-sig-", dir="/tmp") as temporary:
        home = Path(temporary)
        home.chmod(0o700)
        result = subprocess.run(["gpg", "--homedir", str(home), "--batch", "--import", str(public_key.resolve())], capture_output=True)
        if result.returncode:
            raise ValueError("public key import failed")
        result = subprocess.run(["gpg", "--homedir", str(home), "--batch", "--status-fd", "1", "--verify", str(signature.resolve()), str(sums.resolve())], capture_output=True)
        wanted = fingerprint.upper().encode()
        valid = [line.split() for line in result.stdout.splitlines() if line.startswith(b"[GNUPG:] VALIDSIG ")]
        # Signing subkeys report their primary fingerprint as the final field.
        if result.returncode or not any(wanted in (line[2], line[-1]) for line in valid):
            raise ValueError("signature does not match the separately trusted signing key")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", required=True, type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--public-key", type=Path)
    parser.add_argument("--fingerprint")
    parser.add_argument("--unsigned-snapshot", action="store_true")
    args = parser.parse_args()
    try:
        verify(args.directory, args.version, args.public_key, args.fingerprint, args.unsigned_snapshot)
    except (OSError, ValueError, KeyError, zipfile.BadZipFile) as error:
        raise SystemExit(f"Release verification failed: {error}") from None
    print("All six platform archives, manifest and checksums verified." if args.unsigned_snapshot else "Release assets and trusted-key signature verified.")


if __name__ == "__main__":
    main()
