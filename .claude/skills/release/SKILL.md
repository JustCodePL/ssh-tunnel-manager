---
name: release
description: Publish a beta or production release of SSH Tunnel Manager. Use when asked to release, publish, tag or cut a version, a beta or a stable build, or to promote a beta to production.
---

# Release SSH Tunnel Manager

Pushing a version tag is the whole release. CI tests, builds, signs and publishes from the tag. There is no `develop` branch: work is merged to `main` through PRs, and merging publishes nothing.

Never publish a tag or release without Artur's explicit approval of the version and notes. Never force-push or delete a published tag without asking.

## Beta or production

| | Beta | Production |
|---|---|---|
| Tag | `vX.Y.Z-beta.N` | `vX.Y.Z` |
| Commit | Current `main` | Normally the same commit as the last beta |
| GitHub release | Pre-release, not Latest | Latest |
| Who receives it | Beta channel only | Stable and beta channels |

The tag name alone decides this (`.github/workflows/build.yml`). A tag in any other format fails CI. Beta users advance to the final release automatically.

## Pick the version

Find the latest versions with `git tag --sort=-v:refname | head` and `gh release list --limit 5`.

- **New feature:** next minor, `v1.2.0-beta.1`.
- **Bug fix:** next patch, `v1.2.1-beta.1`.
- **Next beta of the same version:** raise N, `v1.2.0-beta.2`. N starts at 1.
- **Production:** drop the suffix, `v1.2.0`.

Nothing else needs bumping. The app takes its version from the tag at build time.

## Publish a beta

1. **Check the state.** On `main`, clean, and up to date with `git pull --ff-only`.
2. **Test.** Build the frontend first, because `main.go` embeds `frontend/dist`. CI only runs `go test ./internal/...`.
   ```
   (cd frontend && npm test && npm run build)
   go test . ./internal/... -count=1
   ```
3. **Read what ships.** `git log <previous-tag>..HEAD --no-merges --pretty='%h %s'`
4. **Write the notes** to `/tmp/release-notes/<version>.md`, in the format below.
5. **Get approval.** Show the version and notes to Artur, and wait.
6. **Tag and push.** The tag must be annotated, because CI reads the release title and body from its message.
   ```
   git tag -a <version> -F /tmp/release-notes/<version>.md
   git push origin <version>
   ```
7. **Watch CI and verify** as described below.

## Publish production

Production promotes a beta that was tested. `v1.1.0` and `v1.1.0-beta.5` are the same commit.

1. **Confirm the beta is good.** Ask Artur whether it was tested. Any new fix goes out as another beta first.
2. **Check the site and docs.** The PR template requires the Features page (`docs/features/`, `docs/pl/features/`), README and comparison page to match. Pages deploy on push to `main`, so a page describing an unreleased feature goes live before its release.
3. **Write the notes** for everything since the previous production release, not since the last beta. End with `Tested as vX.Y.Z-beta.1 to vX.Y.Z-beta.N.` and the Full Changelog line.
4. **Get approval.** Show the version and notes to Artur, and wait.
5. **Tag the beta's commit**, not the tip of `main`, which may contain untested changes.
   ```
   git tag -a <version> -F /tmp/release-notes/<version>.md <last-beta-tag>^{commit}
   git push origin <version>
   ```
6. **Watch CI and verify** as described below.

## Notes format

The first non-empty line is the release title, and the rest is the body. Write them in English.

```markdown
Short title of what changed

- What users notice first. Group under bold headings if there are many changes.
- Use backticks for code and paths. Reference issues as (#N).
- Skip refactors and dependency bumps with no user impact.

**Known limits in this beta**

- Anything untested or missing. Betas only.

**Full Changelog**: https://github.com/JustCodePL/ssh-tunnel-manager/compare/<previous-tag>...<version>
```

Compare a beta with the previous release: `v1.1.0...v1.2.0-beta.1`. A later beta states what is new and adds that it contains the earlier betas.

## Watch CI and verify

Find the run with `gh run list --workflow=build.yml --limit 1`, then wait with `gh run watch <id> --exit-status`. If a job fails, investigate before calling the release done.

```
gh release view <version> --json name,isPrerelease,assets --jq '{name,isPrerelease,assets:[.assets[].name]}'
gh release list --limit 3
```

- The title and body match the notes.
- `isPrerelease` is `true` only for a beta, and `gh release list` shows the right Latest.
- All 5 assets are there: `ssh-tunnel-manager-linux-amd64.tar.gz`, `ssh-tunnel-manager-darwin-arm64.dmg`, `ssh-tunnel-manager-darwin-amd64.dmg`, `ssh-tunnel-manager-windows-amd64.zip`, `ssh-tunnel-manager-amd64-installer.exe`.

To fix a body afterwards: `gh release edit <version> --notes-file <file>`. That file holds only the body, without the title line.

## After publishing

Update tracking as `AGENTS.md` describes, keeping implemented, beta and production as separate states. Describe something as released only after the release and its assets exist.
