# Repository instructions

## Notion tracking

- Track every non-trivial bug fix or improvement for this repository in the Notion database `Zadania` (https://app.notion.com/p/c4716053c9604d06a61b18a085b118e7), project `SSH Tunnel Manager`. It holds everything copied from Linear on 2026-11-04, under the same `JUS-` identifiers; new tasks need no identifier. Reuse an existing task when it already covers the request.
- The built-in RDP view keeps its own page, `SSH Tunnel Manager: wbudowany widok RDP przez IronRDP` (https://app.notion.com/p/40d238194432814f9011f808494b14ab).
- Keep the task's status aligned with the actual work state. A commit or completed implementation is not the same as a published release; record both states explicitly.
- Record the implementation commit SHA and the relevant verification results in the task. Do not add a GitHub commit link until the commit has been pushed and the link resolves.
- Set the task's release field to the SemVer version it ships in. Derive the next version from the latest release or Git tag: a patch increment for a backward-compatible bug fix, a minor increment for a backward-compatible feature.
- Never publish a tag, GitHub Release, or other externally visible release without Artur's explicit approval.

## Public GitHub issue tracking

- Track every public issue for this repository in the JustCodePL organization project at `https://github.com/orgs/JustCodePL/projects/1`.
- Deliver every issue-backed change and every bug fix through a pull request; do not commit those changes directly to the default branch.
- Link the pull request to its GitHub issue by using a closing keyword such as `Fixes #<number>` in the pull request description, unless the issue must intentionally remain open after merge; in that case, use an explicit non-closing issue link.
- Attach the pull request URL to the corresponding Notion task as soon as the pull request is created. Keep the GitHub issue, pull request, and Notion task mutually traceable.
- Keep the public issue updated automatically with concise English comments whenever its distribution state materially changes, including when the implementation is completed, when it is published in a beta or other prerelease, and when it reaches a stable release.
- State the exact verified availability in each comment. Include the commit SHA only after it is pushed and the GitHub commit link resolves; include the release version and public release link only after the corresponding release and artifacts have been published and verified.
- Do not describe a local commit or an untagged commit on a branch as released. Keep implementation, beta publication, and stable publication as explicit separate states.

## Distribution labels

- Keep the `Labels` of a Notion task aligned with the highest distribution channel that actually contains the change: `develop` when it is merged but unpublished, `beta` once a prerelease is published and verified, neither once it is in a stable release. Preserve unrelated labels.

## Project site and feature documentation

- The project site lives in `docs/` and is deployed to GitHub Pages on every merge to `main`. Each user-visible feature has a section on the Features page: `docs/features/index.html` (English) and `docs/pl/features/index.html` (Polish), with a stable anchor such as `#local-forwarding`.
- Every change to a user-visible feature (new feature, changed UI or behaviour, removal) updates the matching Features section in both languages in the same pull request. Add a section for a new feature, rewrite it for a changed one, and remove it for a removed one. Describe only what the code does; do not announce planned features.
- When the UI changes, list the screenshots that must be refreshed (file names under `docs/assets/screenshots/`) in the pull request description. Marketing regenerates the screenshot set with its demo script for each release and delivers files named `v<version>-...`; replace the files on the site and keep `alt`, `width` and `height` accurate.
- When a change affects claims elsewhere on the site, update them in the same pull request too: the home page, `docs/compare/` (EN/PL), the README (Features and Limitations) and `docs/sitemap.xml` for new pages.
- Each release has its own page on the site, written with the `changelog` skill: entries in `scripts/changelog/entries/<language>/<version>.html`, screenshots under `docs/assets/changelog/<version>/`, the version added to `docs/changelog-versions.js`. `node scripts/build-changelog.cjs` writes `docs/changelog/<version>/`, `docs/pl/changelog/<version>/`, both news pages and the sitemap; commit its output.
- Run `node --test docs/*.test.cjs` before opening the pull request; the tests check the analytics scripts, the Umami event attributes and the Features sections.
