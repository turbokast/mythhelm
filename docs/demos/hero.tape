# MYTHHELM hero recording: kill the terminal mid-run, then recover.
# Runs the fake adapter's paced scenario through the real `mythhelm run` TUI,
# kills the foreground process with SIGKILL while the agent is mid-attempt,
# then `mythhelm recover` reattaches to the same worker, the work finishes,
# the checks run, and `mythhelm review` shows the revision-attached result.
# Offline and credential-free: scripted agent, local-scripted billing, no
# network. The product's own admission lines carry the SCRIPTED label.
#
# The kill is a real `timeout -s KILL` of the foreground `mythhelm run`
# process only: the worker and the agent keep running. A worker killed outright
# is a different case (issue #391) and is not shown.
#
# Hidden segments (Hide ... Show) hold setup and terminal hygiene only, never
# product behaviour: sourcing docs/demos/hero-env.sh (a disposable repository
# and state directory under /tmp/demo), the title card, `reset` after the dead
# TUI leaves the terminal raw, copying the run ID out of `runs list` into a
# shell variable, and `clear`. The red marker after the kill is printed by the
# shell from the real exit status of `timeout`; it is an annotation, not
# mythhelm output. Hidden time still passes, so the agent keeps working.
#
# Render only through the record command, which builds the binary this tape
# drives, regenerates the transcript, renders the GIF and verifies all three:
#
#   docs/demos/record.sh
#
# A bare `vhs docs/demos/hero.tape` renders the GIF but is not the record
# command: it skips the build, the transcript regen and the verify pass.
#
# Binary-Version: v0.0.2-0.20261012083830-031676ad5355
# Binary-Commit: 031676ad5355fcad3ef15059693df5f3a9b3d241
# VHS-Version: v0.12.1

Output docs/demos/hero.gif

Require git

Set Shell "bash"
Set FontSize 18
Set Width 1280
Set Height 650
Set Framerate 30
Set TypingSpeed 1ms

# Title card: the first frame the viewer sees.
Hide
Type "PS1= && source docs/demos/hero-env.sh && clear && printf '\033[?25l\n\n\n\n\n\n\n\n\n\n                \033[1;36mMYTHHELM\033[0m  \033[1m— kill the terminal mid-run. Nothing is lost.\033[0m\n\n                \033[33mSCRIPTED DEMO\033[0m  fake agent, offline, no credentials.\n                Recovery never relaunches the agent.\n'"
Enter
Sleep 3s
Show
Sleep 2s
Hide
Type "PS1='$ ' && printf '\033[?25h' && clear"
Enter
Sleep 300ms
Show
Set TypingSpeed 12ms

# SIGKILL after 8s, while the paced scenario is still working. --foreground
# keeps the TUI in the terminal's foreground process group.
Type "timeout --foreground -s KILL 8 mythhelm run --adapter fake --scenario paced \"
Enter
Type "  --billing local-scripted --execution-profile trusted-host \"
Enter
Type "  --task-file task.md --trust-project-config $TRUST"
Enter
# The TUI follows the run for 8 seconds, then the process is gone and its last
# frame stays frozen on the screen for a moment.
Sleep 9s

Set TypingSpeed 1ms
Hide
Type "s=$?; reset; RUN=$(mythhelm runs list | awk 'NR==2{print $1}'); printf '\033[1;31m✗ mythhelm killed (SIGKILL, exit status %s) — the agent is still working…\033[0m\n' $s"
Enter
Sleep 1500ms
Show
Sleep 1800ms

# The worker is a separate process and survived: its pid is the one recover
# reattaches to below.
Set TypingSpeed 12ms
Type 'pgrep -f "__worker --state $MYTHHELM_HOME"'
Enter
Sleep 2s

Type "mythhelm recover $RUN"
Enter
# The journal replays at once, then the agent's remaining work arrives live.
Sleep 7s

Set TypingSpeed 1ms
Hide
Type "clear"
Enter
Sleep 200ms
Show
Set TypingSpeed 12ms
Type "mythhelm review $RUN"
Enter
Sleep 2s
Enter
Sleep 4500ms
