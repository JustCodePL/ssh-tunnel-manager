import { expect, test } from "@playwright/test";
import { choose, closeTools, connect, disconnect, expand, openMain, toolWindow, tunnel } from "./helpers";

async function openSettings(page: any, context: any) {
  return toolWindow(context, () => page.getByTestId("open-settings").click());
}

test.describe("settings", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("the theme changes every open window", async ({ page, context }) => {
    await openMain(page);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "terminal");
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-appearance").click();
    await settings.getByTestId("theme-graphite").click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "graphite");
    await expect(settings.locator("html")).toHaveAttribute("data-theme", "graphite");
    await settings.getByTestId("theme-terminal").click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "terminal");
  });

  test("the default row applies to tunnels, and a tunnel can override it", async ({ page, context }) => {
    await openMain(page);
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-appearance").click();
    await settings.getByTestId("row-address-row").click();
    await expect(tunnel(page, "laptop").locator(".row code.address")).toBeVisible();
    await settings.getByTestId("row-address-panel").click();
    await expect(tunnel(page, "laptop").locator(".row code.address")).toHaveCount(0);

    await expand(page, "stage-a");
    const own = await toolWindow(context, () => tunnel(page, "stage-a").getByTestId("tunnel-settings").click());
    await own.getByTestId("tunnel-row").click();
    await own.getByTestId("row-files-row").click();
    await expect(tunnel(page, "stage-a").locator(".row[aria-expanded='true']").getByRole("button", { name: /Files on stage-a/ })).toHaveCount(0);
    await own.getByTestId("tunnel-save").click();
    await tunnel(page, "stage-a").locator(".row").click(); // tools leave the row while it is expanded
    await expect(tunnel(page, "stage-a").locator(".row").getByRole("button", { name: /Files on stage-a/ })).toBeVisible();
    await expect(tunnel(page, "laptop").locator(".row").getByRole("button", { name: /Files on laptop/ })).toHaveCount(0);
    await own.getByRole("button", { name: "Use the default row" }).click();
    await own.getByTestId("tunnel-save").click();
    await expect(tunnel(page, "stage-a").locator(".row").getByRole("button", { name: /Files on stage-a/ })).toHaveCount(0);
  });

  test("Apply to group copies a row once", async ({ page, context }) => {
    await openMain(page);
    await expand(page, "prod-db");
    const own = await toolWindow(context, () => tunnel(page, "prod-db").getByTestId("tunnel-settings").click());
    await own.getByTestId("tunnel-row").click();
    await own.getByTestId("row-logs-row").click();
    await own.getByTestId("apply-to-group").click();
    await tunnel(page, "prod-web").locator(".row[aria-expanded='true']").click({ timeout: 1000 }).catch(() => {});
    await expect(tunnel(page, "prod-web").locator(".row").getByRole("button", { name: /Logs on prod-web/ })).toBeVisible();

    // A later change does not follow.
    await own.getByTestId("row-logs-panel").click();
    await own.getByTestId("tunnel-save").click();
    await expect(tunnel(page, "prod-web").locator(".row").getByRole("button", { name: /Logs on prod-web/ })).toBeVisible();
  });

  test("a tunnel's refresh override is saved next to the default", async ({ page, context }) => {
    await openMain(page);
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-monitoring").click();
    await expect(settings.getByTestId("refresh-docker")).toHaveAttribute("data-value", "0");
    await choose(settings, "refresh-htop", "10 s");

    await expand(page, "stage-a");
    const own = await toolWindow(context, () => tunnel(page, "stage-a").getByTestId("tunnel-settings").click());
    await own.getByTestId("tunnel-monitoring").click();
    await expect(own.getByTestId("tunnel-refresh-htop")).toHaveText("Default (10 s)");
    await choose(own, "tunnel-refresh-htop", "30 s");
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();
    await own.reload();
    await own.getByTestId("tunnel-monitoring").click();
    await expect(own.getByTestId("tunnel-refresh-htop")).toHaveAttribute("data-value", "30");
    await choose(settings, "refresh-htop", "3 s");
  });

  test("credentials never show a password and can be assigned to a forward", async ({ page, context }) => {
    await openMain(page);
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-credentials").click();
    await expect(settings.getByTestId("credential-form")).toHaveCount(0);
    await settings.getByTestId("credential-create").click();

    await settings.getByTestId("credential-save").click();
    await expect(settings.getByTestId("credential-error")).toContainText("name");

    await settings.getByTestId("credential-name").fill("Office admin");
    await settings.getByTestId("credential-user").fill("administrator");
    await settings.getByTestId("credential-password").fill("s3cret-value");
    await settings.getByTestId("credential-save").click();
    const row = settings.getByTestId("credential-row");
    await expect(row).toContainText("Office admin");
    await expect(row).toContainText("administrator");
    await expect(settings.getByTestId("credential-form")).toHaveCount(0);

    await settings.getByTestId("credential-edit").click();
    await expect(settings.getByTestId("credential-name")).toHaveValue("Office admin");
    await expect(settings.getByTestId("credential-password")).toHaveValue("");
    await expect(settings.getByTestId("credential-password")).toHaveAttribute("placeholder", /Saved/);
    await expect(settings.locator("body")).not.toContainText("s3cret-value");
    await settings.getByTestId("credential-user").fill("root");
    await settings.getByTestId("credential-save").click();
    await expect(row).toContainText("root");
    await settings.getByTestId("credential-edit").click();
    await settings.getByTestId("credential-user").fill("administrator");
    await settings.getByTestId("credential-save").click();
    await expect(row).toContainText("administrator");

    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const own = await toolWindow(context, () => tunnel(page, "prod-web").getByTestId("tunnel-settings").click());
    await own.getByTestId("tunnel-credentials").click();
    const logins = own.getByTestId("rdp-login");
    await expect(logins).toHaveCount(3); // the host itself and two forwards
    await choose(own, "rdp-login", "Office admin", 1);
    await choose(own, "rdp-login", "Office admin", 2);
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();

    await settings.reload();
    await settings.getByTestId("settings-credentials").click();
    await expect(settings.getByTestId("credential-row")).toContainText("2 targets");
  });

  test("window behaviour settings persist", async ({ page, context }) => {
    await openMain(page);
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-windows").click();
    await expect(settings.getByTestId("window-grouping")).toHaveAttribute("data-value", "tool-target");
    await expect(settings.getByTestId("after-disconnect")).toHaveAttribute("data-value", "resume");
    await choose(settings, "window-grouping", "One window per tool, a tab per target");
    await settings.reload();
    await settings.getByTestId("settings-windows").click();
    await expect(settings.getByTestId("window-grouping")).toHaveAttribute("data-value", "tool");
    await choose(settings, "window-grouping", "One window per tool and target");
  });

  test("grouping by tool puts targets into tabs of one window", async ({ page, context }) => {
    await openMain(page);
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-windows").click();
    await choose(settings, "window-grouping", "One window per tool, a tab per target");

    await connect(page, "prod-web");
    await connect(page, "stage-a");
    await expand(page, "prod-web");
    await expand(page, "stage-a");
    const tool = await toolWindow(context, () => tunnel(page, "prod-web").getByRole("button", { name: "Disk" }).click());
    await tunnel(page, "stage-a").getByRole("button", { name: "Disk" }).click();
    await expect(tool.getByTestId("tab")).toHaveCount(2);

    await choose(settings, "window-grouping", "One window per tool and target");
    await disconnect(page, "stage-a");
  });

  test("closing on disconnect closes the tunnel's tools", async ({ page, context }) => {
    await openMain(page);
    await expand(page, "stage-a");
    const own = await toolWindow(context, () => tunnel(page, "stage-a").getByTestId("tunnel-settings").click());
    await own.getByTestId("tunnel-windows").click();
    await choose(own, "tunnel-after-disconnect", "Close windows");
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();

    await connect(page, "stage-a");
    const tool = await toolWindow(context, () => tunnel(page, "stage-a").getByRole("button", { name: "Disk" }).click());
    await expect(tool.getByTestId("disk-list")).toBeVisible();
    await disconnect(page, "stage-a");
    await expect(tool.getByText("no open tabs")).toBeVisible();

    await choose(own, "tunnel-after-disconnect", /^Default/);
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true }).first()).toBeVisible();
  });

  test("the connection form edits a tunnel, and its forwards live on their own page", async ({ page, context }) => {
    await openMain(page);
    await expand(page, "laptop");
    const own = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("tunnel-settings").click());
    const form = own.getByTestId("tunnel-form");
    await expect(form.locator('input[placeholder="bastion.example.com"]')).toHaveValue("127.0.0.1");
    await own.getByTestId("tunnel-forwards").click();
    await expect(form.locator('input[placeholder="bastion.example.com"]')).toBeHidden();
    await expect(form.getByText("add port forwarding")).toBeVisible();
    await expect(form.getByText("port forwards", { exact: true })).toHaveCount(0);
  });
});

