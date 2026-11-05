---
name: changelog
description: Write a release's changelog entry for the project site, reproducing what changed in the real app and taking screenshots on MacBook-Pro-Artur-Agent. Use before every beta or production release, or when asked for a changelog entry, a "what's new" page or release screenshots.
---

# Changelog entry

Every release has an entry on the changelog page, and its GitHub release links to it. The entry says what changed and how to use it, with screenshots of the real app. Run this before tagging: see the `release` skill.

## 1. Decide what to show

Read what ships: `git log <previous-tag>..HEAD --no-merges --pretty='%h %s'`, then the diffs for anything user-visible. Skip refactors and internals. For each change, write down the state a user has to reach to see it.

A later beta's entry covers only what is new since the previous beta.

## 2. Reproduce and capture on the agent Mac

Screenshots come from `MacBook-Pro-Artur-Agent`, from the commit being released. Run there with `delegate_task` and `target.environmentId`. Over `ssh macbook-agent` the app runs but `screencapture` can't read its window, because SSH has no Screen Recording permission; if that is still so, capture on Artur's Mac, where the isolated copy works too. It has `go` and `swiftc` in a login shell (`zsh -lc`), and Node in `~/.local/node/bin`, which has to be added to `PATH`. Don't use `brew install node`: this Intel Mac on macOS 13 builds it from source for over 30 minutes.

```
cd ~/sources/justcode/ssh-tunnel-manager
git fetch && git checkout --detach <commit>
scripts/changelog-capture.sh scripts/changelog-scenarios/<scenario>.sshconfig /tmp/changelog/<scenario> 30
```

- The script runs an isolated copy with a temporary `HOME`. Never drive the installed app or the real `~/.ssh/config`.
- A scenario is an SSH config for that temporary `HOME`. Add one to `scripts/changelog-scenarios/` per state. `# stm:autoconnect=true` starts a tunnel without a click. Use `192.0.2.x` for a host that must fail, and no real hostnames, users or IPs.
- `frontend/e2e/screenshots.spec.ts` saves every screen in both themes from the real backend in Chromium, without native title bars: set `STM_E2E_SCREENSHOTS=<dir>` and run `npm run test:e2e -- screenshots`. Use it when no Mac with Screen Recording permission is at hand, and say so in the entry's review.
- A state that needs a click: pass a driver as the script's fourth argument, JavaScript that clicks through the page. See `scripts/changelog-scenarios/rdp-lexar.driver.js`. Don't use `osascript` with System Events: over SSH on the agent Mac it hangs waiting for an Accessibility prompt.
- A state that needs a real server (RDP, Docker): use `lexar`. Get the login with `request_secret` and `delivery: "file"`, copy it to the agent Mac, set `CHANGELOG_LOGIN_FILE`, and delete the copy afterwards. Reach `lexar` through the `nas` bastion by setting `CHANGELOG_BASTION`, `CHANGELOG_BASTION_USER` and `CHANGELOG_RDP_TARGET`. SSH on its Tailscale address asks for a browser check and the app waits on it. Keep addresses out of the repo. Ask Artur to minimize work windows on `lexar` first, because the session shows the real desktop. Check every frame for real addresses, logins and desktop content before using it, and delete rejected frames. A session can stall on `connecting...`: keep the login file until you have a good frame.

**Look at every frame you keep.** Check that it shows the change and that the app really did what the text will claim, using the app log the script prints. If a change can't be reproduced, say so instead of describing it from the code.

Copy the chosen frames back to `docs/assets/changelog/<version>/` with descriptive names (`tunnel-failed.png`).

## 3. Write the entry

Add one file per language: `scripts/changelog/entries/en/<version>.html` and `scripts/changelog/entries/pl/<version>.html`, each holding one `<article class="changelog-entry" id="<version>">`. Copy the structure of the previous release, and use `@ROOT@/assets/...` for images. Each has:

- The tag, a badge (`BETA`, or `STABLE` with the class `release-badge`), the date and a plain title.
- One `feature-detail` section per change: what is different, with its screenshot. Use the `no-shot` class only when there is nothing to show.
- **How to use it:** the steps, naming the actual buttons.
- **Known limits:** what doesn't work or wasn't tested. Same honesty as the release notes.
- A link to the GitHub release.

Set `width` and `height` to the file's real size (`sips -g pixelWidth -g pixelHeight <file>`) and write `alt` text. Add the version at the start of the list in `docs/changelog-versions.js`, then run `node scripts/build-changelog.cjs`. It writes the release's page in both languages (`docs/changelog/<version>/`, `docs/pl/changelog/<version>/`), the sidebar and Next and Previous links on every page, the news page, which shows the newest release, and the sitemap. Commit the output: the site deploys `docs/` as it is, and a test fails if the pages are out of date.

Describe only what the code does and what you saw it do. Test with `node --test docs/*.test.cjs`.

## 4. Preview on dev

The dev server is `https://ssh-tunnel-manager.justcode.ovh` (user `preview`, Artur has the password; `noindex`). It serves `docs/` of a commit from GitHub, so the commit must be pushed. Analytics are stripped there.

```
git push -u origin <branch>
~/agent-shared/tools/preview/stm-redeploy.sh "$(git rev-parse HEAD)"
```

Give Artur the link to the entry: `https://ssh-tunnel-manager.justcode.ovh/changelog/<version>/`.

## 5. Connect it to the release

The release notes end with the `What's new` line from the `release` skill's format. The site's release list links to the entry through `docs/changelog-versions.js`. Before tagging, merge the entry to `beta` for a beta, where it appears under `/beta/changelog/<version>/`. A production entry goes to `main` with the promotion PR.
