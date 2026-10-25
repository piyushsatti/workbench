# Included by every global git hook. core.hooksPath replaces .git/hooks entirely, so each
# global hook hands over to the repository's own hook of the same name (shared by every
# worktree through the common dir). exec keeps stdin, which pre-push and post-rewrite read.
# Called with (dict "hook" <hook name> "data" .).
common_dir=$(git rev-parse --git-common-dir 2>/dev/null) || exit 0
repo_hook="$common_dir/hooks/{{ .hook }}"
if [ -f "$repo_hook" ] && [ -x "$repo_hook" ]; then
	self=$(cd "$(dirname "$0")" && pwd -P)/$(basename "$0")
	target=$(cd "$(dirname "$repo_hook")" && pwd -P)/$(basename "$repo_hook")
	if [ "$self" != "$target" ]; then
		exec "$repo_hook" "$@"
	fi
fi
exit 0
