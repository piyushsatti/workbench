#!/usr/bin/env bash
# Entry point on a fresh machine: make sure python3 exists, then hand over to install.py with the same arguments.
# macOS: python3 comes with the Xcode Command Line Tools (which also bring git). Ubuntu and Arch: installed with
# the system's package manager through sudo, only when missing. Run as your own user, not root.
set -Eeuo pipefail
HERE="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v python3 >/dev/null 2>&1; then
  case "$(uname -s)" in
    Darwin)
      printf '%s\n' 'python3 is missing. Run xcode-select --install, then run this again.' >&2
      exit 1
      ;;
    Linux)
      # shellcheck disable=SC1091
      . /etc/os-release
      if [[ " ${ID:-} ${ID_LIKE:-} " == *" arch "* ]]; then
        printf 'Installing python with pacman (needed to run the installer)\n'
        sudo pacman -S --needed --noconfirm python
      elif [[ " ${ID:-} ${ID_LIKE:-} " == *" ubuntu "* ]]; then
        printf 'Installing python3 with apt (needed to run the installer)\n'
        sudo apt-get update
        sudo env DEBIAN_FRONTEND=noninteractive apt-get install --yes --no-install-recommends python3
      else
        printf '%s is not supported; workbench installs on macOS, Ubuntu and Arch.\n' "${PRETTY_NAME:-This system}" >&2
        exit 1
      fi
      ;;
    *)
      printf '%s is not supported; workbench installs on macOS, Ubuntu and Arch.\n' "$(uname -s)" >&2
      exit 1
      ;;
  esac
fi

exec python3 -B "$HERE/install.py" "$@"
