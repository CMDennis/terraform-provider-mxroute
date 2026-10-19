import hashlib
import base64
import io
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import privacy_check
from privacy_check import account_match, inspect, load_rules


class PrivacyTests(unittest.TestCase):
    def test_credential_shaped_examples_are_rejected_without_private_rules(self):
        value = ("Mx" + secrets.token_hex(14) + "K1").encode()
        self.assertIn("credential-shaped token", inspect("snapshot.yaml", value, {}))
        self.assertFalse(inspect("snapshot.yaml", b"[API_KEY_FROM_SECRET_STORE]", {}))

    def test_local_undo_trees_still_reject_protected_account_information(self):
        identity = secrets.token_hex(16).encode()
        rules = {len(identity): {hashlib.sha256(identity).hexdigest()}}
        replies = [b"refs/codex/checkpoint\n", b"abc example.txt\n", b"blob\n", identity]
        with patch.object(privacy_check, "git", side_effect=replies):
            self.assertEqual(list(privacy_check.local_snapshot_findings(rules)), ["local snapshot:example.txt"])

    def test_private_identity_and_encoded_variants_never_need_plaintext_rules(self):
        identity = "user" + secrets.token_hex(8)
        encoded = identity.encode()
        variants = [encoded.lower(), base64.b64encode(encoded).lower()]
        rules = {len(v): {hashlib.sha256(v).hexdigest()} for v in variants}
        self.assertTrue(account_match(b"prefix_" + encoded.upper() + b"_suffix", rules))
        self.assertFalse(account_match(b"owner@example.com", rules))
        self.assertIn("protected account information", inspect("example.txt", encoded, rules))
        self.assertTrue(account_match(base64.b64encode(encoded), rules))

    def test_index_content_is_checked_even_if_worktree_is_cleaned(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(["git", "init", "-q", directory], check=True)
            value = secrets.token_hex(16).encode()
            (root / "example.txt").write_bytes(value)
            subprocess.run(["git", "-C", directory, "add", "example.txt"], check=True)
            (root / "example.txt").write_text("sanitized")
            with patch.object(privacy_check, "ROOT", root):
                content = list(privacy_check.staged())
            self.assertEqual(content, [("example.txt", value)])

    def test_full_tag_annotations_are_checked_not_only_subjects(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            def git(*args):
                subprocess.run(["git", "-C", directory, "-c", "user.name=Contributor", "-c", "user.email=contributor@example.com", *args], capture_output=True, check=True)
            git("init", "-q")
            git("commit", "--allow-empty", "-m", "Synthetic privacy fixture")
            identity = secrets.token_hex(16).encode()
            git("tag", "-a", "review-fixture", "-m", "Safe subject\n\n" + identity.decode())
            rules = {len(identity): {hashlib.sha256(identity).hexdigest()}}
            with patch.object(privacy_check, "ROOT", root):
                found = [name for name, data in privacy_check.history() if account_match(data, rules)]
            self.assertEqual(found, ["Git references and tag annotations"])

    def test_missing_required_rules_and_insecure_permissions_fail(self):
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {}, clear=True):
            path = Path(directory) / "rules.json"
            with self.assertRaises(ValueError):
                load_rules(path, True)
            self.assertEqual(load_rules(path, False), {})
            raw = secrets.token_bytes(16)
            path.write_text(json.dumps({"version": 1, "fingerprints": [{"length": 16, "sha256": hashlib.sha256(raw).hexdigest()}]}))
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                load_rules(path, True)
            path.chmod(0o600)
            self.assertTrue(load_rules(path, True))

    def test_runtime_artifacts_and_nested_archives_cannot_bypass_rules(self):
        for name in ["terraform.tfstate", "saved.tfplan", "live.tfvars", ".env", "capture.log", "secret.pem"]:
            self.assertIn("private runtime artifact", inspect(name, b"", {}))
        self.assertIn("live ownership/inventory record", inspect("record.json", b'{"created_by_this_run":true}', {}))
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as out:
            out.writestr("terraform.tfstate", "{}")
        self.assertIn("unsafe archive member", inspect("bundle.zip", archive.getvalue(), {}))

    def test_environment_credentials_only_allow_dynamic_input(self):
        value = secrets.token_hex(16).encode()
        self.assertIn("literal account environment assignment", inspect("example.sh", b"MXROUTE_API_KEY=" + value, {}))
        self.assertFalse(inspect("example.sh", b'MXROUTE_API_KEY="$RUNTIME_SECRET"', {}))


if __name__ == "__main__":
    unittest.main()
