Cut a new release of SSH Tunnel Manager.

Optional `$ARGUMENTS`: target version (e.g. `v1.0.23`). If omitted, bump patch from the latest tag.

## Steps

1. **Verify clean state** — `git status` shows nothing uncommitted; current branch is `main`; `git pull` is up to date.
2. **Pick version** — if `$ARGUMENTS` is set, use it; otherwise read latest tag via `git describe --tags --abbrev=0` and bump the patch (`v1.0.X` → `v1.0.X+1`).
3. **Gather commits** — `git log <prev-tag>..HEAD --no-merges --pretty=format:'%h %s'` to see what shipped since the last release.
4. **Draft release notes** — write a markdown summary to `C:/tmp/release-notes/<version>.md`. Style rules:
   - Bullet list, user-visible changes first. Group by area if there are many (e.g. **Terminal**, **CI**, **Windows**).
   - Lead with features/fixes the user will notice. Skip pure refactors and dependency bumps unless they have user impact.
   - Use backticks for code/paths. Reference issues as `(#N)` when commits mention them.
   - End with: `**Full Changelog**: https://github.com/JustCodePL/ssh-tunnel-manager/compare/<prev-tag>...<version>`
5. **Show notes to user, get confirmation** — print the notes file and wait. Do not tag until the user approves.
6. **Create annotated tag** — `git tag -a <version> -F C:/tmp/release-notes/<version>.md` (CI reads the tag message as the release body — see [.github/workflows/build.yml:216-219](.github/workflows/build.yml#L216-L219)).
7. **Push tag** — `git push origin <version>`. This triggers the build workflow, which runs tests, builds Linux/macOS/Windows artifacts, and creates the GitHub release.
8. **Watch CI** — `gh run watch` (or `gh run list --workflow=build.yml --limit 1`) until green. If a job fails, investigate before declaring done.
9. **Verify release** — `gh release view <version> --repo JustCodePL/ssh-tunnel-manager` shows the expected body and all 5 assets (linux tar.gz, 2x macOS dmg, windows zip, windows installer exe).

## Notes

- `gh` CLI lives at `C:/Program Files/GitHub CLI/gh.exe` on this machine.
- Release-notes drafts in `C:/tmp/release-notes/*.md` are kept for reference; ok to overwrite.
- If you need to fix a release body after the fact: `gh release edit <version> --repo JustCodePL/ssh-tunnel-manager --notes-file C:/tmp/release-notes/<version>.md`.
- Never push tags with `--force` or delete published tags without asking.
