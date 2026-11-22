"""Steps: what an item does besides installing packages. Each step returns Actions, or [] with a note when the
machine already has what the step would give it. A step only reads the machine while planning; it changes
nothing until install.py runs its Actions with --apply.

A step is named in catalog.json as a list: the step name, then its arguments.
"""

import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
from pathlib import Path

from adapters import Action

# The agent harnesses this installer can deploy the agent layer for (agent/<harness>/harness.json).
AGENT_HARNESSES = ("claude", "codex", "cursor")
MICROSOFT_KEY = "https://packages.microsoft.com/keys/microsoft.asc"
# Microsoft's release signing key; the same check as setup/workstation/scripts/packages.sh.
MICROSOFT_FINGERPRINT = "BC528686B50D79E339D3721CEB3E94ADBE1229CF"
CODE_SOURCES = ("/etc/apt/sources.list.d", "/etc/apt/sources.list")


class Context:
    """What a step may look at: the repo, the home folder, the system, the picked items, and a place to note."""

    def __init__(self, repo, home, system, flavor, adapter, items, which=shutil.which):
        self.repo, self.home, self.system, self.flavor = Path(repo), Path(home), system, flavor
        self.adapter, self.items, self.which = adapter, list(items), which
        self.notes = []

    @property
    def data(self):
        return self.home / ".local/share/workbench"

    def local_bin(self, name):
        return self.home / ".local/bin" / name

    def cli(self, name):
        """An installed CLI: ~/.local/bin first, then PATH."""
        local = self.local_bin(name)
        if local.exists():
            return str(local)
        return self.which(name)

    def note(self, line):
        self.notes.append(line)
        return []


def write_file(path, text, mode=0o644):
    def run():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        path.chmod(mode)
    return run


def chezmoi_script(ctx):
    """chezmoi on Ubuntu, from chezmoi's own install script into ~/.local/bin (as the README's dotfiles path does)."""
    found = ctx.cli("chezmoi")
    if found:
        return ctx.note(f"kept the installed chezmoi at {found}; it is not replaced")
    target = ctx.home / ".local/bin"
    return [Action("install chezmoi into ~/.local/bin (get.chezmoi.io)",
                   ["sh", "-c", f'sh -c "$(curl -fsLS get.chezmoi.io)" -- -b "{target}"'])]


def dotfiles(ctx):
    """chezmoi init from this repo's setup/dotfiles, then apply. --less-interactive prompts only for files that
    already exist or changed, so a fresh machine applies quietly and an existing dotfile is never overwritten unasked."""
    source = ctx.repo / "setup/dotfiles"
    chezmoi = ctx.cli("chezmoi") or "chezmoi"
    if ctx.cli("chezmoi"):
        result = subprocess.run([chezmoi, "source-path"], capture_output=True, text=True, check=False)
        if result.returncode == 0 and Path(result.stdout.strip()) == source:
            return ctx.note("dotfiles: chezmoi already uses setup/dotfiles; `workbench dotfiles diff` shows what changed")
    return [Action("chezmoi init --source setup/dotfiles (asks for your git name and email)",
                   [chezmoi, "init", "--source", str(source)]),
            Action("chezmoi apply --less-interactive (asks before replacing a file you already have)",
                   [chezmoi, "--source", str(source), "apply", "--less-interactive"])]


def wrapper(ctx, name, real):
    """~/.local/bin/<name> that runs <real>: Ubuntu installs fd and bat as fdfind and batcat."""
    path = ctx.local_bin(name)
    if path.exists() or ctx.which(name):
        return ctx.note(f"kept the existing {name}")
    return [Action(f"write ~/.local/bin/{name} -> {real}", func=write_file(path, f'#!/bin/sh\nexec {real} "$@"\n', 0o755))]


def pipx(ctx, package):
    found = ctx.cli(package)
    if found:
        return ctx.note(f"kept the installed {package} at {found}; it updates itself")
    return [Action(f"pipx install {package}", ["pipx", "install", package])]


