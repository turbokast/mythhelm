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
# docs/demos/hero-env.sh builds the disposable example repository and state
# directory in the hidden prelude (Hide ... Show). Everything it writes lives
# under $HERO_TMP, which record.sh removes afterwards.
#
# Render only through the record command, which builds the binary this tape
# drives, regenerates the transcript, renders the GIF and verifies all three:
#
#   docs/demos/record.sh
#
# A bare `vhs docs/demos/hero.tape` renders the GIF but is not the record
# command: it skips the build, the transcript regen and the verify pass.
#
# Binary-Version: v0.0.2-0.20261012002858-ebdda801a270
# Binary-Commit: ebdda801a2702d27db9e4ad29c9103c8cf4b288d
# VHS-Version: v0.12.1

Output docs/demos/hero.gif

Require git

Set Shell "bash"
Set FontSize 15
Set Width 1280
Set Height 740
Set Framerate 30
Set TypingSpeed 12ms

Hide
Type "PS1='$ ' && source docs/demos/hero-env.sh && clear"
Enter
Sleep 3s
Type "printf '%s\n' 'MYTHHELM  Kill the terminal mid-run. Nothing is lost.' 'SCRIPTED DEMO  fake agent, offline, no credentials. Recovery never relaunches the agent.'"
Enter
Show
Sleep 2500ms

# SIGKILL after 8s, while the paced scenario is still working. --foreground
# keeps the TUI in the terminal's foreground process group.
Type "timeout --foreground -s KILL 8 mythhelm run --adapter fake --scenario paced \"
Enter
Type "  --billing local-scripted --execution-profile trusted-host \"
Enter
Type "  --task-file task.md --trust-project-config $TRUST"
Enter
Sleep 9s

# The dead TUI leaves the terminal raw; `reset` is what a user would type.
Type "echo exit status $?"
Enter
Sleep 1500ms
Type "reset"
Enter
Sleep 1s
Type "mythhelm runs list"
Enter
Sleep 2500ms

# The tape copies the run ID out of the list above, as a user would paste it.
Hide
Type "RUN=$(mythhelm runs list | awk 'NR==2{print $1}')"
Enter
Show
Type "mythhelm recover $RUN"
Enter
Sleep 9s

Type "clear"
Enter
Type "mythhelm review $RUN"
Enter
Sleep 2s
Enter
Sleep 6s