test.describe("shared logins", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("one saved login connects a tunnel over SSH and is offered to its remote desktops", async ({ page, context }) => {
    await openMain(page);
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-credentials").click();
    await settings.getByTestId("credential-create").click();
    await settings.getByTestId("credential-name").fill("Shared login");
    await settings.getByTestId("credential-user").fill("password-user");
    await settings.getByTestId("credential-password").fill("fixture-password");
    await settings.getByTestId("credential-rdp").uncheck();
    await settings.getByTestId("credential-save").click();
    await expect(settings.getByTestId("credential-row").filter({ hasText: "Shared login" })).toContainText("SSH");

    // laptop's own key is refused for this user, so only the password can work.
    await expand(page, "laptop");
    const own = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("tunnel-settings").click());
    await own.getByTestId("tunnel-credentials").click();
    await own.getByTestId("login-method-login").click();
    await choose(own, "ssh-login", "Shared login");
    await expect(own.getByTestId("ssh-login-user")).toHaveText("password-user");
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();
    await connect(page, "laptop");
    await own.reload();
    await expect(own.getByTestId("tool-window")).toBeVisible();
    await own.getByTestId("tunnel-credentials").click();
    await expect(own.getByTestId("ssh-login")).toHaveAttribute("data-value", /.+/);
    await expect(own.getByTestId("ssh-host-key")).toContainText("SHA256:");
    await own.close();

    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const prodSettings = await toolWindow(context, () => tunnel(page, "prod-web").getByTestId("tunnel-settings").click());
    await prodSettings.getByTestId("tunnel-credentials").click();
    await prodSettings.getByTestId("login-method-login").click();
    await choose(prodSettings, "ssh-login", "Shared login");
    // A forward with its own login would connect instead of asking.
    await choose(prodSettings, "rdp-login", "Ask when connecting", 2);
    await prodSettings.getByTestId("tunnel-save").click();
    await expect(prodSettings.getByText("Saved", { exact: true })).toBeVisible();

    const rdp = await toolWindow(context, () => tunnel(page, "prod-web").getByTestId("forward").nth(2).getByRole("button", { name: "Attach" }).click());
    const form = rdp.locator(".rdp-form");
    const saved = form.getByTestId("rdp-saved-login");
    // The tunnel's SSH login is preselected, even though it is not for RDP yet.
    await expect(saved).toHaveText("Shared login (SSH)");
    await expect(form.getByTestId("rdp-chosen-login")).toHaveText("password-user");
    await expect(form.locator('input[type="password"]')).toHaveCount(0);
    await expect(form.locator('label:has([data-testid="rdp-save-login"])')).toHaveText(/use this login here next time/);

    await choose(rdp, "rdp-saved-login", "type a login");
    await expect(form.locator('input[type="password"]')).toHaveCount(1);
    await expect(form.locator('label:has([data-testid="rdp-save-login"])')).toHaveText(/save in credentials/);

    // The dropdown comes first, then the fields, then the checkbox.
    const order = await form.locator('[data-testid="rdp-saved-login"], input[type="password"], [data-testid="rdp-save-login"]').evaluateAll((items) => items.map((item) => (item as HTMLElement).dataset.testid ?? "password"));
    expect(order).toEqual(["rdp-saved-login", "password", "rdp-save-login"]);
    await rdp.close();

    await prodSettings.getByTestId("login-method-key").click();
    await prodSettings.getByTestId("tunnel-save").click();
    await expect(prodSettings.getByText("Saved", { exact: true }).first()).toBeVisible();
    await prodSettings.close();
    const laptop = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("tunnel-settings").click());
    await laptop.getByTestId("tunnel-credentials").click();
    await laptop.getByTestId("forget-host-key").click();
    await expect(laptop.getByTestId("ssh-host-key")).toHaveCount(0);
    await laptop.getByTestId("login-method-key").click();
    await laptop.getByTestId("tunnel-save").click();
    await expect(laptop.getByText("Saved", { exact: true })).toBeVisible();
    await laptop.close();
    await disconnect(page, "laptop");
  });
});

