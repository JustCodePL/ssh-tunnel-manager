# Repository instructions

## Linear tracking

- Track every non-trivial bug fix or improvement for this repository in the Linear team `JustCode - Artur Czuba` and project `SSH Tunnel Manager`. Reuse an existing issue when it already covers the request; otherwise create one.
- Keep the Linear issue status aligned with the actual work state. A commit or completed implementation is not the same as a published release; record both states explicitly.
- Record the implementation commit SHA and the relevant verification results in the issue. Do not add a GitHub commit link until the commit has been pushed and the link resolves.
- Link each issue intended for distribution to the corresponding native Linear Release.
- If the target Release does not exist, derive the next version from the latest existing release or Git tag using SemVer: use a patch increment for a backward-compatible bug fix, a minor increment for a backward-compatible feature, and a major increment for a breaking change. Use a prerelease suffix only when the request explicitly targets a prerelease. Create the Release in the appropriate pipeline and link the issue to it.
- If native Linear Releases are unavailable for the workspace or plan, do not enable a trial or upgrade the plan automatically. Use a project milestone named after the SemVer version (for example, `v1.0.32`) as the fallback, attach the issue to it, and note the Releases limitation in the issue.
- Never publish a tag, GitHub Release, or other externally visible release without Artur's explicit approval.
