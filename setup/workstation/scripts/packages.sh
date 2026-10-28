#!/usr/bin/env bash
set -Eeuo pipefail
SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
[[ $EUID == 0 ]] || exit 1
# The profile selects which listed packages to install (defaults.yml, scripts/profile.py).
# Names only: apt installs the version Ubuntu currently offers.
PROFILE=${1:-linux-full}
LIST="$SCRIPT_DIR/profiles/ubuntu-26.04/apt.txt"
export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l
APT=(apt-get -o Acquire::Retries=3 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 -o DPkg::Lock::Timeout=120)
BOOTSTRAP_PACKAGES=()
for package in ca-certificates curl gnupg; do
  entry=$(awk -v package="$package" '$1 == package {print $1; found=1} END {if (!found) exit 1}' "$LIST") || {
    printf 'Missing required package %s in %s.\n' "$package" "$LIST" >&2
    exit 1
  }
  BOOTSTRAP_PACKAGES+=("$entry")
done
timeout 600 "${APT[@]}" update
# The signed Ubuntu packages supply everything needed to configure the Code repository.
timeout 900 "${APT[@]}" install --yes --no-install-recommends \
  "${BOOTSTRAP_PACKAGES[@]}"
# The profile helper needs Python. Bare images lack it, so install the interpreter first.
if ! command -v python3 >/dev/null; then
  timeout 900 "${APT[@]}" install --yes --no-install-recommends python3
fi
if ! PROFILE_PACKAGES=$(python3 "$SCRIPT_DIR/scripts/profile.py" apt "$PROFILE"); then
  exit 1
fi
if [[ -n $PROFILE_PACKAGES ]]; then
  mapfile -t PACKAGES <<< "$PROFILE_PACKAGES"
else
  PACKAGES=()
fi
(( ${#PACKAGES[@]} )) || { printf 'Profile %s selected no packages.\n' "$PROFILE" >&2; exit 1; }
# Only profiles with the gui module install VS Code, so only they add its repository.
if [[ " ${PACKAGES[*]} " == *" code "* ]]; then
  TEMP_DIR=$(mktemp -d /tmp/workstation-root.XXXXXX)
  trap 'rm -r "$TEMP_DIR"' EXIT
  curl --fail --silent --show-error --location --retry 3 --max-time 120 \
    https://packages.microsoft.com/keys/microsoft.asc -o "$TEMP_DIR/microsoft.asc"
  # Trust the download only if it is exactly Microsoft's release signing key.
  MICROSOFT_FINGERPRINT=BC528686B50D79E339D3721CEB3E94ADBE1229CF
  export GNUPGHOME="$TEMP_DIR/gnupg"
  install -d -m 0700 "$GNUPGHOME"
  fingerprints=$(gpg --batch --with-colons --show-keys "$TEMP_DIR/microsoft.asc" | awk -F: '$1 == "fpr" {print $10}')
  if [[ $fingerprints != "$MICROSOFT_FINGERPRINT" ]]; then
    printf 'The Microsoft key has fingerprint %s, want %s; not adding the VS Code repository.\n' \
      "${fingerprints//$'\n'/ }" "$MICROSOFT_FINGERPRINT" >&2
    exit 1
  fi
  gpg --batch --dearmor --output "$TEMP_DIR/microsoft.gpg" "$TEMP_DIR/microsoft.asc"
  install -d -m 0755 /etc/apt/keyrings
  install -m 0644 "$TEMP_DIR/microsoft.gpg" /etc/apt/keyrings/workstation-microsoft.gpg
  printf '%s\n' 'Types: deb' 'URIs: https://packages.microsoft.com/repos/code' 'Suites: stable' \
    'Components: main' "Architectures: $(dpkg --print-architecture)" \
    'Signed-By: /etc/apt/keyrings/workstation-microsoft.gpg' > "$TEMP_DIR/workstation-code.sources"
  # Respect an existing Code repository instead of adding a conflicting Signed-By entry.
  if ! grep -rl 'packages.microsoft.com/repos/code' /etc/apt/sources.list.d /etc/apt/sources.list 2>/dev/null | grep -q .; then
    install -m 0644 "$TEMP_DIR/workstation-code.sources" /etc/apt/sources.list.d/workstation-code.sources
  fi
  timeout 600 "${APT[@]}" update
fi
timeout 120 "${APT[@]}" --simulate install --no-install-recommends "${PACKAGES[@]}" >/dev/null
timeout 1800 "${APT[@]}" install --yes --no-install-recommends "${PACKAGES[@]}" </dev/null
