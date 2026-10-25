# Workstation

Run this on Ubuntu 26.04 LTS from your normal user account:

```sh
sudo ./bootstrap.sh
```

That installs the complete profile. There is no TUI, Apply All, or
selection prompt. Sudo authenticates once; system packages run as root, and
configuration, extensions and third-party builds run as the user who invoked sudo.

The profile includes Ghostty, the Nerd Font, the tmux/plugin setup, shell
aliases and prompt, the VS Code extensions/preferences adapted to Linux, Docker, development
runtimes and Claude/Codex/OpenCode.

[Versions and licenses](docs/software.md) describes the pinned sources.
Configuration replacements are saved in ~/.local/state/workstation/TIMESTAMP/.
Existing authenticated application configurations are preserved.

After installation, open a new terminal. A new login is needed for Docker group
membership. The installer never logs you out.
Continue requires your model endpoint; account sign-in and private data remain yours.

Verify again as your normal user:

```sh
./verify.sh
```

Every required check must pass. The detailed result is
~/.local/state/workstation/report.json, including any pending session activation.

## Profiles and modules

`defaults.yml` lists the machine profiles (`linux`, `linux-full`, `darwin`), the modules each one includes, and each module's packages.
It holds package names only. Pins stay in `profiles/ubuntu-26.04/`. `scripts/profile.py` expands a profile (stdlib only; `bootstrap.sh` and `scripts/packages.sh` call it as bare **`python3`**, not `uv run`, because it runs before Python tooling is installed — [decisions](../../docs/decisions.md#python)):

```sh
python3 scripts/profile.py show linux        # modules, apt entries, tools, harnesses
python3 scripts/profile.py check           # every lock entry belongs to exactly one module
```

Workstation unit tests from the repo root: `uv run python -B -m unittest discover -s setup/workstation/tests` (the tmux tests need real sockets, so run them outside a sandbox).

Pick the profile with `sudo ./bootstrap.sh --profile linux`. The default is
`linux-full`, which installs exactly the packages `apt.txt` lists, in the same
order (a test checks this), at the versions Ubuntu currently offers. `linux` drops the
`gui` module: no Ghostty, VS Code, Nerd Font or Wayland clipboard, and applies its own
Dotfiles lock without those settings. CLIs you already have (chezmoi, uv, Claude Code,
Codex, OpenCode) are kept and listed under `kept` in the report; missing ones get the latest. `./verify.sh` checks the profile
recorded at install time. `darwin` is listed for chezmoi but has no package lock, so
bootstrap refuses it. None of this installs the OS: it adds packages and dotfiles to an
Ubuntu 26.04 that is already installed.

`defaults.yml` is a small YAML subset (described at the top of the file). Standard-library
Python can parse it, and it is still valid YAML. The subset allows no quotes, no inline
mappings, and no values that YAML would read as booleans or numbers.

## Installation sources are explicit

| Path | Responsibility |
|---|---|
| bootstrap.sh | OS/sudo gate, profile choice, apt installation by name, user handoff, verification |
| defaults.yml | Profiles, their modules and each module's package names |
| scripts/profile.py | Expands a profile into modules and locked apt entries (standard library only) |
| scripts/workstation.py | User-owned tools, pinned plugins, locked canonical Dotfiles render, Nerd Font, VS Code extensions, checks |
| profiles/ubuntu-26.04/ | Apt package names (apt.txt), tmux plugin commits (plugins.json), VS Code extension IDs/versions (extensions.json), chezmoi and font download pins (versions.json) |
| profiles/ubuntu-26.04/dotfiles.lock.json, dotfiles.linux.lock.json | exact target set and rendered-content lock of the Dotfiles for `linux-full` and `linux` |
| tests/ | Recovery tests and complete Ubuntu integration build |

The sibling `../dotfiles` component is the canonical authored source for shared
shell, tmux, Starship, Ghostty and btop files. Workstation renders only its locked
Ubuntu target set into an archive. The installer validates the target set and
content digest, then backs up and replaces only those files, avoiding an existing
Chezmoi configuration's removal prompts. A system-root Workstation worktree contains both setup owners at the same paths;
a partial/component-only checkout needs an explicit verified Dotfiles source override. Tests and named build contexts may set
`WORKSTATION_DOTFILES_SOURCE` explicitly.

The Ubuntu profile excludes Dotfiles' TPM run-once script because Workstation
installs plugins at pinned commits.
Every install records the source, profile, rendered digest and each target's
before/after state in its recovery directory.

The old TUI, selection manifests and partial Docker recipes have been retired;
their previous versions remain in Git history.

## Verification before shipping

```sh
./tests/test.sh
docker build --build-context dotfiles=../dotfiles --platform linux/amd64 --target recovery -f tests/Dockerfile.complete -t workstation:recovery .
docker build --build-context dotfiles=../dotfiles --platform linux/amd64 --target installed -f tests/Dockerfile.complete -t workstation:verified .
```

The recovery image is retained before the install. The complete build executes
the real command twice, then the standalone verifier. It tests Linux userland,
not a physical GPU or Docker daemon inside the container.
Use a real Ubuntu machine for the final session-level checks. macOS support remains deferred.

## Recovery

Each run saves changed files under its recovery directory using paths relative
to your home. The original symlink is preserved if a managed file was a symlink.
To recover a particular file, inspect its backup and copy that specific entry
back.
Do not recursively restore the entire home directory.

Apt package changes are recorded in /var/log/apt/history.log. These file-level
recovery copies do not undo packages; use the machine backup for a full rollback.
