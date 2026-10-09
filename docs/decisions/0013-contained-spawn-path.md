# 0013. Contained spawn path: the worker spawns `__contain`, which execs the native once

- Status: accepted
- Date: 2026-10-09

## Context

ADR-0004 gives each attempt one detached worker that owns one native process
(I18) and stops it as a group (I06). A contained profile needs the native to
run inside a user and mount namespace with a read-only root, and Go cannot run
mount setup between fork and exec. The spawn site (`launcher.Launch`) and its
single-launch rule must not multiply.

## Decision

- A `Launch` that carries `Containment` makes the worker spawn
  `mythhelm __contain` instead of the native, with the namespace clone flags
  and its own process group. `__contain` reads a bounded (1 MiB), strictly
  decoded `ContainSpec` on stdin, puts the prompt pipe from fd 3 onto stdin,
  builds the boundary and execs the native in the same process. Exactly one
  process exists per worker; its PID and group are the native's.
- The prompt travels on a dedicated pipe so stdin carries only the control
  envelope. The worker copies `ProcSpec.Stdin` into it and ends the copy when
  the native is reaped.
- Before a contained spawn the worker starts the egress proxy on a
  loopback ephemeral port, fills `Policy.ProxyAddr` with its address, pins the
  proxy variables into the child environment and stops the proxy when the
  native is reaped. `Launch.ProxyAllow` is the exact `host:port` list; empty
  denies everything.
- A `Launch` without `Containment` takes the previous code path unchanged. Off
  Linux a contained launch is refused with `ErrUnsupported`.

## Consequences

- The launcher still refuses a second launch and the stop ladder still signals
  one group, so I06 and I18 hold. `TestContainedLaunchEndToEnd`,
  `TestContainedStdinForwardedByteForByte` and
  `TestContainedProxyDenyThroughPinnedEnv` pin the boundary, the prompt bytes
  and the proxy pins; `TestLaunchWithoutContainmentUnchanged` pins the
  uncontained path.
- A setup failure inside `__contain` ends the native process with exit code 1
  and a message on the attempt's captured stderr; the worker sees a native
  exit, not a failed launch. Distinguishing them needs a status channel and is
  left to the task that wires admission.
- The proxy pins are the child's initial environment. The native can unset
  them, and direct egress is not blocked; both stay disclosed residuals.
