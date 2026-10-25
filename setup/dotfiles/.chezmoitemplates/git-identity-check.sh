# Included by the global git hooks that create commits (pre-commit, pre-merge-commit).
# Called with (dict "hook" <hook name> "data" .).
expected_name={{ dig "git" "name" "" .data | replace "'" "'\\''" | printf "'%s'" }}
expected_email={{ dig "git" "email" "" .data | replace "'" "'\\''" | printf "'%s'" }}

refuse() {
	printf '{{ .hook }}: refused: %s\n' "$1" >&2
	printf '{{ .hook }}: expected identity is %s <%s> (chezmoi data git.name / git.email).\n' "$expected_name" "$expected_email" >&2
	printf '{{ .hook }}: do not write a different identity into git config; fix the environment or ~/.config/chezmoi/chezmoi.toml and run chezmoi apply.\n' >&2
	exit 1
}

if [ -z "$expected_email" ] || [ -z "$expected_name" ]; then
	printf '{{ .hook }}: refused: no expected git identity is configured on this machine.\n' >&2
	printf '{{ .hook }}: run `chezmoi init` (it asks for git.name and git.email), then `chezmoi apply`.\n' >&2
	exit 1
fi

lower() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

check() {
	role=$1
	ident=$(git var "GIT_${role}_IDENT" 2>/dev/null) || refuse "git could not resolve the $(lower "$role") identity."
	# ident is "Name <email> <epoch> <tz>"; drop the timestamp, then split on " <".
	person=${ident% * *}
	name=${person% <*}
	email=${person##* <}
	email=${email%>}
	if [ "$name" != "$expected_name" ] || [ "$(lower "$email")" != "$(lower "$expected_email")" ]; then
		refuse "$(lower "$role") would be $name <$email>."
	fi
}

check AUTHOR
check COMMITTER
