# Rulebook: watching long-running jobs

Load before arming a Monitor or a background wait on a job.

- **A monitor ends when the job ends, not when its log goes quiet.**
  `tail -F log | grep ... | while read ...; break` does not stop at the match: `break` ends the
  loop, but `tail` only learns its reader is gone on its next write, and a finished job writes
  nothing more, so the monitor hangs until its timeout. Tie the watch to the job instead:
  - the job runs in a tmux pane: watch with `tail --pid=<job pid> -F log | grep ...`, so tail
    exits with the process; or
  - poll the log in a loop that exits on the terminal line:
    `until grep -qE '^done in|Traceback' log; do sleep 5; done`, and emit progress lines from
    the same loop; or
  - the job writes a final marker (`echo EXIT $? >> log`): match it and exit the script.
- **The filter matches every way the job can end**: the success line, the failure summary, a
  traceback, and the shell's `EXIT <code>` marker, so silence never stands in for success.
- **One watcher per job.** Stop a pane wait (`tmux_wait`) or an earlier monitor before arming a
  new one, so one event is not reported twice (a doubled report inflates failure counts).
- **A long job prints timed, flushed progress, and the progress is visible.** Print the start, the counts, one line per unit with elapsed time, and a final done line, flushed (`PYTHONUNBUFFERED=1` when logging). Run it in a pane the user can see and have events pushed to the session; a log file alone, or polling pane by pane, is not enough.
