---
name: release
description: Publish a beta or production release of SSH Tunnel Manager. Use when asked to release, publish, tag or cut a version, a beta or a stable build, or to promote a beta to production.
---

# Release SSH Tunnel Manager

Pushing a version tag is the whole release. CI tests, builds, signs and publishes from the tag. Merging publishes no release. `AGENTS.md` describes the branches: `beta` holds what is in beta and `main` what is in production.

Never publish a tag or release without Artur's explicit approval of the version and notes. Never force-push or delete a published tag without asking.

## Beta or production

| | Beta | Production |
|---|---|---|
| Tag | `vX.Y.Z-beta.N` | `vX.Y.Z` |
| Commit | Current `beta` | Merge of the last beta into `main` |
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

1. **Check the state.** On `beta`, clean, and up to date with `git pull --ff-only`. Run `git log HEAD..origin/main` and align `beta` first if it lists anything.
2. **Test.** CI repeats all of this for the tag and refuses to build or publish if anything fails, including the end-to-end tests and a smoke run of the built app on every platform.
   ```
   (cd frontend && npm run check && npm test && npm run build)
   go test ./... -count=1
   (cd frontend && npm run test:e2e)
   ```
3. **Read what ships.** `git log <previous-tag>..HEAD --no-merges --pretty='%h %s'`
4. **Write the notes** to `/tmp/release-notes/<version>.md`, in the format below.
5. **Write the changelog entry** with the `changelog` skill, unless the release has no user-visible change.
6. **Get approval.** Show the version, notes and the changelog preview to Artur, and wait.
7. **Merge the entry to `beta`** and pull. Pages deploy on the merge, so the link in the notes works once the release is out.
8. **Tag and push.** The tag must be annotated, because CI reads the release title and body from its message.
   ```
   git tag -a <version> -F /tmp/release-notes/<version>.md
   git push origin <version>
   ```
9. **Watch CI and verify** as described below.
10. **Check the dev preview** as described under After publishing.

## Publish production

Production promotes a beta that was tested: the tagged commit on `main` has the same files as the last beta, apart from the site entry for the release.

1. **Confirm the beta is good.** Ask Artur whether it was tested. Any new fix goes out as another beta first.
2. **Check the site and docs.** The PR template requires the Features page (`docs/features/`, `docs/pl/features/`), README and comparison page to match. The site root deploys from `main`, so these go live when the promotion merges, just before the tag. Merge straight before tagging.
3. **Write the notes** for everything since the previous production release, not since the last beta. Add a changelog entry for the version that summarizes its betas, with the `changelog` skill. End with `Tested as vX.Y.Z-beta.1 to vX.Y.Z-beta.N.` and the Full Changelog line.
4. **Get approval.** Show the version and notes to Artur, and wait.
5. **Promote the beta.** Merge the changelog entry to `beta`, then open a PR from `beta` to `main` and list `Fixes #N` for each issue it ships. If `beta` holds other commits after the last beta tag, create `release/<version>` at that tag instead, add the entry there and open the PR from it. Merge with a merge commit, not a squash or rebase. Check that only the entry differs: `git diff --stat <last-beta-tag> origin/main`.
6. **Tag the merge commit on `main`.**
   ```
   git switch main && git pull --ff-only
   git tag -a <version> -F /tmp/release-notes/<version>.md
   git push origin <version>
   ```
7. **Watch CI and verify** as described below.
8. **Align `beta` with `main`** as `AGENTS.md` describes: `git switch beta && git pull --ff-only && git merge origin/main && git push origin beta`.

## Publish a production hotfix

For a fix that cannot wait for a beta, only with Artur's approval. Branch from `main`, open a PR to `main`, then follow steps 3, 4, 6, 7 and 8 above with the next patch version.

## Notes format

The first non-empty line is the release title, and the rest is the body. Write them in English.

```markdown
Short title of what changed

- What users notice first. Group under bold headings if there are many changes.
- Use backticks for code and paths. Reference issues as (#N).
- Skip refactors and dependency bumps with no user impact.

**Known limits in this beta**

- Anything untested or missing. Betas only.

**What's new**: https://justcodepl.github.io/ssh-tunnel-manager/changelog/<version>/

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

The dev preview updates itself: merging to `main` or `beta` runs `dev.yml`, so it shows the same site as production. Nothing to run by hand. Check `https://ssh-tunnel-manager.justcode.ovh/changelog/<version>/` once `gh run list --workflow=dev.yml --limit 1` shows the run succeeded.

Update tracking as `AGENTS.md` describes, keeping implemented, beta and production as separate states. Describe something as released only after the release and its assets exist.
