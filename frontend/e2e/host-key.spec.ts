import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "@playwright/test";
import { disconnect, openMain, toggle, toolWindow, tunnel } from "./helpers";

const here = dirname(fileURLToPath(import.meta.url));
const { home } = JSON.parse(readFileSync(join(here, ".state.json"), "utf8")) as { home: string };
const knownHosts = join(home, ".ssh", "known_hosts");

test.describe("unknown SSH server", () => {
  const name = "prod-db";
  let original = "";

  test.beforeEach(() => {
    original = readFileSync(knownHosts, "utf8");
    writeFileSync(knownHosts, "");
  });

  test.afterEach(() => {
    writeFileSync(knownHosts, original);
  });

  test("connects only after the user trusts its key", async ({ page, context }) => {
    await openMain(page);
    const row = tunnel(page, name);

    await toggle(page, name).click();
    const dialog = page.getByTestId("host-key-dialog");
    await expect(dialog).toBeVisible({ timeout: 20_000 });
    await expect(page.getByTestId("host-key-fingerprint")).toContainText("SHA256:");
    await page.getByTestId("host-key-reject").click();
    await expect(dialog).toBeHidden();
    await expect(row).toHaveAttribute("data-status", "error", { timeout: 20_000 });

    await toggle(page, name).click();
    await expect(dialog).toBeVisible({ timeout: 20_000 });
    await page.getByTestId("host-key-trust").click();
    await expect(dialog).toBeHidden();
    await expect(row).toHaveAttribute("data-status", "connected", { timeout: 20_000 });
    await disconnect(page, name);

    // The accepted key is remembered, so there is no second question.
    await toggle(page, name).click();
    await expect(row).toHaveAttribute("data-status", "connected", { timeout: 20_000 });
    await expect(dialog).toBeHidden();
    await disconnect(page, name);

    const settings = await toolWindow(context, () => tunnel(page, name).getByTestId("tunnel-settings").click());
    await settings.getByTestId("tunnel-credentials").click();
    await expect(settings.getByTestId("ssh-host-key")).toContainText("SHA256:");
    await settings.getByTestId("forget-host-key").click();
    await expect(settings.getByTestId("ssh-host-key")).toHaveCount(0);
    await settings.close();
  });
});
