#!/usr/bin/env python3
"""Compare public upstream bytes without credentials or overwriting the baseline."""
import hashlib
import json
from pathlib import Path
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def main():
    baseline = json.loads((ROOT / "api/baseline.json").read_text())
    # A fresh opener avoids credentials, cookies, .netrc and inherited proxies.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    request = urllib.request.Request("https://api.mxroute.com/openapi.yaml", headers={"User-Agent": "mxroute-provider-public-spec-review"})
    with opener.open(request, timeout=30) as response:
        content = response.read(8 * 1024 * 1024 + 1)
        if response.geturl() != request.full_url or len(content) > 8 * 1024 * 1024:
            raise SystemExit("Unexpected public specification response; review manually.")
    if hashlib.sha256(content).hexdigest() != baseline.get("upstream_sha256", baseline["sha256"]):
        raise SystemExit("Public API specification changed. Follow api/README.md; no accepted snapshot was modified.")
    print("Public API specification matches the accepted baseline. No authenticated requests made.")


if __name__ == "__main__":
    main()
