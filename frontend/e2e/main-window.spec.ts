import { expect, test } from "@playwright/test";
import { connect, disconnect, expand, openMain, toggle, tunnel } from "./helpers";

test.describe("title bar", () => {
  test.describe("on macOS", () => {
    test.use({ userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)" });
    test("leaves the buttons to the system", async ({ page }) => {
      await openMain(page);
      await expect(page.locator(".titlebar")).toHaveCount(0);
      await expect(page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--titlebar-height").trim())).resolves.toBe("0px");
    });
  });
  test.describe("on Windows", () => {
    test.use({ userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)" });
    test("keeps its own bar and buttons", async ({ page }) => {
      await openMain(page);
      await expect(page.locator(".titlebar .win-btn")).toHaveCount(3);
      await expect(page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--titlebar-height").trim())).resolves.toBe("36px");
    });
  });
});

test.describe("main window", () => {
  test("groups tunnels and keeps forwards behind expanding", async ({ page }) => {
    await openMain(page);

    const production = page.locator('[data-testid="group"][data-group="Production"]');
    await expect(production).toContainText("prod-web");
    await expect(production).toContainText("prod-db");
    await expect(page.locator('[data-testid="group"][data-group="Staging"]')).toContainText("stage-a");
    await expect(page.locator('[data-testid="group"][data-group="Unassigned"]')).toContainText("laptop");

    const row = tunnel(page, "prod-web");
    await expect(row).toContainText("4 forwards");
    await expect(row).not.toContainText("127.0.0.1:15432");

    await expand(page, "prod-web");
    await expect(row.getByTestId("forward")).toHaveCount(4);
    await expect(row).toContainText("127.0.0.1:15432");
    await expect(row).toContainText("to 10.0.0.1:3389");
  });

  test("copying a forward's address keeps it in place and shows a notification", async ({ page }) => {
    await openMain(page);
    await expand(page, "prod-web");
    const address = tunnel(page, "prod-web").getByTestId("forward").first().locator("code");
    await expect(address).toHaveText("127.0.0.1:15432");
    await address.click();
    await expect(page.getByText("Copied 127.0.0.1:15432")).toBeVisible();
    await expect(address).toHaveText("127.0.0.1:15432");
  });

  test("collapses a group from its band", async ({ page }) => {
    await openMain(page);
    const staging = page.locator('[data-testid="group"][data-group="Staging"]');
    await staging.locator(".band").click();
    await expect(staging).not.toContainText("stage-a");
    await staging.locator(".band").click();
    await expect(staging).toContainText("stage-a");
  });

  test("the switch connects and disconnects a tunnel", async ({ page }) => {
    await openMain(page);
    await connect(page, "laptop");
    await expect(toggle(page, "laptop")).toHaveAttribute("aria-checked", "true");
    await disconnect(page, "laptop");
    await expect(toggle(page, "laptop")).toHaveAttribute("aria-checked", "false");
  });

  test("a failed tunnel shows its reason in the row and stops", async ({ page }) => {
    await openMain(page);
    await toggle(page, "broken-host").click();
    const row = tunnel(page, "broken-host");
    await expect(row).toHaveAttribute("data-status", "error", { timeout: 40_000 });
    await expect(row.locator(".dot")).toHaveText("!");
    await expect(row.locator(".reason")).not.toBeEmpty();
    await expect(toggle(page, "broken-host")).toHaveAttribute("aria-checked", "false");
    await expect(row.getByText("Connect", { exact: true })).toHaveCount(0);
  });

  test("filters by search and by chips", async ({ page }) => {
    await openMain(page);
    await page.getByTestId("search").fill("stage");
    await expect(tunnel(page, "stage-a")).toBeVisible();
    await expect(tunnel(page, "prod-web")).toHaveCount(0);
    await page.getByTestId("search").fill("");

    await connect(page, "stage-a");
    await page.getByTestId("filter-live").click();
    await expect(tunnel(page, "stage-a")).toBeVisible();
    await expect(tunnel(page, "laptop")).toHaveCount(0);
    await page.getByRole("tab", { name: /All/ }).click();
    await disconnect(page, "stage-a");
  });

  test("connects a whole group", async ({ page }) => {
    await openMain(page);
    const band = page.locator('[data-testid="group"][data-group="Staging"] .band');
    await band.getByRole("button", { name: "Connect all" }).click();
    await expect(tunnel(page, "stage-a")).toHaveAttribute("data-status", "connected", { timeout: 20_000 });
    await band.getByRole("button", { name: "Disconnect all" }).click();
    await expect(tunnel(page, "stage-a")).toHaveAttribute("data-status", "disconnected");
  });

  test("keyboard moves through tunnels and expands", async ({ page }) => {
    await openMain(page);
    await page.keyboard.press("j");
    await expect(page.locator(".row.selected")).toHaveCount(1);
    const selected = page.locator('[data-testid="tunnel"]:has(.row.selected)');
    await page.keyboard.press("Enter");
    await expect(selected.locator(".row")).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Enter");
    await expect(selected.locator(".row")).toHaveAttribute("aria-expanded", "false");
  });

  test("a new tunnel can start in a group, and a group can exist before its first tunnel", async ({ page }) => {
    await openMain(page);
    const staging = page.locator('[data-testid="group"][data-group="Staging"]');
    await staging.getByTestId("add-to-group").click();
    await expect(page.getByTestId("group-input")).toHaveValue("Staging");
    await page.locator(".cancel-btn").click();

    await page.getByTestId("new-menu").click();
    await page.getByTestId("new-group").click();
    await page.getByTestId("new-group-name").fill("Empty one");
    await page.getByTestId("new-group-name").press("Enter");
    const created = page.locator('[data-testid="group"][data-group="Empty one"]');
    await expect(created).toContainText("0 tunnels");
    await created.getByTestId("add-to-group").click();
    await expect(page.getByTestId("group-input")).toHaveValue("Empty one");
    await page.locator(".cancel-btn").click();
    await created.getByTestId("delete-group").click();
    await expect(created).toHaveCount(0);
  });

  test("the group dropdown shows what does not fit, and the top bar has its own buttons", async ({ page }) => {
    await openMain(page);
    await expect(page.getByTestId("more")).toHaveCount(0);
    await expect(page.getByTestId("open-settings")).toBeVisible();
    await expect(page.getByTestId("more-groups")).toHaveCount(0);

    // The tunnel's settings sit right before its switch, without expanding it.
    const controls = tunnel(page, "laptop").locator(".row").locator('[data-testid="tunnel-settings"], [role="switch"]');
    await expect(controls.nth(0)).toHaveAttribute("data-testid", "tunnel-settings");
    await expect(controls.nth(1)).toHaveAttribute("role", "switch");

    await connect(page, "laptop");
    await expect(page.getByTestId("disconnect-all")).toBeVisible();

    await page.setViewportSize({ width: 340, height: 700 });
    await page.getByTestId("more-groups").click();
    const hidden = page.getByRole("menuitem", { name: /Unassigned/ });
    await expect(hidden).toBeVisible();
    await hidden.click();
    await expect(tunnel(page, "laptop")).toBeVisible();
    await expect(tunnel(page, "prod-web")).toHaveCount(0);
    await page.getByRole("tab", { name: /All/ }).click();
    await page.setViewportSize({ width: 1100, height: 700 });

    await page.getByTestId("disconnect-all").click();
    await expect(tunnel(page, "laptop")).toHaveAttribute("data-status", "disconnected");
    await expect(page.getByTestId("disconnect-all")).toHaveCount(0);
  });
});
