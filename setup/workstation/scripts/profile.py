"""Expand a machine profile from defaults.yml into its modules and packages.

Standard library only: bootstrap runs this before any Python package is installed.
defaults.yml is kept to a small YAML subset (described at the top of that file) that
the parser below reads; anything outside the subset is an error, never a guess.

Usage:
  profile.py apt PROFILE          apt package names, in apt.txt order
  profile.py has PROFILE MODULE   exit 0 when the profile includes the module
  profile.py show PROFILE         the whole expansion as JSON
  profile.py check                validate every profile and lock
"""

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULTS = ROOT / "defaults.yml"
DEFAULT_PROFILE = "linux-full"

KEY = re.compile(r"^[A-Za-z0-9_.-]+$")
WORD = re.compile(r"^[A-Za-z0-9_.][A-Za-z0-9_.+:/~@-]*$")
FLOAT = re.compile(
    r"^[-+]?([0-9]*\.[0-9]*|[0-9]+)([eE][-+]?[0-9]+)?$|^\.(inf|nan)$", re.IGNORECASE
)
BOOLEAN_WORDS = {"yes", "no", "on", "off", "true", "false", "y", "n"}


class ProfileError(Exception):
    pass


# ---------------------------------------------------------------------------
# Parser for the defaults.yml subset
# ---------------------------------------------------------------------------


def _strip_comment(text):
    if text.lstrip().startswith("#"):
        return ""
    index = text.find(" #")
    return text if index < 0 else text[:index]


def _scalar(text, where):
    if text == "null" or text == "~":
        return None
    if re.fullmatch(r"-?[0-9]+", text):
        return int(text)
    if FLOAT.match(text) and text != ".":
        raise ProfileError(f"{where}: '{text}' reads as a number in YAML; not allowed")
    if text.lower() in BOOLEAN_WORDS:
        raise ProfileError(f"{where}: '{text}' reads as a boolean in YAML; not allowed")
    if not WORD.match(text) or ": " in text or text.endswith(":"):
        raise ProfileError(
            f"{where}: '{text}' is outside the supported plain-word subset"
        )
    return text


def _value(text, where):
    if text.startswith("["):
        if not text.endswith("]"):
            raise ProfileError(f"{where}: flow lists must close on the same line")
        inner = text[1:-1].strip()
        if not inner:
            return []
        return [_scalar(item.strip(), where) for item in inner.split(",")]
    return _scalar(text, where)


def parse(text, source="defaults.yml"):
    """Parse the subset into dicts, lists, strings, integers and None."""
    lines = []
    for number, raw in enumerate(text.splitlines(), 1):
        where = f"{source}:{number}"
        if "\t" in raw:
            raise ProfileError(f"{where}: tabs are not allowed")
        body = _strip_comment(raw).rstrip()
        if not body.strip():
            continue
        indent = len(body) - len(body.lstrip(" "))
        if indent % 2:
            raise ProfileError(f"{where}: indentation must be a multiple of two spaces")
        lines.append((indent, body.strip(), where))

    position = 0

    def block(indent):
        nonlocal position
        if position >= len(lines) or lines[position][0] != indent:
            raise ProfileError(f"{lines[position - 1][2]}: expected a nested block")
        if lines[position][1].startswith("- ") or lines[position][1] == "-":
            items = []
            while position < len(lines) and lines[position][0] == indent:
                _, content, where = lines[position]
                if not content.startswith("- "):
                    raise ProfileError(f"{where}: mixed list and mapping entries")
                items.append(_scalar(content[2:].strip(), where))
                position += 1
            return items
        mapping = {}
        while position < len(lines) and lines[position][0] == indent:
            _, content, where = lines[position]
            if content.startswith("- "):
                raise ProfileError(f"{where}: mixed list and mapping entries")
            key, sep, rest = content.partition(":")
            if not sep or not KEY.match(key) or (rest and not rest.startswith(" ")):
                raise ProfileError(f"{where}: expected 'key: value'")
            if key in mapping:
                raise ProfileError(f"{where}: duplicate key '{key}'")
            position += 1
            rest = rest.strip()
            if rest:
                mapping[key] = _value(rest, where)
            elif position < len(lines) and lines[position][0] > indent:
                if lines[position][0] != indent + 2:
                    raise ProfileError(f"{lines[position][2]}: indent by two spaces")
                mapping[key] = block(indent + 2)
            else:
                mapping[key] = None
        return mapping

    if not lines:
        raise ProfileError(f"{source}: empty")
    if lines[0][0] != 0:
        raise ProfileError(f"{lines[0][2]}: the document must start at column 0")
    result = block(0)
    if position != len(lines):
        raise ProfileError(f"{lines[position][2]}: unexpected indentation")
    return result


# ---------------------------------------------------------------------------
# Profiles
# ---------------------------------------------------------------------------