def docker_group(ctx):
    user = os.environ.get("USER") or ctx.home.name
    actions = [Action(f"add {user} to the docker group (takes effect at the next login)",
                      ["usermod", "--append", "--groups", "docker", user], sudo=True)]
    if Path("/run/systemd/system").is_dir():
        actions.append(Action("systemctl enable --now docker", ["systemctl", "enable", "--now", "docker.service"], sudo=True))
    return actions


def npm_cli(ctx, name, package):
    """An agent CLI from npm into ~/.local/share/workbench/npm with a wrapper in ~/.local/bin, when missing.
    One already installed is kept: it updates itself (the rule setup/workstation follows too)."""
    found = ctx.cli(name)
    if found:
        return ctx.note(f"kept the installed {name} at {found}; it updates itself")
    prefix = ctx.data / "npm"
    target = prefix / "node_modules/.bin" / name
    return [Action(f"npm install {package}@latest into ~/.local/share/workbench/npm",
                   ["npm", "install", "--no-audit", "--no-fund", "--prefix", str(prefix), f"{package}@latest"]),
            Action(f"write ~/.local/bin/{name} -> the npm install",
                   func=write_file(ctx.local_bin(name), f'#!/bin/sh\nexec "{target}" "$@"\n', 0o755))]


def claude_plugins_wanted(repo):
    """The official plugin list, kept in one place: the agents module of setup/workstation/defaults.yml."""
    import importlib.util
    path = Path(repo) / "setup/workstation/scripts/profile.py"
    # Loaded by path under its own name: a plain `import profile` would find the standard library's profile module.
    spec = importlib.util.spec_from_file_location("workstation_profile", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return list(module.load()["modules"]["agents"].get("claude_plugins") or [])


# Where each marketplace in the plugin list comes from (setup/workstation/scripts/workstation.py has the same table).
CLAUDE_MARKETPLACES = {"claude-plugins-official": "anthropics/claude-plugins-official"}


def claude_json(claude, *args):
    """A `claude ... --json` listing, or [] when claude is missing or the output is not JSON."""
    if not claude:
        return []
    result = subprocess.run([claude, *args, "--json"], capture_output=True, text=True, check=False)
    try:
        return json.loads(result.stdout or "[]")
    except ValueError:
        return []


def claude_plugins(ctx):
    """The listed official plugins that are missing, for the user; one already installed is left as is."""
    claude = ctx.cli("claude")
    have = {p.get("id") for p in claude_json(claude, "plugin", "list")}
    missing = [p for p in claude_plugins_wanted(ctx.repo) if p not in have]
    if not missing:
        return ctx.note("claude plugins: every listed plugin is installed")
    known = {m.get("name") for m in claude_json(claude, "plugin", "marketplace", "list")}
    actions = []
    for marketplace in sorted({p.split("@", 1)[1] for p in missing} - known):
        source = CLAUDE_MARKETPLACES[marketplace]
        actions.append(Action(f"claude plugin marketplace add {source}", ["claude", "plugin", "marketplace", "add", source]))
    actions += [Action(f"claude plugin install {p}", ["claude", "plugin", "install", "--scope", "user", p]) for p in missing]
    return actions


def agent_layer(ctx):
    """scripts/deploy_agent.py --apply for the harnesses picked in this run, or all three when none was picked."""
    chosen = [h for h in AGENT_HARNESSES if h in ctx.items] or list(AGENT_HARNESSES)
    return [Action(f"deploy the agent layer: common part, then {', '.join(chosen)}",
                   [sys.executable, "-B", str(ctx.repo / "scripts/deploy_agent.py"), "--apply", "--harness", ",".join(chosen)])]


def nerd_font(ctx):
    """JetBrainsMono Nerd Font on Ubuntu, at the version and checksum in the workstation's versions.json."""
    versions = json.loads((ctx.repo / "setup/workstation/profiles/ubuntu-26.04/versions.json").read_text())
    fonts = ctx.home / ".local/share/fonts/JetBrainsMono"
    if fonts.is_dir() and any(fonts.glob("*.ttf")):
        return ctx.note("kept the installed JetBrainsMono Nerd Font")
    version, digest = versions["fontVersion"], versions["fontSha256"]
    url = f"https://github.com/ryanoasis/nerd-fonts/releases/download/v{version}/JetBrainsMono.tar.xz"

    def run():
        import hashlib
        with tempfile.TemporaryDirectory() as temp:
            archive = Path(temp) / "font.tar.xz"
            with urllib.request.urlopen(url, timeout=120) as response:
                archive.write_bytes(response.read())
            if hashlib.sha256(archive.read_bytes()).hexdigest() != digest:
                raise RuntimeError(f"{url} does not match the checksum in versions.json")
            fonts.mkdir(parents=True, exist_ok=True)
            with tarfile.open(archive) as tar:
                for member in tar.getmembers():
                    name = Path(member.name).name
                    if member.isfile() and (name.endswith(".ttf") or name.startswith(("OFL", "LICENSE"))):
                        data = tar.extractfile(member)
                        if data:
                            (fonts / name).write_bytes(data.read())
        subprocess.run(["fc-cache", str(fonts)], check=True)
    return [Action(f"install JetBrainsMono Nerd Font {version} into ~/.local/share/fonts (checksummed)", func=run)]


def microsoft_code_repo(ctx):
    """Microsoft's apt repository for VS Code, trusted only if the key has Microsoft's fingerprint. An existing
    Code repository is respected, as setup/workstation/scripts/packages.sh does."""
    for source in CODE_SOURCES:
        path = Path(source)
        files = sorted(path.glob("*")) if path.is_dir() else [path] if path.exists() else []
        if any("packages.microsoft.com/repos/code" in f.read_text(errors="replace") for f in files if f.is_file()):
            return ctx.note("VS Code: a Microsoft Code apt repository is already configured")

    def run():
        with tempfile.TemporaryDirectory() as temp:
            key = Path(temp) / "microsoft.asc"
            with urllib.request.urlopen(MICROSOFT_KEY, timeout=120) as response:
                key.write_bytes(response.read())
            env = dict(os.environ, GNUPGHOME=str(Path(temp) / "gnupg"))
            Path(env["GNUPGHOME"]).mkdir(mode=0o700)
            shown = subprocess.run(["gpg", "--batch", "--with-colons", "--show-keys", str(key)],
                                   capture_output=True, text=True, env=env, check=True).stdout
            prints = [line.split(":")[9] for line in shown.splitlines() if line.startswith("fpr:")]
            if prints != [MICROSOFT_FINGERPRINT]:
                raise RuntimeError(f"Microsoft key fingerprint {prints}, want {MICROSOFT_FINGERPRINT}; repository not added")
            keyring = Path(temp) / "microsoft.gpg"
            subprocess.run(["gpg", "--batch", "--dearmor", "--output", str(keyring), str(key)], env=env, check=True)
            arch = subprocess.run(["dpkg", "--print-architecture"], capture_output=True, text=True, check=True).stdout.strip()
            sources = Path(temp) / "workstation-code.sources"
            sources.write_text("\n".join(["Types: deb", "URIs: https://packages.microsoft.com/repos/code", "Suites: stable",
                                          "Components: main", f"Architectures: {arch}",
                                          "Signed-By: /etc/apt/keyrings/workstation-microsoft.gpg"]) + "\n")
            sudo = [] if os.geteuid() == 0 else ["sudo"]
            subprocess.run([*sudo, "install", "-d", "-m", "0755", "/etc/apt/keyrings"], check=True)
            subprocess.run([*sudo, "install", "-m", "0644", str(keyring), "/etc/apt/keyrings/workstation-microsoft.gpg"], check=True)
            subprocess.run([*sudo, "install", "-m", "0644", str(sources), "/etc/apt/sources.list.d/workstation-code.sources"], check=True)
            subprocess.run([*sudo, "apt-get", "update"], check=True)
    return [Action("add Microsoft's VS Code apt repository (key fingerprint checked)", func=run)]


STEPS = {
    "chezmoi-script": chezmoi_script,
    "dotfiles": dotfiles,
    "wrapper": wrapper,
    "pipx": pipx,
    "docker-group": docker_group,
    "npm-cli": npm_cli,
    "claude-plugins": claude_plugins,
    "agent-layer": agent_layer,
    "nerd-font": nerd_font,
    "microsoft-code-repo": microsoft_code_repo,
}


def actions(ctx, step):
    name, args = step[0], step[1:]
    return STEPS[name](ctx, *args)
