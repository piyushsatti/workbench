"""The pi marketplace's plugin repos: listed in plugins.json, found as sibling folders of this repo,
or, for an entry with `path`, as that folder inside this repo.

Shared by write_catalogs.py, lint_plugins.py and deploy_agent.py. `PI_SOURCE` overrides the folder that
holds the repos (default: the folder this repo sits in, ~/Source).
"""

import json
import os
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def source_dir():
    return Path(os.environ.get("PI_SOURCE", ROOT.parent))


def entries(root=ROOT):
    """The plugins.json entries: name, github (owner/repo) and sha (null until pushed)."""
    return json.loads((root / "plugins.json").read_text())["plugins"]


def repos(root=ROOT, source=None):
    """(entry, repo folder) for each plugin; the folder may be missing on a machine that hasn't cloned it."""
    source = Path(source) if source else source_dir()
    return [(entry, Path(root) / entry["path"] if "path" in entry else source / entry["name"]) for entry in entries(root)]
