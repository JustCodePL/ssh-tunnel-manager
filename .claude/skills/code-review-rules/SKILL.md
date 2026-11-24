---
name: code-review-rules
description: Project-specific traps for reviewing SSH Tunnel Manager changes. Read by the pre-commit review (personal-code-review) together with AGENTS.md.
---

# Code review rules for SSH Tunnel Manager

- **Vendored code must be committed whole.** A change that adds or updates files under `frontend/vendor/` checks `git status --ignored` and `git check-ignore -v` for the new tree. A nested folder (noVNC's `vendor/pako`) was once matched by a broad `.gitignore` pattern, so a clean checkout could not build.