def load(path=DEFAULTS):
    data = parse(Path(path).read_text(), Path(path).name)
    if data.get("schema") != 1:
        raise ProfileError("defaults.yml: unsupported schema")
    for section in ("profiles", "modules"):
        if not isinstance(data.get(section), dict) or not data[section]:
            raise ProfileError(f"defaults.yml: '{section}' must be a non-empty mapping")
    return data


def lock_entries(list_path):
    """The package names in apt.txt, one per line; blank lines and # comments are skipped.

    Names only: apt installs whatever version Ubuntu currently offers, so security updates never
    leave the list stale.
    """
    entries = []
    for line in Path(list_path).read_text().splitlines():
        fields = line.split()
        if fields and not fields[0].startswith("#"):
            if "=" in fields[0]:
                raise ProfileError(f"{Path(list_path).name}: '{fields[0]}' has a version; list names only")
            entries.append(fields[0])
    return entries


def _list(module, name, field):
    value = module.get(field, [])
    if value is None:
        return []
    if not isinstance(value, list):
        raise ProfileError(f"module {name}: '{field}' must be a list")
    return value


def expand(name, data=None, root=ROOT):
    data = load() if data is None else data
    profiles, modules = data["profiles"], data["modules"]
    if name not in profiles:
        known = ", ".join(sorted(profiles))
        raise ProfileError(f"Unknown profile '{name}'. Known profiles: {known}")
    profile = profiles[name]
    selected = profile.get("modules") or []
    if len(set(selected)) != len(selected):
        raise ProfileError(f"profile {name}: a module is listed twice")
    for module in selected:
        if module not in modules:
            raise ProfileError(f"profile {name}: unknown module '{module}'")
        for required in _list(modules[module], module, "requires"):
            if required not in selected:
                raise ProfileError(
                    f"profile {name}: module '{module}' requires '{required}'"
                )
    apt_names, owner = [], {}
    tools, harnesses, external, claude_plugins = [], [], [], []
    for module in selected:
        definition = modules[module]
        for package in _list(definition, module, "apt"):
            if package in owner:
                raise ProfileError(
                    f"apt package '{package}' is in both {owner[package]} and {module}"
                )
            owner[package] = module
            apt_names.append(package)
        tools += _list(definition, module, "tools")
        harnesses += _list(definition, module, "harnesses")
        external += _list(definition, module, "external")
        claude_plugins += _list(definition, module, "claude_plugins")
    result = {
        "profile": name,
        "os": profile.get("os"),
        "locks": profile.get("locks"),
        "dotfiles_lock": profile.get("dotfiles_lock"),
        "modules": selected,
        "apt_names": apt_names,
        "apt": None,
        "tools": tools,
        "harnesses": harnesses,
        "external": external,
        "claude_plugins": claude_plugins,
    }
    if profile.get("locks"):
        entries = lock_entries(Path(root) / profile["locks"] / "apt.txt")
        missing = [package for package in apt_names if package not in entries]
        if missing:
            raise ProfileError(
                f"profile {name}: not listed in {profile['locks']}/apt.txt: "
                + ", ".join(missing)
            )
        wanted = set(apt_names)
        # List order, so the full profile reproduces the list exactly.
        result["apt"] = [e for e in entries if e in wanted]
    return result


def check(data=None, root=ROOT):
    """Validate every profile and that each lock entry belongs to exactly one module."""
    data = load() if data is None else data
    problems = []
    for name in data["profiles"]:
        try:
            expand(name, data, root)
        except ProfileError as error:
            problems.append(str(error))
    all_apt = {}
    for module, definition in data["modules"].items():
        for package in _list(definition, module, "apt"):
            if package in all_apt:
                problems.append(
                    f"apt package '{package}' in {all_apt[package]} and {module}"
                )
            all_apt[package] = module
    locks = {p.get("locks") for p in data["profiles"].values() if p.get("locks")}
    for lock in sorted(locks):
        for entry in lock_entries(Path(root) / lock / "apt.txt"):
            if entry.split("=", 1)[0] not in all_apt:
                problems.append(f"{lock}/apt.txt entry '{entry}' belongs to no module")
    if problems:
        raise ProfileError("\n".join(problems))


def main(argv):
    command, args = (argv[0], argv[1:]) if argv else ("", [])
    try:
        if command == "apt" and len(args) == 1:
            packages = expand(args[0])["apt"]
            if packages is None:
                raise ProfileError(
                    f"Profile '{args[0]}' has no package lock; bootstrap.sh cannot install it yet"
                )
            print("\n".join(packages))
        elif command == "has" and len(args) == 2:
            return 0 if args[1] in expand(args[0])["modules"] else 1
        elif command == "show" and len(args) == 1:
            print(json.dumps(expand(args[0]), indent=2))
        elif command == "check" and not args:
            check()
            print("defaults.yml is consistent with every profile lock")
        else:
            print(__doc__.strip(), file=sys.stderr)
            return 2
    except ProfileError as error:
        print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
