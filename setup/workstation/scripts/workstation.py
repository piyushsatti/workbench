"""Unprivileged workstation installation and verification.

The root launcher installs system packages, then runs this as SUDO_USER.
Only explicitly managed files are replaced; their old contents are recoverable.
"""

import grp
import hashlib
import importlib.util
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PROFILE = ROOT / "profiles/ubuntu-26.04"
HOME = Path.home()
STATE = HOME / ".local/state/workstation"
DATA = HOME / ".local/share/workstation"
VERSIONS = json.loads((PROFILE / "versions.json").read_text())
EXTENSIONS = json.loads((PROFILE / "extensions.json").read_text())
RUN = None
REPORT = {"status": "running", "stages": [], "pending": [], "failed": []}


def _load_profiles():
    # Loaded by path: the module name `profile` would otherwise resolve to the stdlib one.
    spec = importlib.util.spec_from_file_location(
        "workstation_profiles", Path(__file__).resolve().with_name("profile.py")
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


PROFILES = _load_profiles()
SELECTED = None


def select_profile(name=None):
    """Expand the profile from defaults.yml: argument, then $WORKSTATION_PROFILE, then default."""
    global SELECTED
    name = name or os.environ.get("WORKSTATION_PROFILE") or PROFILES.DEFAULT_PROFILE
    expansion = PROFILES.expand(name, root=ROOT)
    if expansion["locks"] != str(PROFILE.relative_to(ROOT)):
        raise RuntimeError(
            f"Profile {name} has no Ubuntu 26.04 lock; it cannot be installed here"
        )
    SELECTED = expansion
    REPORT["profile"] = name
    return expansion


def selected():
    """The selected profile's expansion, selecting the default one on first use."""
    return SELECTED or select_profile()


def has(module):
    return module in selected()["modules"]


def dotfiles_locked():
    """Whether this profile applies a locked Dotfiles render."""
    return has("shell") and selected()["dotfiles_lock"] is not None


def dotfiles_lock():
    """The selected profile's Dotfiles lock: its exact targets and their rendered digest."""
    return json.loads((ROOT / selected()["dotfiles_lock"]).read_text())


def run(*args, cwd=None, capture=False, timeout=300, env=None):
    return subprocess.run(
        [str(a) for a in args],
        cwd=cwd,
        env=env,
        check=True,
        timeout=timeout,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
    ).stdout


def save_report():
    path = STATE / "report.json"
    if STATE.is_symlink():
        raise RuntimeError(
            f"Refusing to write report through symlinked state directory: {STATE}"
        )
    if not STATE.resolve().is_relative_to(HOME.resolve()):
        raise RuntimeError(f"Refusing to write report outside target home: {STATE}")
    if path.is_symlink():
        raise RuntimeError(f"Refusing to write report through symlink: {path}")
    STATE.mkdir(parents=True, exist_ok=True)
    fd, temp = tempfile.mkstemp(prefix=".report-", dir=STATE)
    try:
        with os.fdopen(fd, "w") as stream:
            stream.write(json.dumps(REPORT, indent=2) + "\n")
        os.chmod(temp, 0o600)
        os.replace(temp, path)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def stage(name, action):
    print(f"\n[{name}]", flush=True)
    REPORT["current_stage"] = name
    save_report()
    action()
    REPORT["stages"].append(name)
    save_report()


def backup(path):
    """Copy exactly one managed entry, retaining symlinks as symlinks."""
    if not path.exists() and not path.is_symlink():
        return
    if RUN is None:
        raise RuntimeError("Recovery directory is not initialized")
    dest = RUN / "files" / path.relative_to(HOME)
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() or dest.is_symlink():
        return
    if path.is_dir() and not path.is_symlink():
        shutil.copytree(path, dest, symlinks=True)
    else:
        shutil.copy2(path, dest, follow_symlinks=False)


def write(path, content, mode=0o644, seed=False):
    if not path.parent.resolve().is_relative_to(HOME.resolve()):
        raise RuntimeError(f"Managed parent resolves outside the target home: {path}")
    if seed and (path.exists() or path.is_symlink()):
        return
    payload = content.encode() if isinstance(content, str) else content
    if path.is_file() and not path.is_symlink() and path.read_bytes() == payload:
        return
    backup(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temp = tempfile.mkstemp(prefix=".workstation-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(payload)
        os.chmod(temp, mode)
        os.replace(temp, path)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def resolve_dotfiles_source():
    """Find the canonical sibling checkout or an explicit test/build source."""
    override = os.environ.get("WORKSTATION_DOTFILES_SOURCE")
    candidates = []
    if override:
        candidates.append(Path(override).expanduser())
    else:
        candidates.extend((ROOT.parent / "dotfiles", ROOT.parent.parent / "dotfiles"))
    for candidate in candidates:
        candidate = candidate.resolve()
        if (
            (candidate / ".chezmoiignore.tmpl").is_file()
            and (candidate / "dot_bashrc.tmpl").is_file()
            and (candidate / "dot_tmux.conf.tmpl").is_file()
        ):
            return candidate
    searched = ", ".join(str(path) for path in candidates)
    raise RuntimeError(f"Canonical Dotfiles source not found; checked: {searched}")


def rendered_dotfiles_digest(rendered):
    digest = hashlib.sha256()
    for name in sorted(rendered):
        content, mode = rendered[name]
        digest.update(name.encode())
        digest.update(b"\0")
        digest.update(f"{mode:o}".encode())
        digest.update(b"\0")
        digest.update(content)
        digest.update(b"\0")
    return digest.hexdigest()


def chezmoi_binary():
    bundled = HOME / ".local/bin/chezmoi"
    return bundled if os.access(bundled, os.X_OK) else shutil.which("chezmoi")


def chezmoi_owns_dotfiles():
    """The chezmoi binary when the user's chezmoi is initialised from this repo's Dotfiles source, else None.

    chezmoi then owns the dotfiles: the install leaves them alone and verify checks only that chezmoi has nothing
    to apply. The locked render is for machines where chezmoi is not set up from this source."""
    binary = chezmoi_binary()
    if not binary:
        return None
    result = subprocess.run(
        [str(binary), "source-path"], env=dict(os.environ, HOME=str(HOME)),
        capture_output=True, text=True, timeout=60, check=False,
    )
    if result.returncode or not result.stdout.strip():
        return None
    try:
        ours = resolve_dotfiles_source()
    except RuntimeError:
        return None
    return Path(binary) if Path(result.stdout.strip()).resolve() == ours else None


def render_dotfiles():
    """Render and lock the exact Ubuntu targets without applying Chezmoi state."""
    lock = dotfiles_lock()
    source = resolve_dotfiles_source()
    binary = chezmoi_binary()
    if not binary:
        raise RuntimeError("Chezmoi is required to render canonical Dotfiles")
    targets = tuple(lock["targets"])
    data = {
        "chezmoi": {"os": "linux"},
        "workstation": {"profile": lock["profile"]},
    }
    archive = subprocess.run(
        [
            str(binary),
            "--source",
            str(source),
            "--destination",
            str(HOME),
            "--config",
            "/dev/null",
            "--config-format",
            "toml",
            "--override-data",
            json.dumps(data),
            "archive",
            *(str(HOME / target) for target in targets),
        ],
        check=True,
        stdout=subprocess.PIPE,
        timeout=120,
        # chezmoi applies the process umask to the modes it renders; install runs under 077, and the
        # lock records the modes the source declares, so render under the usual 022.
        umask=0o022,
    ).stdout
    rendered = {}
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:*") as bundle:
        for member in bundle.getmembers():
            if member.isdir():
                continue
            rel = Path(member.name)
            if rel.is_absolute() or ".." in rel.parts or not member.isfile():
                raise RuntimeError(f"Unexpected rendered entry: {member.name}")
            if member.name in rendered:
                raise RuntimeError(f"Duplicate rendered entry: {member.name}")
            rendered[member.name] = (
                bundle.extractfile(member).read(),
                member.mode & 0o777,
            )
    if set(rendered) != set(targets):
        missing = sorted(set(targets) - set(rendered))
        extra = sorted(set(rendered) - set(targets))
        raise RuntimeError(
            f"Dotfiles target mismatch; missing={missing}, extra={extra}"
        )
    digest = rendered_dotfiles_digest(rendered)
    if digest != lock["rendered_sha256"]:
        raise RuntimeError(
            f"Canonical Dotfiles render differs from the Ubuntu profile lock: {digest}"
        )
    return source, rendered, digest


def install_tree(source, target, revision):
    if not target.parent.resolve().is_relative_to(HOME.resolve()):
        raise RuntimeError(f"Managed parent resolves outside the target home: {target}")
    marker = target / ".workstation-revision"
    if marker.is_file() and marker.read_text().strip() == revision:
        return
    target.parent.mkdir(parents=True, exist_ok=True)
    if target.exists() or target.is_symlink():
        dest = RUN / "files" / target.relative_to(HOME)
        dest.parent.mkdir(parents=True, exist_ok=True)
        if dest.exists() or dest.is_symlink():
            raise RuntimeError(f"Recovery collision at {dest}")
        target.rename(dest)
    shutil.copytree(
        source, target, symlinks=True, ignore=shutil.ignore_patterns(".git")
    )
    marker.write_text(revision + "\n")


def download(url, target, checksum=None):
    target.parent.mkdir(parents=True, exist_ok=True)
    if not target.exists():
        temp = target.with_suffix(target.suffix + ".part")
        run(
            "curl",
            "--fail",
            "--silent",
            "--show-error",
            "--location",
            "--retry",
            "3",
            "--connect-timeout",
            "20",
            "--max-time",
            "300",
            url,
            "--output",
            temp,
            timeout=1000,
        )
        temp.rename(target)
    if checksum and hashlib.sha256(target.read_bytes()).hexdigest() != checksum:
        raise RuntimeError(f"Checksum mismatch: {target}")
    return target


def checkout(name, url, commit):
    target = DATA / "sources" / name / commit
    if (target / ".git").exists():
        try:
            head = run("git", "rev-parse", "HEAD", cwd=target, capture=True).strip()
        except subprocess.CalledProcessError:
            head = None
        if head != commit:
            if RUN is None:
                raise RuntimeError("Recovery directory is not initialized")
            recovery = RUN / "incomplete-checkouts" / name / commit
            recovery.parent.mkdir(parents=True, exist_ok=True)
            if recovery.exists() or recovery.is_symlink():
                raise RuntimeError(f"Recovery collision at {recovery}")
            target.rename(recovery)
    if (target / ".git").exists():
        if run(
            "git",
            "status",
            "--porcelain",
            "--untracked-files=no",
            cwd=target,
            capture=True,
        ):
            raise RuntimeError(f"Modified pinned source: {target}")
        return target
    target.mkdir(parents=True, exist_ok=True)
    run("git", "init", "--quiet", target)
    run("git", "fetch", "--quiet", "--depth=1", url, commit, cwd=target)
    run("git", "checkout", "--quiet", "--detach", "FETCH_HEAD", cwd=target)
    if run("git", "rev-parse", "HEAD", cwd=target, capture=True).strip() != commit:
        raise RuntimeError(f"Upstream revision mismatch: {name}")
    return target


def tools():
    chezmoi_tool()
    if has("agents"):
        harness_clis()
    if has("dev"):
        uv_tool()
    if has("shell"):
        for name, command in (("fd", "fdfind"), ("bat", "batcat")):
            write(
                HOME / ".local/bin" / name,
                f'#!/bin/sh\nexec /usr/bin/{command} "$@"\n',
                0o755,
            )


def chezmoi_tool():
    """Install the pinned, checksum-verified chezmoi when it is missing; an installed one is kept."""
    path = cli_path("chezmoi")
    if path:
        keep("chezmoi", path)
        return
    # The digest is pinned in versions.json, not read from a checksums file of the same release.
    machine = os.uname().machine
    architecture = {"x86_64": "amd64", "aarch64": "arm64"}.get(machine)
    checksum = VERSIONS["chezmoiSha256"].get(architecture)
    if not checksum:
        raise RuntimeError(f"There is no pinned chezmoi for {machine}; add its SHA-256 to versions.json")
    version = VERSIONS["chezmoi"]
    asset = f"chezmoi_{version}_linux_{architecture}.tar.gz"
    url = f"https://github.com/twpayne/chezmoi/releases/download/v{version}"
    archive = download(f"{url}/{asset}", DATA / "cache" / asset, checksum)
    with tarfile.open(archive) as tar:
        write(HOME / ".local/bin/chezmoi", tar.extractfile("chezmoi").read(), 0o755)


def cli_path(name):
    """Where an installed CLI is, or None: ~/.local/bin first, then PATH. A dangling symlink is not one."""
    local = HOME / ".local/bin" / name
    if local.exists():
        return local
    found = shutil.which(name)
    return Path(found) if found else None


def keep(name, path):
    """An installed CLI is the user's: it is left as is and updates itself."""
    note = f"Kept the installed {name} at {path}; it updates itself."
    print(note)
    REPORT.setdefault("kept", []).append(note)


# npm package for each agent CLI the agents module can install when it is missing.
HARNESS_PACKAGES = {"claude": "@anthropic-ai/claude-code", "codex": "@openai/codex", "opencode": "opencode-ai"}


def harnesses():
    """The agent CLIs the selected profile lists under `harnesses:` in defaults.yml."""
    names = selected()["harnesses"]
    unknown = [name for name in names if name not in HARNESS_PACKAGES]
    if unknown:
        raise RuntimeError(f"No npm package known for harness: {', '.join(unknown)}")
    return names


def npm_wrapper(target):
    """The ~/.local/bin script this installer writes for an agent CLI it installed with npm."""
    return f'#!/bin/sh\nexec "{target}" "$@"\n'


def harness_clis():
    """Install the latest agent CLIs that are missing; never replace one already installed. Our own wrapper
    whose npm install is gone (say ~/.local/share/workstation was cleared) counts as missing, so it is repaired."""
    npm_dir = DATA / "npm"
    missing = []
    for name in harnesses():
        path = cli_path(name)
        target = npm_dir / "node_modules/.bin" / name
        ours_broken = (path == HOME / ".local/bin" / name and not target.exists()
                       and path.read_text(errors="replace") == npm_wrapper(target))
        if path and not ours_broken:
            keep(name, path)
        else:
            missing.append(name)
    if not missing:
        return
    npm_dir.mkdir(parents=True, exist_ok=True)
    # Native-package postinstall scripts run only as the workstation user.
    run("npm", "install", "--no-audit", "--no-fund", "--prefix", npm_dir,
        *(f"{HARNESS_PACKAGES[name]}@latest" for name in missing), timeout=900)
    for name in missing:
        target = npm_dir / "node_modules/.bin" / name
        if not target.exists():
            raise RuntimeError(f"Missing installed CLI: {name}")
        write(HOME / ".local/bin" / name, npm_wrapper(target), 0o755)


def uv_tool():
    """Install the latest uv when it is missing; an installed one is kept."""
    path = cli_path("uv")
    if path:
        keep("uv", path)
    else:
        run("pipx", "install", "uv", timeout=600)


# Where each Claude marketplace named in claude_plugins comes from, for a machine that lacks it.
CLAUDE_MARKETPLACES = {"claude-plugins-official": "anthropics/claude-plugins-official"}


def installed_claude_plugins(claude):
    return {plugin["id"] for plugin in json.loads(run(claude, "plugin", "list", "--json", capture=True))}


def claude_plugins():
    """Install the profile's Claude plugins that are missing; an installed one is left as is."""
    wanted = selected()["claude_plugins"]
    claude = cli_path("claude")
    if not wanted or not claude:
        return
    missing = [plugin for plugin in wanted if plugin not in installed_claude_plugins(claude)]
    if not missing:
        return
    known = {item["name"] for item in json.loads(run(claude, "plugin", "marketplace", "list", "--json", capture=True))}
    for marketplace in sorted({plugin.split("@", 1)[1] for plugin in missing} - known):
        if marketplace not in CLAUDE_MARKETPLACES:
            raise RuntimeError(f"Unknown Claude marketplace: {marketplace}")
        run(claude, "plugin", "marketplace", "add", CLAUDE_MARKETPLACES[marketplace], timeout=600)
    for plugin in missing:
        run(claude, "plugin", "install", "--scope", "user", plugin, timeout=600)


def pinned_plugins():
    """The tmux plugin commits. They live in the Dotfiles' chezmoi data, which the Mac's install script reads too."""
    return json.loads((resolve_dotfiles_source() / ".chezmoidata/tmux_plugins.json").read_text())["tmuxPlugins"]


def plugins():
    for plugin in pinned_plugins():
        source = checkout(
            plugin["name"], f"https://github.com/{plugin['repo']}.git", plugin["commit"]
        )
        if (
            plugin["name"] == "tmux-thumbs"
            and not (source / "target/release/tmux-thumbs").exists()
        ):
            run("cargo", "build", "--release", "--locked", cwd=source, timeout=1200)
        target = HOME / ".tmux/plugins" / plugin["name"]
        install_tree(source, target, plugin_revision(plugin))
        if plugin["name"] == "tmux-resurrect":
            patch_resurrect(target)


def plugin_revision(plugin):
    """The installed marker of a plugin: its commit, and for tmux-resurrect also the SHA-256 of the Dotfiles patch,
    so an edited patch makes a fresh copy of the pinned tree instead of failing to apply over the old one."""
    if plugin["name"] != "tmux-resurrect":
        return plugin["commit"]
    patch = resolve_dotfiles_source() / ".patches/tmux-resurrect.diff"
    return f"{plugin['commit']} {hashlib.sha256(patch.read_bytes()).hexdigest()}"


def resurrect_patch(tree, *options):
    """git apply the Dotfiles' tmux-resurrect patch in a tree copied without .git; True when it succeeds."""
    patch = resolve_dotfiles_source() / ".patches/tmux-resurrect.diff"
    # The ceiling stops git from finding a repository above the tree and applying the patch relative to that.
    env = dict(os.environ, GIT_CEILING_DIRECTORIES=str(tree.resolve().parent))
    return subprocess.run(
        ["git", "apply", *options, str(patch)], cwd=tree, env=env, capture_output=True, check=False
    ).returncode == 0


def patch_resurrect(tree):
    """Apply the local tmux-resurrect patches once. Dotfiles' run_after script needs .git, which install_tree drops."""
    if resurrect_patch(tree, "--reverse", "--check"):
        return
    if not resurrect_patch(tree):
        raise RuntimeError(f"tmux-resurrect patches no longer apply: {tree}")


def fonts_and_themes():
    if has("gui"):
        nerd_font()


def nerd_font():
    version = VERSIONS["fontVersion"]
    archive = download(
        f"https://github.com/ryanoasis/nerd-fonts/releases/download/v{version}/JetBrainsMono.tar.xz",
        DATA / "cache" / f"JetBrainsMono-{version}.tar.xz",
        VERSIONS["fontSha256"],
    )
    with tarfile.open(archive) as tar:
        for member in tar.getmembers():
            if member.isfile() and (
                member.name.endswith(".ttf")
                or Path(member.name).name.startswith(("OFL", "LICENSE"))
            ):
                write(
                    HOME / ".local/share/fonts/JetBrainsMono" / Path(member.name).name,
                    tar.extractfile(member).read(),
                )
    run("fc-cache", HOME / ".local/share/fonts")


def config():
    if dotfiles_locked():
        if chezmoi_owns_dotfiles():
            source = resolve_dotfiles_source()
            REPORT["dotfiles"] = {"owner": "chezmoi", "source": str(source)}
            print(f"chezmoi manages the dotfiles from {source}; left as they are")
        else:
            apply_dotfiles()
    if has("agents"):
        agent_seeds()


def apply_dotfiles():
    source, rendered, digest = render_dotfiles()
    manifest = {
        "source": str(source),
        "profile": dotfiles_lock()["profile"],
        "rendered_sha256": digest,
        "targets": [],
    }
    for rel_name in sorted(rendered):
        contents, mode = rendered[rel_name]
        target = HOME / rel_name
        existed = target.exists() or target.is_symlink()
        if target.is_symlink():
            kind = "symlink"
            before = None
        elif target.is_file():
            kind = "file"
            before = hashlib.sha256(target.read_bytes()).hexdigest()
        elif target.is_dir():
            kind = "directory"
            before = None
        else:
            kind = "absent"
            before = None
        manifest["targets"].append(
            {
                "path": rel_name,
                "before_kind": kind,
                "existed": existed,
                "before_sha256": before,
                "after_sha256": hashlib.sha256(contents).hexdigest(),
                "mode": f"{mode:04o}",
                "applied": False,
            }
        )

    manifest_path = RUN / "dotfiles-manifest.json"

    def persist_manifest():
        temp = manifest_path.with_suffix(".json.tmp")
        temp.write_text(json.dumps(manifest, indent=2) + "\n")
        os.replace(temp, manifest_path)

    # Persist every preimage before the first mutation. A partial failure can then
    # distinguish original files/symlinks from targets that were previously absent.
    REPORT["dotfiles"] = manifest
    persist_manifest()
    save_report()
    by_name = {item["path"]: item for item in manifest["targets"]}
    for rel_name in sorted(rendered):
        contents, mode = rendered[rel_name]
        write(HOME / rel_name, contents, mode)
        by_name[rel_name]["applied"] = True
        persist_manifest()
        save_report()


def agent_seeds():
    # Safe appearance seeds, without copying authenticated sessions, hooks, or plugins.
    write(
        HOME / ".claude/settings.json",
        '{"theme":"dark-ansi"}\n',
        0o600,
        seed=True,
    )
    write(
        HOME / ".config/opencode/opencode.json",
        json.dumps(
            {
                "$schema": "https://opencode.ai/config.json",
                "permission": {
                    "*": "deny",
                    "read": "allow",
                    "glob": "allow",
                    "grep": "allow",
                },
            },
            indent=2,
        )
        + "\n",
        0o600,
        seed=True,
    )


def installed_extensions():
    """VS Code's installed extensions as lower-case id@version strings."""
    return set(run("code", "--list-extensions", "--show-versions", capture=True).lower().splitlines())


def vscode():
    # Install explicit dependencies first, preventing Code from choosing a newer
    # transitive version before our pinned version is installed.
    by_id = {item["id"].lower(): item for item in EXTENSIONS}
    ordered = []
    visited = set()
    visiting = set()

    def visit(item):
        key = item["id"].lower()
        if key in visited:
            return
        if key in visiting:
            raise RuntimeError(f"Circular extension dependency: {key}")
        visiting.add(key)
        for dependency in item.get("dependencies", []):
            if dependency.lower() in by_id:
                visit(by_id[dependency.lower()])
        visiting.remove(key)
        visited.add(key)
        ordered.append(item)

    for item in EXTENSIONS:
        visit(item)
    installed = installed_extensions()
    for extension in ordered:
        spec = f"{extension['id']}@{extension['version']}"
        if spec.lower() not in installed:
            run(
                "code",
                "--install-extension",
                spec,
                "--do-not-include-pack-dependencies",
                timeout=300,
            )
    # Continue's schema path must reference the installed Linux extension, not a path from another machine.
    import json5

    manifest = HOME / ".vscode/extensions/extensions.json"
    items = json.loads(manifest.read_text())
    continuation = next(
        x for x in items if x["identifier"]["id"].lower() == "continue.continue"
    )
    location = continuation.get("relativeLocation")
    if location:
        schema = HOME / ".vscode/extensions" / location / "config-yaml-schema.json"
        if schema.is_file():
            path = HOME / ".config/Code/User/settings.json"
            settings = json5.loads(path.read_text())
            schemas = settings.setdefault("yaml.schemas", {})
            schemas[schema.as_uri()] = [".continue/**/*.yaml"]
            write(path, json.dumps(settings, indent=2) + "\n")


# The values verify_tmux expects from the locked ~/.tmux.conf. `status 2` is the blank line above the bar.
TMUX_EXPECTED = (("status", "2"), ("status-position", "bottom"), ("prefix", "C-Space"))
# Whether each profile's tmux starts at boot (tmux-continuum). linux-full does; linux does not.
TMUX_BOOT = {"linux-full": "on", "linux": "off"}
CONTINUUM_BOOT = re.compile(r"^set -g @continuum-boot '(on|off)'$", re.MULTILINE)


def check_tmux_boot(config_text, profile):
    """The config's @continuum-boot must be the profile's; a missing or doubled setting is an error."""
    found = CONTINUUM_BOOT.findall(config_text)
    expected = TMUX_BOOT.get(profile)
    if expected is None:
        return
    if found != [expected]:
        raise RuntimeError(f"tmux automatic boot for {profile} must be {expected}, found {found or 'none'}")


TMUX_PLUGIN = re.compile(r"^set -g @plugin '([^']+)'$", re.MULTILINE)
TPM_RUN = "run '~/.tmux/plugins/tpm/tpm'"


def isolated_tmux_config(config_text):
    """The config for the throwaway check server, which must touch nothing real.

    tmux-continuum must not load there: it restores sessions, and on load it turns the user's boot unit on or
    off (`systemctl --user disable` when boot is off). Changing its options here is not enough, and neither is
    dropping its @plugin line, because tpm reads the plugin list from ~/.tmux.conf on disk, not from this file.
    So tpm's run line is replaced by running every other plugin's *.tmux files directly, as tpm would.
    """
    names = [plugin.split("#")[0].rstrip("/").rsplit("/", 1)[-1].removesuffix(".git")
             for plugin in TMUX_PLUGIN.findall(config_text)]
    runs = [f"""run 'for f in ~/.tmux/plugins/{name}/*.tmux; do "$f"; done'"""
            for name in names if name not in ("tpm", "tmux-continuum")]
    lines = []
    for line in config_text.splitlines():
        lines.extend(runs if line.strip() == TPM_RUN else [line])
    return "\n".join(lines) + "\n"


# The VS Code stage adds the Continue extension's machine-specific `yaml.schemas` entry to the locked
# settings, so that one key is the installer's, not the lock's.
VSCODE_SETTINGS = ".config/Code/User/settings.json"
INSTALLER_OWNED_KEYS = {VSCODE_SETTINGS: ("yaml.schemas",)}


def dotfile_matches(rel_name, expected, actual):
    """True when an installed dotfile is the locked render, apart from keys the installer itself adds."""
    if actual == expected:
        return True
    owned = INSTALLER_OWNED_KEYS.get(rel_name)
    if not owned:
        return False
    try:
        locked, installed = json.loads(expected), json.loads(actual)
    except ValueError:
        return False
    for key in owned:
        locked.pop(key, None)
        installed.pop(key, None)
    return locked == installed


def verify_tmux():
    # A separate server with persistence disabled cannot restore or stop real sessions.
    name = f"workstation-check-{os.getpid()}"
    real_text = (HOME / ".tmux.conf").read_text()
    check_tmux_boot(real_text, (SELECTED or {}).get("profile"))
    config_text = isolated_tmux_config(real_text)
    with tempfile.TemporaryDirectory(prefix="workstation-tmux-") as temp:
        path = Path(temp) / "tmux.conf"
        path.write_text(config_text)
        try:
            run(
                "tmux",
                "-L",
                name,
                "-f",
                path,
                "new-session",
                "-d",
                "-s",
                "verify",
                "sleep 60",
                capture=True,
            )
            # Startup can report config errors without failing new-session. Explicit
            # sourcing must succeed, just as it must when reloading a live server.
            run("tmux", "-L", name, "source-file", path, capture=True)
            for option, value in TMUX_EXPECTED:
                actual = run(
                    "tmux", "-L", name, "show-option", "-gv", option, capture=True
                ).strip()
                if actual != value:
                    raise RuntimeError(
                        f"tmux {option}: expected {value}, found {actual}"
                    )
            bar = run(
                "tmux", "-L", name, "show-option", "-gv", "status-right", capture=True
            )
            if "continuum_save.sh" not in bar or "%H:%M" not in bar:
                raise RuntimeError("Nord status bar or persistence hook is missing")
            popup = run(
                "tmux", "-L", name, "show-option", "-gv", "@popup-toggle", capture=True
            )
            if not popup.strip():
                raise RuntimeError("tmux popup plugin did not load")
        finally:
            subprocess.run(
                ["tmux", "-L", name, "kill-server"], capture_output=True, check=False
            )


def reload_tmux():
    # Only a ~/.tmux.conf this install wrote is reloaded: none without a dotfiles lock, nor when chezmoi owns it.
    if not dotfiles_locked() or chezmoi_owns_dotfiles():
        return
    current = subprocess.run(
        ["tmux", "list-sessions", "-F", "#{session_id}"],
        capture_output=True,
        text=True,
        check=False,
    )
    if current.returncode:
        return
    previous = run(
        "tmux", "show-option", "-gqv", "@continuum-restore-max-delay", capture=True
    ).strip()
    # Reload appearance and bindings without treating this as server startup.
    run("tmux", "set-option", "-g", "@continuum-restore-max-delay", "0")
    try:
        run("tmux", "source-file", HOME / ".tmux.conf", capture=True)
    finally:
        if previous:
            run("tmux", "set-option", "-g", "@continuum-restore-max-delay", previous)
        else:
            run("tmux", "set-option", "-gu", "@continuum-restore-max-delay")
    remaining = run("tmux", "list-sessions", "-F", "#{session_id}", capture=True)
    if not set(current.stdout.splitlines()).issubset(remaining.splitlines()):
        raise RuntimeError("An existing tmux session disappeared during reload")


# Commands each module must put on PATH, in check order.
COMMANDS = (
    ("ghostty", "gui"),
    ("tmux", "tmux"),
    ("starship", "shell"),
    ("docker", "containers"),
    ("fzf", "shell"),
    ("fdfind", "shell"),
    ("batcat", "shell"),
    ("eza", "shell"),
    ("zoxide", "shell"),
    ("wl-copy", "gui"),
    ("lazygit", "shell"),
    ("code", "gui"),
    ("gh", "dev"),
    ("cargo", "dev"),
    ("python3", "dev"),
    ("node", "dev"),
    ("npm", "dev"),
    ("java", "dev"),
    ("cmake", "dev"),
    ("ssh-copy-id", "base"),
)


def installed_clis():
    """The CLIs verify expects: uv with the dev module, then the profile's agent harnesses."""
    return (["uv"] if has("dev") else []) + (harnesses() if has("agents") else [])


DOCKER_RELOGIN = "You are in the docker group, but this session started before you joined it: log out and in for Docker access."


def docker_group_pending():
    """In the docker group's member list, but not in this process's groups: a new terminal is not a new
    login, so `docker info` would fail here with a socket permission error on a correctly installed machine."""
    try:
        group = grp.getgrnam("docker")
    except KeyError:
        return False
    user = os.environ.get("USER") or os.environ.get("LOGNAME", "")
    return user in group.gr_mem and group.gr_gid not in os.getgroups()


def verify_dotfiles(checks):
    """The dotfiles part of verify: chezmoi has nothing to apply when it owns them, else the locked render's checks."""
    owner = chezmoi_owns_dotfiles() if dotfiles_locked() else None
    if has("tmux") and not (dotfiles_locked() and not owner):
        checks.append("tmux plugins, resurrect patches and compiled thumbs")
    if owner:
        # Scripts (run_after_*) run on every apply, so they always show; only files count.
        pending_changes = run(owner, "status", "--exclude=scripts", capture=True, env=dict(os.environ, HOME=str(HOME)))
        if pending_changes.strip():
            raise RuntimeError("chezmoi has changes to apply; review them with chezmoi diff:\n" + pending_changes.rstrip())
        checks.append(f"chezmoi has nothing to apply from {resolve_dotfiles_source()}")
    elif dotfiles_locked():
        verify_locked_dotfiles(checks)


def verify_locked_dotfiles(checks):
    """The locked render is what is installed, and the shell and tmux it configures behave."""
    if has("tmux"):
        # The status bar, prefix and popup checks read the locked ~/.tmux.conf.
        verify_tmux()
        checks.append(
            "tmux plugins, resurrect patches, compiled thumbs, Nord status bar, prefix and popup"
        )
    aliases = run(
        "bash",
        "--noprofile",
        "--rcfile",
        HOME / ".bashrc",
        "-ic",
        "alias ll; declare -F z; command -v starship",
        capture=True,
    )
    if "eza -la" not in aliases:
        raise RuntimeError("ll is not configured")
    checks.append("interactive shell aliases, Starship and zoxide")
    _, rendered, digest = render_dotfiles()
    for rel_name, (expected, _) in rendered.items():
        target = HOME / rel_name
        if not target.is_file() or not dotfile_matches(rel_name, expected, target.read_bytes()):
            raise RuntimeError(f"Canonical Dotfiles target mismatch: {target}")
    checks.append(f"locked canonical Dotfiles profile {digest[:12]}")


def verify():
    checks = []
    docker_note = None
    for package in selected()["apt"]:
        status = run("dpkg-query", "-W", "-f=${Status}", package, capture=True).strip()
        if status != "install ok installed":
            raise RuntimeError(f"System package not installed: {package} ({status or 'unknown'})")
    checks.append("all listed system packages")
    for command, module in COMMANDS:
        if has(module) and not shutil.which(command):
            raise RuntimeError(f"Missing command: {command}")
    versions = []
    for command in installed_clis():
        path = cli_path(command)
        if not path:
            raise RuntimeError(f"Missing CLI: {command}")
        versions.append(run(path, "--version", capture=True).strip().splitlines()[0])
    checks.append("development tools" + (f" and {', '.join(versions)}" if versions else ""))
    wanted = selected()["claude_plugins"]
    if wanted:
        missing = sorted(set(wanted) - installed_claude_plugins(cli_path("claude")))
        if missing:
            raise RuntimeError(f"Claude plugins not installed: {', '.join(missing)}")
        checks.append(f"{len(wanted)} Claude plugins")
    if has("containers"):
        run("docker", "compose", "version", capture=True)
        run("docker", "buildx", "version", capture=True)
        if run("ps", "-p", "1", "-o", "comm=", capture=True).strip() == "systemd":
            if docker_group_pending():
                docker_note = DOCKER_RELOGIN
                checks.append("Docker Compose and Buildx; daemon access waits for a new login")
            else:
                run("docker", "info", "--format", "{{.ServerVersion}}", capture=True)
                checks.append("Docker daemon access, Compose and Buildx")
        else:
            checks.append("Docker Compose and Buildx; daemon requires a systemd host")
    if has("tmux"):
        for plugin in pinned_plugins():
            marker = HOME / ".tmux/plugins" / plugin["name"] / ".workstation-revision"
            if marker.read_text().strip() != plugin_revision(plugin):
                raise RuntimeError(f"Plugin mismatch: {plugin['name']}")
        if not resurrect_patch(HOME / ".tmux/plugins/tmux-resurrect", "--reverse", "--check"):
            raise RuntimeError("tmux-resurrect local patches are not applied")
        for executable in ("thumbs", "tmux-thumbs"):
            path = HOME / ".tmux/plugins/tmux-thumbs/target/release" / executable
            if not os.access(path, os.X_OK):
                raise RuntimeError(f"Missing precompiled tmux helper: {executable}")
    verify_dotfiles(checks)
    if has("gui"):
        family = run(
            "fc-match", "-f", "%{family}", "JetBrainsMono Nerd Font", capture=True
        )
        if "Nerd Font" not in family:
            raise RuntimeError("Nerd Font selection fell back to another font")
        checks.append("actual Nerd Font resolution")
        installed = installed_extensions()
        missing = [
            f"{e['id']}@{e['version']}"
            for e in EXTENSIONS
            if f"{e['id']}@{e['version']}".lower() not in installed
        ]
        if missing:
            raise RuntimeError("Missing VS Code extensions: " + ", ".join(missing))
        import json5

        code = json5.loads((HOME / ".config/Code/User/settings.json").read_text())
        if code["workbench.colorTheme"] != "Nord":
            raise RuntimeError("VS Code theme is not Nord")
        checks.append(f"{len(EXTENSIONS)} pinned VS Code extensions and Nord selection")
    REPORT["checks"] = checks
    REPORT["status"] = "verified"
    REPORT["pending"] = [
        "Account sign-in for Claude/Codex/GitHub is personal and is not copied.",
    ]
    if has("gui"):
        REPORT["pending"].append(
            "Continue is installed; its existing model configuration is preserved. A fresh machine needs an inference endpoint."
        )
    if docker_note:
        REPORT["pending"].append(docker_note)
    save_report()
    for check in checks:
        print(f"PASS {check}")
    for item in REPORT["pending"]:
        print(f"NOTE {item}")


def main():
    global RUN
    if os.geteuid() == 0:
        raise RuntimeError(
            "User configuration must run as the workstation user, never root"
        )
    os.umask(0o077)
    os.environ["GIT_TERMINAL_PROMPT"] = "0"
    action = sys.argv[1] if len(sys.argv) == 2 else ""
    if action == "verify":
        report_path = STATE / "report.json"
        if report_path.exists():
            REPORT.update(json.loads(report_path.read_text()))
        REPORT["failed"] = []
        # Verify the profile that was installed unless one is named explicitly.
        select_profile(os.environ.get("WORKSTATION_PROFILE") or REPORT.get("profile"))
        verify()
        return
    if action != "install":
        raise RuntimeError(
            "Use sudo ./bootstrap.sh to install, or ./verify.sh to verify"
        )
    select_profile()
    STATE.mkdir(parents=True, exist_ok=True)
    DATA.mkdir(parents=True, exist_ok=True)
    RUN = Path(tempfile.mkdtemp(prefix=time.strftime("%Y%m%d-%H%M%S-"), dir=STATE))
    REPORT["recovery"] = str(RUN)
    print(f"Profile {SELECTED['profile']}: {', '.join(SELECTED['modules'])}")
    for name, function, modules in (
        ("CLI tools", tools, ("base",)),
        ("tmux plugins", plugins, ("tmux",)),
        ("fonts and themes", fonts_and_themes, ("gui",)),
        ("configuration", config, ("shell", "agents")),
        ("Claude plugins", claude_plugins, ("agents",)),
        ("VS Code extensions", vscode, ("gui",)),
        ("existing tmux configuration reload", reload_tmux, ("tmux",)),
    ):
        if any(has(module) for module in modules):
            stage(name, function)
    REPORT["status"] = "installed-awaiting-verification"
    save_report()
    print(f"Recovery copies: {RUN}")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:  # noqa: BLE001 - report every failed installation stage
        REPORT["status"] = "failed"
        detail = (
            (error.stderr or error.stdout or "")[-4000:]
            if isinstance(error, subprocess.CalledProcessError)
            else ""
        )
        REPORT["failed"].append(f"{error}\n{detail}".strip())
        save_report()
        print(f"FAILED: {error}", file=sys.stderr)
        if detail:
            print(detail, file=sys.stderr)
        sys.exit(1)
