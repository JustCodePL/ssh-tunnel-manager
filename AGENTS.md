# Repository instructions

## Linear tracking

- Track every non-trivial bug fix or improvement for this repository in the Linear team `JustCode - Artur Czuba` and project `SSH Tunnel Manager`. Reuse an existing issue when it already covers the request; otherwise create one.
- Keep the Linear issue status aligned with the actual work state. A commit or completed implementation is not the same as a published release; record both states explicitly.
- Record the implementation commit SHA and the relevant verification results in the issue. Do not add a GitHub commit link until the commit has been pushed and the link resolves.
- Link each issue intended for distribution to the corresponding native Linear Release.
- If the target Release does not exist, derive the next version from the latest existing release or Git tag using SemVer: use a patch increment for a backward-compatible bug fix, a minor increment for a backward-compatible feature, and a major increment for a breaking change. Use a prerelease suffix only when the request explicitly targets a prerelease. Create the Release in the appropriate pipeline and link the issue to it.
- If native Linear Releases are unavailable for the workspace or plan, do not enable a trial or upgrade the plan automatically. Use a project milestone named after the SemVer version (for example, `v1.0.32`) as the fallback, attach the issue to it, and note the Releases limitation in the issue.
- Never publish a tag, GitHub Release, or other externally visible release without Artur's explicit approval.

## Public GitHub issue tracking

- Track every public issue for this repository in the JustCodePL organization project at `https://github.com/orgs/JustCodePL/projects/1`.
- Deliver every issue-backed change and every bug fix through a pull request; do not commit those changes directly to the default branch.
- Link the pull request to its GitHub issue by using a closing keyword such as `Fixes #<number>` in the pull request description, unless the issue must intentionally remain open after merge; in that case, use an explicit non-closing issue link.
- Attach the pull request URL to the corresponding Linear issue as soon as the pull request is created. Keep the GitHub issue, pull request, and Linear issue mutually traceable.
- Keep the public issue updated automatically with concise English comments whenever its distribution state materially changes, including when the implementation is completed, when it is published in a beta or other prerelease, and when it reaches a stable release.
- State the exact verified availability in each comment. Include the commit SHA only after it is pushed and the GitHub commit link resolves; include the release version and public release link only after the corresponding release and artifacts have been published and verified.
- Do not describe a local commit or an untagged commit on a branch as released. Keep implementation, beta publication, and stable publication as explicit separate states.

## Linear distribution labels

- Keep Linear issue labels aligned with the highest distribution channel that actually contains the change, following the team document `Sposób pracy: Artur + Codex`.
- Apply the `develop` label when a change is implemented on the development branch but has not yet been published in a beta or stable release.
- When a beta or other prerelease is successfully published and verified, replace `develop` with `beta` on every issue included in that prerelease.
- When the change reaches a stable release, remove both `develop` and `beta`. Preserve all unrelated labels throughout these transitions.
- Determine the included issues from the linked Linear Release or milestone, the commits since the previous release, and the published release notes. Do not label unrelated diagnostic or administrative issues as part of a product release.
