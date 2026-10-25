# Shell and filesystem safety

- Never force past a refusal. Do not use destructive force flags, hook bypasses, hard resets, forced checkout, forced push, forced branch deletion, or `force: true` tool parameters.
- Never run `git clean -f`, `-fd`, or `-fdx`.
- Before removing a directory, inspect its contents and Git state. Never use `rm -rf` on anything not created in the current authorized turn.
- Use absolute paths or a subshell. Do not use a bare `cd` that persists hidden state.
- Check macOS command support before relying on GNU-only flags such as `timeout`, `date -d`, `readlink -f`, `stat -c`, or `find -printf`.
- Pasteable command blocks contain commands only, with no inline comments or backticks.
- On multi-host systems, identify a mounted path by host before recursive or bulk writes.
- Preserve untracked and ignored files. A clean-looking Git view is not proof that a directory is disposable.
- Prefer reversible, additive operations. State the blast radius and rollback boundary before a structural change.

Never use `git worktree remove --force`, `git push --force`, `git push --force-with-lease`, a `+ref` push, `git checkout --force`, `git switch --force`, `git reset --hard`, `git branch -D`, `--no-verify`, destructive `gh --yes`, or any force-valued tool parameter. A protected operation may proceed only through a safe non-force path after its state is understood and authorized.

When a command fails because a guard protects data, stop and report the command, refusal, and protected state. Do not find another route around the guard.
