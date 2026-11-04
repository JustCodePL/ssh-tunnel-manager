import { expect, test } from "@playwright/test";
import { closeTools, connect, expand, openMain, toolWindow, toggle, tunnel } from "./helpers";

// Not a check: saves screenshots of every screen when STM_E2E_SCREENSHOTS names
// a directory.
const dir = process.env.STM_E2E_SCREENSHOTS;

test.describe("screenshots", () => {
  test.skip(!dir, "set STM_E2E_SCREENSHOTS to save screenshots");

  for (const theme of ["terminal", "graphite"] as const) {
    test(`every screen in ${theme}`, async ({ page, context }) => {
      test.setTimeout(180_000);
      // A notification left over from an earlier step would cover the screen.
      const shot = async (target: typeof page, name: string) => {
        await expect(target.locator(".toast")).toHaveCount(0, { timeout: 20_000 });
        await target.screenshot({ path: `${dir}/${theme}-${name}.png` });
      };

      await openMain(page);
      const settings = await toolWindow(context, () => page.getByTestId("open-settings").click());
      await settings.getByTestId("settings-appearance").click();
      await settings.getByTestId(`theme-${theme}`).click();
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);

      await shot(page, "01-list");
      await connect(page, "prod-web");
      await connect(page, "stage-a");
      if ((await tunnel(page, "broken-host").getAttribute("data-status")) !== "error") {
        await toggle(page, "broken-host").click();
        await expect(tunnel(page, "broken-host")).toHaveAttribute("data-status", "error", { timeout: 40_000 });
      }
      await expand(page, "prod-web");
      await expect(tunnel(page, "prod-web").getByTestId("attach")).toHaveAttribute("aria-haspopup", "menu", { timeout: 15_000 });
      await shot(page, "02-expanded");
      await tunnel(page, "prod-web").getByTestId("attach").click();
      await shot(page, "03-attach-menu");
      await page.keyboard.press("Escape");
      await page.locator("body").click({ position: { x: 5, y: 5 } });

      await page.keyboard.press("ControlOrMeta+k");
      await page.getByTestId("palette-input").fill("term");
      await shot(page, "04-palette");
      await page.keyboard.press("Escape");

      await settings.setViewportSize({ width: 900, height: 620 });
      for (const section of ["general", "appearance", "windows", "monitoring", "credentials", "maintenance", "updates"]) {
        await settings.getByTestId(`settings-${section}`).click();
        await settings.waitForTimeout(150);
        if (section === "credentials" && (await settings.getByTestId("credential-row").count()) === 0) {
          await settings.getByTestId("credential-create").click();
          await settings.getByTestId("credential-name").fill("Office admin");
          await settings.getByTestId("credential-user").fill("administrator");
          await settings.getByTestId("credential-password").fill("example");
          await settings.getByTestId("credential-save").click();
          await expect(settings.getByTestId("credential-row")).toHaveCount(1);
        }
        await shot(settings, `10-settings-${section}`);
      }

      const own = await toolWindow(context, () => tunnel(page, "prod-web").getByTestId("tunnel-settings").click());
      await own.setViewportSize({ width: 900, height: 660 });
      for (const section of ["connection", "forwards", "credentials", "row", "monitoring", "windows"]) {
        await own.getByTestId(`tunnel-${section}`).click();
        await own.waitForTimeout(200);
        // The credentials page is the longest.
        await own.setViewportSize({ width: 900, height: section === "credentials" ? 860 : 660 });
        await shot(own, `20-tunnel-${section}`);
      }

      const button = (name: string) => tunnel(page, "prod-web").locator(".panel-head").getByRole("button", { name, exact: true });
      const terminal = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("attach").click());
      await terminal.setViewportSize({ width: 860, height: 500 });
      await terminal.getByTestId("terminal").locator(".xterm-helper-textarea").last().pressSequentially("uptime\n");
      await terminal.getByTestId("new-tab").click();
      await expect(terminal.getByTestId("tab")).toHaveCount(2);
      await terminal.waitForTimeout(500);
      await shot(terminal, "30-terminal");

      const files = await toolWindow(context, () => button("Files").click());
      await files.setViewportSize({ width: 960, height: 520 });
      await expect(files.getByTestId("files-table")).toBeVisible();
      await files.locator('[data-testid="file-row"][data-name="deploy.sh"]').hover();
      await shot(files, "31-files");

      const htop = await toolWindow(context, () => button("htop").click());
      await htop.setViewportSize({ width: 860, height: 520 });
      await expect(htop.getByTestId("process-table")).toBeVisible();
      await shot(htop, "32-htop");

      const docker = await toolWindow(context, () => button("Docker").click());
      await docker.setViewportSize({ width: 960, height: 560 });
      await docker.getByTestId("docker-list").getByText("db", { exact: true }).click();
      await expect(docker.getByTestId("docker-detail")).toContainText("postgres:16");
      await shot(docker, "33-docker");

      const disk = await toolWindow(context, () => button("Disk").click());
      await disk.setViewportSize({ width: 760, height: 320 });
      await expect(disk.getByTestId("disk-list")).toBeVisible();
      await shot(disk, "34-disk");

      const logs = await toolWindow(context, () => button("Logs").click());
      await logs.setViewportSize({ width: 800, height: 360 });
      await shot(logs, "35-logs");

      const rdp = await toolWindow(context, () => tunnel(page, "prod-web").getByTestId("forward").nth(1).getByRole("button", { name: "Attach" }).click());
      await rdp.setViewportSize({ width: 900, height: 640 });
      await expect(rdp.locator(".rdp-form")).toBeVisible();
      await shot(rdp, "36-rdp-login");

      await page.getByTestId("new-menu").click();
      await page.getByTestId("new-tunnel").click();
      await expect(page.getByTestId("tunnel-form")).toBeVisible();
      await shot(page, "40-new-tunnel");
      await page.locator(".close-btn").first().click();

      await closeTools(context, page);
    });
  }
});