test.describe("maintenance", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("the log can be read and filtered, and Portless appears only on macOS", async ({ page, context }) => {
    await openMain(page);
    const settings = await openSettings(page, context);
    await settings.getByTestId("settings-maintenance").click();

    const lines = settings.getByTestId("log-line");
    await expect(lines.first()).toBeVisible();
    await expect(settings.getByTestId("log-entries")).not.toContainText("AssetFileServerFS");
    const all = await lines.count();

    await settings.getByTestId("log-search").fill("no-such-text-in-any-log");
    await expect(lines).toHaveCount(0);
    await expect(settings.getByTestId("log-empty")).toContainText("Nothing matches");
    await settings.getByTestId("log-search").fill("");
    await expect(lines).toHaveCount(all);

    await settings.getByTestId("log-level-error").click();
    for (const line of await lines.all()) await expect(line).toHaveAttribute("data-level", "ERROR");
    await settings.getByTestId("log-level-all").click();

    await expect(settings.getByTestId("diagnostics-version")).not.toBeEmpty();
    await settings.getByTestId("reset-windows").click();
    await expect(settings.getByText("Tool windows open in their default place from now on")).toBeVisible();

    const portless = settings.getByTestId("maintenance-portless");
    if (process.platform === "darwin") {
      await expect(portless.getByTestId("portless-status")).not.toBeEmpty();
      // Only looked at: clicking these would ask for admin rights on this machine.
      await expect(portless.getByTestId("portless-install")).toBeVisible();
      await expect(portless.getByTestId("portless-remove")).toBeVisible();
      await expect(portless.getByTestId("portless-refresh")).toBeVisible();
    } else {
      await expect(portless).toHaveCount(0);
    }
  });
});

