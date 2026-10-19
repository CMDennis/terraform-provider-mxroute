#!/usr/bin/env python3
"""Protect runtime identities using private, owner-only hash rules outside Git."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
from urllib.parse import quote

from privacy_check import DEFAULT_RULES, ROOT


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, help="optional private live-test manifest outside the checkout")
    args = parser.parse_args()
    values = {os.environ[name] for name in ("MXROUTE_SERVER", "MXROUTE_USERNAME", "MXROUTE_API_KEY") if os.environ.get(name)}
    if args.manifest:
        path = args.manifest.resolve()
        if ROOT == path or ROOT in path.parents:
            parser.error("private manifests must remain outside the checkout")
        manifest = json.loads(path.read_text())
        for name in ("domain", "local_part", "email"):
            if isinstance(manifest.get(name), str):
                values.add(manifest[name])
        values.update(name for name in manifest.get("baseline_configuration_fingerprints", {}) if "." in name)
    if not values:
        parser.error("supply authorized identities through runtime environment variables; no values are printed or stored")
    entries = set()
    for value in values:
        for variant in (value, quote(value, safe=""), base64.b64encode(value.encode()).decode()):
            data = variant.lower().encode()
            if 4 <= len(data) <= 1024:
                entries.add((len(data), hashlib.sha256(data).hexdigest()))
    if DEFAULT_RULES.is_file():
        old = json.loads(DEFAULT_RULES.read_text())
        if old.get("version") != 1:
            parser.error("existing rules have an unsupported version; nothing changed")
        entries.update((item["length"], item["sha256"]) for item in old["fingerprints"])
    if not entries:
        parser.error("no identity is long enough to fingerprint safely")
    DEFAULT_RULES.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd = os.open(DEFAULT_RULES, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as out:
        json.dump({"version": 1, "fingerprints": [{"length": n, "sha256": digest} for n, digest in sorted(entries)]}, out)
    DEFAULT_RULES.chmod(0o600)
    print("Private hash-only rules updated outside Git. No account values printed or saved.")


if __name__ == "__main__":
    main()
