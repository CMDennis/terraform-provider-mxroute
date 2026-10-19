#!/usr/bin/env python3
"""Check Git content without printing secrets; private rules contain hashes only."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_RULES = Path.home() / ".config/mxroute-terraform/privacy-fingerprints.json"
TOKENS = re.compile(rb"[A-Za-z0-9_@.+%=-]+")
PRIVATE_KEY = re.compile(rb"-----BEGIN (?:[A-Z ]*PRIVATE KEY|PGP PRIVATE KEY BLOCK)-----")
KEY_TOKEN = re.compile(rb"\b(?:Mx[a-f0-9]{24,}[a-z0-9]{0,8}|gh[pousr]_[a-z0-9]{30,}|github_pat_[a-z0-9_]{30,}|AKIA[0-9A-Z]{16})\b", re.IGNORECASE)
LIVE_RECORD = re.compile(rb'"(?:created_by_this_run|baseline_configuration_fingerprints|absence_confirmed_before_create)"\s*:')
LITERAL_ENV = re.compile(rb"\bMXROUTE_(?:API_KEY|USERNAME|SERVER)\s*=\s*[\"']?([A-Za-z0-9][A-Za-z0-9._@|:+-]+)")


def git(*args):
    result = subprocess.run(["git", *args], cwd=ROOT, capture_output=True)
    if result.returncode:
        raise ValueError("Git content inspection failed; details withheld")
    return result.stdout


def load_rules(path=None, required=False):
    encoded = os.environ.get("MXROUTE_PRIVACY_FINGERPRINTS")
    target = Path(path or os.environ.get("MXROUTE_PRIVACY_RULES", DEFAULT_RULES))
    if encoded:
        value = json.loads(encoded)
    elif target.is_file():
        if target.stat().st_mode & 0o077:
            raise ValueError("private fingerprint file must be readable only by its owner")
        value = json.loads(target.read_text())
    elif required:
        raise ValueError("private fingerprint rules are required; configure them outside Git")
    else:
        return {}
    if value.get("version") != 1 or not value.get("fingerprints"):
        raise ValueError("invalid or empty private fingerprint rules")
    groups = {}
    for item in value["fingerprints"]:
        n, digest = item.get("length"), item.get("sha256")
        if not isinstance(n, int) or not 4 <= n <= 1024 or not isinstance(digest, str) or not re.fullmatch("[a-f0-9]{64}", digest):
            raise ValueError("invalid private fingerprint entry")
        groups.setdefault(n, set()).add(digest)
    return groups


def account_match(data, rules):
    # Fingerprints are of lowercase byte strings. Sliding windows inside tokens
    # also catch identifiers embedded in paths or configuration strings.
    for token in TOKENS.findall(data.lower()):
        for length, hashes in rules.items():
            for i in range(len(token) - length + 1):
                if hashlib.sha256(token[i:i + length]).hexdigest() in hashes:
                    return True
    return False


def forbidden_path(path):
    name = Path(path).name.lower()
    return (".tfstate" in name or ".tfplan" in name or name in {"crash.log", ".netrc"}
            or name.endswith((".tfvars", ".tfvars.json", ".pem", ".key", ".p12", ".pfx", ".log"))
            or name == ".env" or name.startswith(".env.")
            or any(p in {"private-artifacts", ".secrets", ".terraform"} for p in Path(path).parts))


def inspect(path, data, rules, nested=False):
    reasons = set()
    if forbidden_path(path):
        reasons.add("private runtime artifact")
    if account_match(path.encode() + b"\n" + data, rules):
        reasons.add("protected account information")
    if PRIVATE_KEY.search(data):
        reasons.add("private key material")
    if KEY_TOKEN.search(data):
        reasons.add("credential-shaped token")
    # Source tests can mention the field names; a JSON object artifact containing
    # live-run provenance must never be a repository fixture.
    if path.endswith(".json") and LIVE_RECORD.search(data):
        reasons.add("live ownership/inventory record")
    for match in LITERAL_ENV.finditer(data):
        value = match.group(1)
        if value.lower() not in {b"true", b"false"}:
            reasons.add("literal account environment assignment")
    if not nested and data[:4] == b"PK\x03\x04":
        try:
            with zipfile.ZipFile(io.BytesIO(data)) as archive:
                if sum(f.file_size for f in archive.infolist()) > 256 << 20:
                    reasons.add("archive too large to inspect")
                else:
                    for item in archive.infolist():
                        if inspect(item.filename, archive.read(item), rules, nested=True):
                            reasons.add("unsafe archive member")
        except (ValueError, OSError, zipfile.BadZipFile):
            reasons.add("uninspectable archive")
    return reasons


def worktree():
    names = git("ls-files", "-z", "--cached", "--others", "--exclude-standard").split(b"\0")
    for raw in sorted(set(names)):
        if raw:
            name = os.fsdecode(raw)
            path = ROOT / name
            if path.is_file():
                yield name, path.read_bytes()


def staged():
    names = git("diff", "--cached", "--name-only", "-z", "--diff-filter=ACMR").split(b"\0")
    for raw in names:
        if raw:
            name = os.fsdecode(raw)
            yield name, git("show", ":" + name)


def history():
    # Codex undo trees are local app metadata, not branch/tag history. Audit
    # their protected identities separately; generic checks cover publication.
    for line in git("rev-list", "--objects", "--branches", "--tags", "--remotes", "HEAD").splitlines():
        oid, separator, path = line.partition(b" ")
        if not separator:
            continue
        if git("cat-file", "-t", oid.decode()).strip() == b"blob":
            yield "history:" + os.fsdecode(path), git("cat-file", "blob", oid.decode())
    # Commit/tag text and ref names can also leak identities without a file.
    yield "Git commit metadata", git("log", "--all", "--format=%an <%ae>%n%cn <%ce>%n%B")
    yield "Git references and tag annotations", git("for-each-ref", "--format=%(refname)%0a%(contents)")


def local_snapshot_findings(rules):
    refs = git("for-each-ref", "--format=%(refname)", "refs/codex/").splitlines()
    if not refs:
        return
    for line in git("rev-list", "--objects", *[ref.decode() for ref in refs]).splitlines():
        oid, separator, raw = line.partition(b" ")
        if separator and git("cat-file", "-t", oid.decode()).strip() == b"blob":
            data = git("cat-file", "blob", oid.decode())
            if account_match(raw + b"\n" + data, rules) or PRIVATE_KEY.search(data):
                yield "local snapshot:" + os.fsdecode(raw)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--worktree", action="store_true")
    parser.add_argument("--staged", action="store_true")
    parser.add_argument("--history", action="store_true")
    parser.add_argument("--private-rules")
    parser.add_argument("--require-private-rules", action="store_true")
    parser.add_argument("--artifact-dir", type=Path)
    parser.add_argument("--message-file", type=Path)
    args = parser.parse_args()
    try:
        rules = load_rules(args.private_rules, args.require_private_rules)
        streams = []
        if args.worktree or not (args.staged or args.history or args.artifact_dir or args.message_file):
            streams.append(worktree())
        if args.staged:
            streams.append(staged())
        if args.history:
            streams.append(history())
        if args.message_file:
            streams.append([("Git commit message", args.message_file.read_bytes())])
        if args.artifact_dir:
            streams.append(((str(p.relative_to(args.artifact_dir)), p.read_bytes())
                            for p in args.artifact_dir.rglob("*") if p.is_file()))
        found = False
        if args.history:
            for path in local_snapshot_findings(rules):
                print(f"Privacy check: {path}: protected account information or private key material", file=sys.stderr)
                found = True
        for stream in streams:
            for path, content in stream:
                for reason in sorted(inspect(path, content, rules)):
                    print(f"Privacy check: {path}: {reason}", file=sys.stderr)
                    found = True
        if found:
            return 1
        label = "private account fingerprints + generic checks" if rules else "generic checks only; private account rules unavailable"
        print(f"Privacy review passed ({label}). No sensitive values printed.")
    except (OSError, ValueError, json.JSONDecodeError):
        print("Privacy inspection failed or rules are invalid; details withheld. Do not commit or publish.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
