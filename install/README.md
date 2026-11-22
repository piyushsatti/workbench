# install

Pick what you want on a fresh machine and get it in one run, on macOS, Ubuntu or Arch (Omarchy included) ([decisions](../docs/decisions.md#install)).

```bash
workbench install                         # a numbered menu: pick a bundle or items
workbench install --bundle tools          # just the tools
workbench install --bundle tools+agent    # the tools and the agent layer
workbench install --pick tmux,neovim,gh   # only these (plus what they need)
workbench install --bundle tools --no-gui # a headless box
workbench install list                    # what this system is offered
workbench update                          # pull workbench, then upgrade what install put in place
```

Every run prints its plan first. `--apply` runs it; on a terminal you are asked instead. Nothing changes without one of those.

## On a bare machine

`install.sh` only makes sure `python3` exists, then runs `install.py`.

```bash
# macOS: git and python3 come with the Command Line Tools
xcode-select --install
git clone https://github.com/piyushsatti/workbench ~/Source/workbench
~/Source/workbench/install/install.sh --bundle tools+agent
```

```bash
# Ubuntu
sudo apt install -y git
git clone https://github.com/piyushsatti/workbench ~/Source/workbench
~/Source/workbench/install/install.sh --bundle tools+agent
```

```bash
# Arch or Omarchy
sudo pacman -S --needed git
git clone https://github.com/piyushsatti/workbench ~/Source/workbench
~/Source/workbench/install/install.sh --bundle tools+agent
```

Run it as your own user. It uses `sudo` for apt and pacman; Homebrew refuses root.

## How it is built

| File | What |
|---|---|
| `catalog.json` | the pick-list: each item's group, description, packages per system (`brew`/`cask`, `apt`, `pacman`/`aur`), `requires`, `steps`, and the two bundles |
| `install.py` | resolves picks, builds the plan, runs it, remembers picks in `~/.local/state/workbench/install.json` |
| `adapters/` | one per system: `problems()` (refuse early), `prepare()`, `installed()`, `install()`, `upgrade()` |
| `steps.py` | what an item does besides packages: dotfiles through chezmoi, the agent CLIs from npm, the docker group, the Nerd Font on Ubuntu, the agent layer (`scripts/deploy_agent.py --apply`) |
| `install.sh` | bare-machine entry |

The folder is called `adapters/`, not `os/`, because `install/` is on Python's path and an `os` package would shadow the standard library.

Rules it follows:

- **Standard library only, Python 3.9 or later.** macOS's own `python3` (Command Line Tools) is 3.9.
- **Installed is kept.** A package already installed is not reinstalled. An agent CLI already on the machine is kept: it updates itself. Upgrades are `update`'s job.
- **Dotfiles never overwrite unasked.** `chezmoi apply --less-interactive` asks before replacing a file that already exists.
- **Omarchy owns its desktop.** On Omarchy, bundles leave out `dotfiles` and `nerd-font` (Omarchy ships its own configs for ghostty, tmux, starship, git and btop, and its own JetBrains Mono Nerd font). Name an item with `--pick` to install it anyway.
- **Arch has no partial upgrades.** Installs use `pacman -S --needed` (as Omarchy's own `omarchy-pkg-add` does); `update` runs `yay -Syu` or `paru -Syu`, else `sudo pacman -Syu`. AUR items need yay or paru; without one they are skipped with a note, and the installer never builds a helper.
- **macOS installs Homebrew** with the official script from brew.sh when `brew` is missing. Docker on macOS is the `docker` CLI with Colima as the engine (`colima start`), not Docker Desktop.

## Ubuntu and `workbench setup`

On Ubuntu, the `tools` bundle installs exactly the packages of `setup/workstation/profiles/ubuntu-26.04/apt.txt` (a test checks it). `workbench setup` stays the full pinned Ubuntu 26.04 workstation: the locked dotfiles render, tmux plugin pins, the VS Code extensions list, OpenCode with its deny-by-default config, and the end-to-end verify. The installer does not do those yet.

## What `update` does

1. `git pull --ff-only` in the workbench checkout, then reruns itself from the pulled copy.
2. Installs anything the catalog now lists for your remembered picks.
3. Upgrades their packages (`brew upgrade`, `apt-get install --only-upgrade`, or a full Arch upgrade).
4. Updates the agent CLIs this installer put in `~/.local/share/workbench/npm`; ones you installed another way update themselves.
5. Redeploys the agent layer, if you picked it.

## Tests

```bash
uv run python -B -m unittest discover -s install/tests
/usr/bin/python3 -B -m unittest discover -s install/tests   # on a Mac: the 3.9 floor
```

They check the catalog (requires order, bundle names, step names, the apt.txt match), pick resolution, the menu parser, system detection from `/etc/os-release` samples, and plans built against a fake adapter. They run no package manager. Nothing here has been run on a real Ubuntu, Arch or Omarchy install yet; on macOS only the dry run has.
