#!/usr/bin/env python3
"""Bump or verify release versions in plugin metadata files."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
PLUGIN_NAME = "save-to-spotify"
CODEX_MANIFEST = ROOT / "plugin/.codex-plugin/plugin.json"
CLAUDE_MANIFEST = ROOT / "plugin/.claude-plugin/plugin.json"
ROOT_CLAUDE_MANIFEST = ROOT / ".claude-plugin/plugin.json"

MANIFESTS = [
    CODEX_MANIFEST,
    CLAUDE_MANIFEST,
    ROOT_CLAUDE_MANIFEST,
]

MARKETPLACE = ROOT / ".claude-plugin/marketplace.json"


def relative(path: Path) -> str:
    return str(path.relative_to(ROOT))


def normalize_version(version: str) -> str:
    version = version.strip().removeprefix("refs/tags/").removeprefix("v")
    if not version:
        raise ValueError("version cannot be empty")
    return version


def read_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise FileNotFoundError(f"Required metadata file is missing: {relative(path)}")
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValueError(f"Invalid JSON in {relative(path)}: {exc}") from exc


def write_json(path: Path, data: dict[str, Any]) -> None:
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def check_manifest(path: Path, expected: str) -> str | None:
    actual = read_json(path).get("version")
    if actual != expected:
        return f"{relative(path)}: version {actual!r} does not match {expected!r}"
    return None


def check_marketplace(expected: str) -> list[str]:
    data = read_json(MARKETPLACE)
    plugins = data.get("plugins")
    if not isinstance(plugins, list):
        return [f"{relative(MARKETPLACE)}: missing plugins array"]

    matches = [plugin for plugin in plugins if plugin.get("name") == PLUGIN_NAME]
    if not matches:
        return [f"{relative(MARKETPLACE)}: missing plugin entry for {PLUGIN_NAME!r}"]

    errors = []
    for plugin in matches:
        actual = plugin.get("version")
        if actual != expected:
            errors.append(
                f"{relative(MARKETPLACE)}: {PLUGIN_NAME!r} version {actual!r} "
                f"does not match {expected!r}"
            )
    return errors


def check_claude_manifest_sync() -> list[str]:
    plugin_manifest = read_json(CLAUDE_MANIFEST)
    root_manifest = read_json(ROOT_CLAUDE_MANIFEST)
    if root_manifest != plugin_manifest:
        return [
            f"{relative(ROOT_CLAUDE_MANIFEST)} must match {relative(CLAUDE_MANIFEST)}"
        ]
    return []


def check_versions(version: str) -> list[str]:
    errors = []
    for path in MANIFESTS:
        error = check_manifest(path, version)
        if error:
            errors.append(error)
    errors.extend(check_marketplace(version))
    errors.extend(check_claude_manifest_sync())
    return errors


def bump_versions(version: str) -> None:
    codex_manifest = read_json(CODEX_MANIFEST)
    codex_manifest["version"] = version
    write_json(CODEX_MANIFEST, codex_manifest)

    claude_manifest = read_json(CLAUDE_MANIFEST)
    claude_manifest["version"] = version
    write_json(CLAUDE_MANIFEST, claude_manifest)
    write_json(ROOT_CLAUDE_MANIFEST, claude_manifest)

    marketplace = read_json(MARKETPLACE)
    plugins = marketplace.get("plugins")
    if not isinstance(plugins, list):
        raise ValueError(f"{relative(MARKETPLACE)}: missing plugins array")

    updated = False
    for plugin in plugins:
        if plugin.get("name") == PLUGIN_NAME:
            plugin["version"] = version
            updated = True

    if not updated:
        raise ValueError(f"{relative(MARKETPLACE)}: missing plugin entry for {PLUGIN_NAME!r}")

    write_json(MARKETPLACE, marketplace)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="Release version or tag, for example 0.1.2 or v0.1.2")
    parser.add_argument("--check", action="store_true", help="Verify versions without writing files")
    args = parser.parse_args()

    try:
        version = normalize_version(args.version)
        if args.check:
            errors = check_versions(version)
            if errors:
                for error in errors:
                    print(error)
                return 1
            print(f"Plugin metadata versions match {version}")
        else:
            bump_versions(version)
            print(f"Bumped plugin metadata versions to {version}")
    except (FileNotFoundError, ValueError) as exc:
        print(f"error: {exc}")
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
