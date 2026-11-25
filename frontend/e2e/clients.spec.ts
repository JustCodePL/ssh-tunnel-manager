import { expect, test } from "@playwright/test";
import { choose, client, closeTools, directServers, openClients, openMain, toolWindow } from "./helpers";

// The fixture's VNC servers ask for this VNC password.
const PASSWORD = "fixture-password";

async function addDesktop(page: import("@playwright/test").Page, desktop: { name: string; protocol: "rdp" | "vnc"; port: number }) {
  await page.getByTestId("add-desktop").click();
  const form = page.getByTestId("desktop-form");
  await form.getByTestId("desktop-name").fill(desktop.name);
  await form.getByTestId(`desktop-protocol-${desktop.protocol}`).click();
  await form.getByTestId("desktop-address").fill("127.0.0.1");
  await form.getByTestId("desktop-port").fill(String(desktop.port));
  return form;
}

test.describe("main window tabs", () => {
  test("switches views and opens on the last one", async ({ page }) => {
    await openMain(page);
    await page.getByTestId("main-tab-clients").click();
    await expect(page.getByTestId("client-list")).toBeVisible();
    // Sessions on the hosts are listed as clients through SSH.
    await expect(client(page, "prod-web").first()).toContainText("via prod-web");
    await expect(page.locator('[data-testid="client"][data-kind="forward"]').first()).toBeVisible();

    await page.reload();
    await expect(page.getByTestId("main-tab-clients")).toHaveAttribute("aria-selected", "true");
    await expect(page.getByTestId("client-list")).toBeVisible();

    await page.getByTestId("main-tab-web").click();
    // The scenario has no portless forward.
    await expect(page.getByTestId("web-list")).toContainText("No web pages yet");
    await page.getByTestId("main-tab-tunnels").click();
    await expect(page.getByTestId("tunnel-list")).toBeVisible();
  });
});

test.describe("remote desktops without SSH", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("a VNC desktop connects directly and answers the check", async ({ page, context }) => {
    const direct = await directServers();
    await openClients(page, "direct-mac");
    const form = await addDesktop(page, { name: "direct-mac", protocol: "vnc", port: direct.vnc });
    await expect(form.getByTestId("desktop-vnc-warning")).toContainText("VNC does not encrypt the connection");
    await form.getByTestId("desktop-save").click();
    await expect(form).toHaveCount(0);

    const row = client(page, "direct-mac");
    await expect(row).toContainText("direct");
    await expect(row).toHaveAttribute("data-state", "reach-up");

    const tool = await toolWindow(context, () => row.click());
    const vnc = tool.getByTestId("vnc");
    const login = vnc.locator(".vnc-form");
    await expect(login).toBeVisible();
    if (await login.getByTestId("vnc-saved-login").count()) await choose(tool, "vnc-saved-login", "type a login");
    await login.getByTestId("vnc-username").fill("");
    await login.getByTestId("vnc-password").fill(PASSWORD);
    await login.getByTestId("vnc-save-login").uncheck();
    await login.getByRole("button", { name: "connect" }).click();
    await expect(vnc.locator(".vnc-title")).toContainText("direct-mac (fixture-mac)");
    await expect(vnc.getByTestId("vnc-screen").locator("canvas")).toBeVisible();
    await tool.close();

    await openClients(page, "direct-mac");
  });

  test("the first RDP certificate waits for the user, and the check finds a stopped server", async ({ page, context }) => {
    const direct = await directServers();
    await openClients(page, "direct-pc", "direct-off");
    const form = await addDesktop(page, { name: "direct-pc", protocol: "rdp", port: direct.rdp });
    await expect(form.getByTestId("desktop-vnc-warning")).toHaveCount(0);
    await form.getByTestId("desktop-save").click();

    const tool = await toolWindow(context, () => client(page, "direct-pc").click());
    const rdp = tool.getByTestId("rdp");
    const login = rdp.locator(".rdp-form");
    await expect(login).toBeVisible();
    if (await login.getByTestId("rdp-saved-login").count()) await choose(tool, "rdp-saved-login", "type a login");
    await login.getByLabel("user").fill("me");
    await login.getByLabel("password").fill("secret");
    await login.getByTestId("rdp-save-login").uncheck();
    await login.getByRole("button", { name: "connect" }).click();

    const first = rdp.getByTestId("rdp-first-certificate");
    await expect(first).toBeVisible({ timeout: 20_000 });
    await expect(first.getByTestId("rdp-certificate-fingerprint")).toHaveText(direct.rdpFingerprint);
    await first.getByTestId("rdp-trust-certificate").click();
    // Trusted now: the next attempt gets past the certificate. The fixture is
    // not a full RDP server, so the session goes no further.
    await expect(first).toHaveCount(0);
    await expect(rdp.getByTestId("rdp-first-certificate")).toHaveCount(0, { timeout: 5_000 });
    await tool.close();

    // A desktop that stopped answering turns grey after a check.
    const closed = await addDesktop(page, { name: "direct-off", protocol: "rdp", port: 9 });
    await closed.getByTestId("desktop-save").click();
    await page.getByTestId("check-connections").click();
    await expect(client(page, "direct-off")).toHaveAttribute("data-state", "reach-down");
    await expect(client(page, "direct-pc")).toHaveAttribute("data-state", "reach-up");

    await openClients(page, "direct-pc", "direct-off");
  });
});
