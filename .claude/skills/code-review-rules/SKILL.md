---
name: code-review-rules
description: Project-specific traps for reviewing SSH Tunnel Manager changes. Read by the pre-commit review (personal-code-review) together with AGENTS.md.
---

# Code review rules for SSH Tunnel Manager

- **Vendored code must be committed whole.** A change that adds or updates files under `frontend/vendor/` checks `git status --ignored` and `git check-ignore -v` for the new tree. A nested folder (noVNC's `vendor/pako`) was once matched by a broad `.gitignore` pattern, so a clean checkout could not build.
- **The host key callback runs on every rekey.** x/crypto/ssh calls the `HostKeyCallback` on every rekey, not only during the handshake: any state it changes (timers, flags) must handle calls after the handshake has finished.
- **Every SSH agent connection is closed, and every agent request is bounded.** The launchd `ssh-agent` has 256 file descriptors; leaked connections once exhausted them, after which the agent accepted connections but never answered and logins hung in `agent.List`. Agent access goes through `AgentDialer` (`internal/ssh/agent.go`), and tests pass a fake or nil dialer, never the user's agent: an empty `SSH_AUTH_SOCK` does not disable it on macOS, where the launchd socket is the fallback.