test.describe("tunnel settings", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("one Save keeps every page, and Delete sits in the same bar", async ({ page, context }) => {
    await openMain(page);
    await expand(page, "stage-a");
    const own = await toolWindow(context, () => tunnel(page, "stage-a").getByTestId("tunnel-settings").click());
    await expect(own.getByTestId("delete-tunnel")).toBeVisible();
    await own.getByTestId("tunnel-credentials").click();
    await expect(own.getByTestId("delete-tunnel")).toBeVisible();

    // A key file asks for a user and a path, a saved login does not.
    await expect(own.getByTestId("tunnel-user")).toHaveValue("demo");
    await own.getByTestId("login-method-login").click();
    await expect(own.getByTestId("tunnel-user")).toHaveCount(0);
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Choose a saved login, or use a key file.")).toBeVisible();
    await own.getByTestId("login-method-key").click();

    // The filter shows one protocol at a time.
    await own.getByTestId("credentials-rdp").click();
    await expect(own.getByTestId("credentials-ssh-section")).toHaveCount(0);
    await own.getByTestId("credentials-ssh").click();
    await expect(own.getByTestId("credentials-rdp-section")).toHaveCount(0);
    await own.getByTestId("credentials-all").click();

    // Turning remote desktop on shows its port and which credentials to use.
    const rdpSwitch = own.getByTestId("rdp-enabled");
    if ((await rdpSwitch.getAttribute("aria-checked")) === "true") await rdpSwitch.click();
    await expect(own.getByTestId("tunnel-rdp-port")).toHaveCount(0);
    await rdpSwitch.click();
    await expect(own.getByTestId("tunnel-rdp-port")).toBeVisible();
    await expect(own.getByTestId("rdp-login").first()).toHaveText("Ask when connecting");
    await own.getByTestId("tunnel-monitoring").click();
    await choose(own, "tunnel-refresh-disk", "30 s");
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();

    await own.reload();
    await expect(own.getByTestId("tool-window")).toBeVisible();
    await own.getByTestId("tunnel-monitoring").click();
    await expect(own.getByTestId("tunnel-refresh-disk")).toHaveAttribute("data-value", "30");
    await own.getByTestId("tunnel-credentials").click();
    await expect(own.getByTestId("rdp-enabled")).toHaveAttribute("aria-checked", "true");

    await own.getByTestId("tunnel-monitoring").click();
    await choose(own, "tunnel-refresh-disk", /^Default/);
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();
  });

  test("a portless suffix ends every forward's domain, and an empty one changes nothing", async ({ page, context }) => {
    await openMain(page);
    await expand(page, "prod-db");
    const own = await toolWindow(context, () => tunnel(page, "prod-web").getByTestId("tunnel-settings").click());

    await own.getByTestId("tunnel-forwards").click();
    await own.locator(".portless-toggle").first().click();
    const domain = own.getByTestId("forward-domain").first();
    const suffix = own.getByTestId("domain-suffix").first();
    await expect(suffix).toHaveText(".ssh-local");
    await domain.fill("moj-adres");

    await own.getByTestId("tunnel-connection").click();
    await own.getByTestId("portless-suffix").fill(".Dev.");
    await own.getByTestId("tunnel-forwards").click();
    await expect(domain).toHaveValue("moj-adres");
    await expect(suffix).toHaveText(".dev.ssh-local");

    await own.getByTestId("tunnel-connection").click();
    await own.getByTestId("portless-suffix").fill("");
    await own.getByTestId("tunnel-forwards").click();
    await expect(domain).toHaveValue("moj-adres");
    await expect(suffix).toHaveText(".ssh-local");
    // Closed without saving: saving a Portless forward would ask for admin rights.
    await own.close();

    // Saved on a tunnel without Portless forwards, and an invalid one is refused.
    const other = await toolWindow(context, () => tunnel(page, "prod-db").getByTestId("tunnel-settings").click());
    await other.getByTestId("portless-suffix").fill("bad suffix");
    await other.getByTestId("tunnel-save").click();
    await expect(other.getByText(/invalid portless suffix/).first()).toBeVisible();
    await other.getByTestId("portless-suffix").fill("dev");
    await other.getByTestId("tunnel-save").click();
    await expect(other.getByText("Saved", { exact: true })).toBeVisible();
    await other.reload();
    await expect(other.getByTestId("portless-suffix")).toHaveValue("dev");
    await other.getByTestId("portless-suffix").fill("");
    await other.getByTestId("tunnel-save").click();
    await expect(other.getByText("Saved", { exact: true })).toBeVisible();
  });

  test("what a tunnel's settings save shows in the main window at once", async ({ page, context }) => {
    await openMain(page);
    const row = tunnel(page, "laptop").locator(".row");
    await expect(row).not.toHaveClass(/tinted/);
    const own = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("tunnel-settings").click());
    await own.locator('.color-swatch[title="red"]').click();
    await own.getByTestId("group-input").fill("Moved");
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();

    await expect(row).toHaveClass(/tinted/);
    await expect(row).toHaveAttribute("style", /--tunnel-color: #ef4444/);
    await expect(page.locator('[data-testid="group"][data-group="Moved"]')).toContainText("laptop");

    await own.locator('.color-swatch[title="no color"]').click();
    await own.getByTestId("group-input").fill("");
    await own.getByTestId("tunnel-save").click();
    await expect(page.locator('[data-testid="group"][data-group="Unassigned"]')).toContainText("laptop");
    await expect(row).not.toHaveClass(/tinted/);
  });

  test("a notification stays centered and clear of the settings buttons", async ({ page, context }) => {
    await openMain(page);
    const own = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("tunnel-settings").click());
    // The window opens at 820 wide and can shrink to 420.
    for (const width of [820, 420]) {
      await own.setViewportSize({ width, height: 600 });
      await own.getByTestId("tunnel-save").click();
      const note = own.getByText("Saved", { exact: true });
      await expect(note).toBeVisible();
      const box = (await note.boundingBox())!;
      expect(Math.abs(box.x + box.width / 2 - width / 2)).toBeLessThan(1);
      for (const id of ["tunnel-save", "delete-tunnel"]) {
        const button = (await own.getByTestId(id).boundingBox())!;
        expect(box.y + box.height, `${id} at ${width}`).toBeLessThanOrEqual(button.y);
      }
      await expect(note).toBeHidden();
    }
  });

  test("Save without changes does not reconnect the tunnel", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "stage-a");
    await expand(page, "stage-a");
    const own = await toolWindow(context, () => tunnel(page, "stage-a").getByTestId("tunnel-settings").click());
    await expect(own.getByTestId("tunnel-form")).toBeVisible();
    await own.getByTestId("tunnel-save").click();
    await expect(own.getByText("Saved", { exact: true })).toBeVisible();
    await page.waitForTimeout(500);
    await expect(tunnel(page, "stage-a")).toHaveAttribute("data-status", "connected");
    await disconnect(page, "stage-a");
  });
});

test.describe("command palette", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("finds and runs an action from the keyboard", async ({ page, context }) => {
    await openMain(page);
    await page.keyboard.press("ControlOrMeta+k");
    const input = page.getByTestId("palette-input");
    await expect(input).toBeFocused();
    await input.fill("terminal laptop");
    await expect(page.getByTestId("palette").getByRole("option").first()).toContainText("Terminal on laptop");
    const tool = await toolWindow(context, () => page.keyboard.press("Enter"));
    await expect(tool.getByTestId("terminal").locator(".xterm-rows")).toContainText("fixture$");
    await expect(page.getByTestId("palette")).toHaveCount(0);
  });

  test("Escape closes it", async ({ page }) => {
    await openMain(page);
    await page.getByTestId("open-palette").click();
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("palette")).toHaveCount(0);
  });
});
