#!/usr/bin/env python3
"""Generate/check schema docs with runtime credentials removed from the process."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default="v0.25.0")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if shutil.which("terraform") is None:
        parser.error("install Terraform first; documentation generation never auto-downloads it")
    env = {
        k: v for k, v in os.environ.items()
        if not k.startswith(("MXROUTE_", "TF_LOG", "TF_VAR_", "TF_CLI_ARGS"))
        and k not in {"TF_REATTACH_PROVIDERS", "TF_CLI_CONFIG_FILE"}
    }
    def generated_docs():
        return {str(p.relative_to(ROOT)): p.read_bytes()
                for p in (ROOT / "docs").rglob("*.md")
                if p.parent.name != "design"}
    before = generated_docs()
    with tempfile.TemporaryDirectory(prefix="mxroute-docs-") as tmp:
        cli_config = Path(tmp) / "terraform.rc"
        cli_config.write_text("")
        env["TF_CLI_CONFIG_FILE"] = str(cli_config)
        # Generation builds a binary and asks for its schema; it does not
        # configure a client, use credentials, or request any API endpoint.
        subprocess.run([
            "go", "run", f"github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@{args.version}",
            "generate", "--provider-name", "mxroute", "--provider-dir", str(ROOT),
        ], cwd=tmp, env=env, check=True)
    if args.check and before != generated_docs():
        raise SystemExit("Generated documentation changed; review docs/index.md, docs/resources and docs/data-sources and rerun the check.")


if __name__ == "__main__":
    main()
