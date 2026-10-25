#!/bin/sh
# Bootstrap tpm (tmux plugin manager). Plugins themselves install with
# prefix + I inside tmux.
if [ ! -d "$HOME/.tmux/plugins/tpm" ]; then
  git clone --depth 1 https://github.com/tmux-plugins/tpm "$HOME/.tmux/plugins/tpm"
fi
