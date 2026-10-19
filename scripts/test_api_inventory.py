"""Offline regression checks for the Phase 0 scope-freeze tooling."""

import copy
import json
from pathlib import Path
import shutil
import tempfile
import unittest

from api_inventory import ROOT, build_inventory, read_json, render_markdown, validate_fixtures


class InventoryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for directory in ("api", "testdata", "docs"):
            shutil.copytree(ROOT / directory, self.root / directory)

    def write_json(self, path, value):
        (self.root / path).write_text(json.dumps(value))

    def test_baseline_and_generated_outputs_are_exact(self):
        inventory = build_inventory(self.root)
        self.assertEqual(len(inventory["operations"]), 43)
        self.assertEqual(inventory, read_json(ROOT / "api/operations.json"))
        self.assertEqual(render_markdown(inventory), (ROOT / "docs/design/operation-inventory.md").read_text())
        self.assertGreaterEqual(validate_fixtures(self.root, inventory), 31)

    def test_snapshot_tampering_fails_closed(self):
        snapshot = self.root / "api/snapshots/openapi-2026-10-19.yaml"
        snapshot.write_text(snapshot.read_text() + "\n# unexpected change\n")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            build_inventory(self.root)

    def test_unmapped_and_obsolete_operations_fail(self):
        coverage = read_json(self.root / "api/coverage.json")
        moved = coverage.pop("GET /domains")
        coverage["GET /undocumented-bulk-import"] = moved
        self.write_json("api/coverage.json", coverage)
        with self.assertRaisesRegex(ValueError, "unmapped=.*GET /domains.*obsolete=.*undocumented"):
            build_inventory(self.root)

    def test_duplicate_client_mapping_fails(self):
        coverage = read_json(self.root / "api/coverage.json")
        coverage["POST /domains"]["client_method"] = coverage["GET /domains"]["client_method"]
        self.write_json("api/coverage.json", coverage)
        with self.assertRaisesRegex(ValueError, "client methods must be unique"):
            build_inventory(self.root)

    def test_no_invented_schemas_or_lost_nullability(self):
        inventory = build_inventory(self.root)
        rows = {row["key"]: row for row in inventory["operations"]}
        self.assertNotIn("content", rows["POST /reseller/packages"]["responses"]["201"])
        self.assertIn("content", rows["POST /domains"]["responses"]["201"])
        self.assertNotIn("content", rows["DELETE /domains/{domain}"]["responses"]["204"])
        self.assertTrue(inventory["components"]["schemas"]["DnsInfo"]["properties"]["dkim"]["nullable"])
        self.assertEqual(rows["GET /domains"]["parameters"], [])

    def test_fixture_unknown_operation_and_mismatched_request_fail(self):
        inventory = build_inventory(self.root)
        fixtures = read_json(self.root / "testdata/contracts/scenarios.json")
        original = copy.deepcopy(fixtures)
        fixtures["cases"][0]["operation_key"] = "GET /bulk-import"
        self.write_json("testdata/contracts/scenarios.json", fixtures)
        with self.assertRaisesRegex(ValueError, "unknown operation"):
            validate_fixtures(self.root, inventory)
        original["cases"][0]["path"] = "/quota"
        self.write_json("testdata/contracts/scenarios.json", original)
        with self.assertRaisesRegex(ValueError, "does not match"):
            validate_fixtures(self.root, inventory)

    def test_duplicate_fixture_id_and_unknown_issue_fail(self):
        inventory = build_inventory(self.root)
        fixtures = read_json(self.root / "testdata/contracts/scenarios.json")
        original = copy.deepcopy(fixtures)
        fixtures["cases"].append(copy.deepcopy(fixtures["cases"][0]))
        self.write_json("testdata/contracts/scenarios.json", fixtures)
        with self.assertRaisesRegex(ValueError, "duplicate fixture"):
            validate_fixtures(self.root, inventory)
        original["cases"][0]["contract_issue_ids"] = ["C99"]
        self.write_json("testdata/contracts/scenarios.json", original)
        with self.assertRaisesRegex(ValueError, "unknown contract issue"):
            validate_fixtures(self.root, inventory)

    def test_204_body_is_rejected(self):
        inventory = build_inventory(self.root)
        fixtures = read_json(self.root / "testdata/contracts/scenarios.json")
        case = next(case for case in fixtures["cases"] if case["response"]["status"] == 204)
        case["response"]["body"] = {}
        self.write_json("testdata/contracts/scenarios.json", fixtures)
        with self.assertRaisesRegex(ValueError, "204 must have no body"):
            validate_fixtures(self.root, inventory)


if __name__ == "__main__":
    unittest.main()
