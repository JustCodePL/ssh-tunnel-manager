import { expect, test } from "@playwright/test";
import { choose, closeTools, connect, expand, openMain, toolWindow, tunnel } from "./helpers";

// The fixture's VNC server behind prod-web asks for this VNC password.
const PASSWORD = "fixture-password";

test.describe("screen sharing", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("a VNC forward asks for a login, shows the screen and remembers the login", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const forward = tunnel(page, "prod-web").getByTestId("forward").nth(4);
    await expect(forward).toContainText("VNC");
    await expect(forward).toContainText("to 10.0.0.4:5900");

    let tool = await toolWindow(context, () => forward.getByRole("button", { name: "Attach" }).click());
    const vnc = tool.getByTestId("vnc");
    const form = vnc.locator(".vnc-form");
    // A retry finds the login saved by the first attempt and connects at once.
    await expect(form.or(vnc.locator(".vnc-title", { hasText: "fixture-mac" }))).toBeVisible();
    if (!(await form.isVisible())) await vnc.getByRole("button", { name: "change login" }).click();
    await expect(form).toBeVisible();
    if (await form.getByTestId("vnc-saved-login").count()) await choose(tool, "vnc-saved-login", "type a login");

    await form.getByTestId("vnc-username").fill("");
    await form.getByTestId("vnc-password").fill("wrong");
    await form.getByRole("button", { name: "connect" }).click();
    await expect(form.getByTestId("vnc-message")).toContainText("Authentication failed");

    await form.getByTestId("vnc-password").fill(PASSWORD);
    await form.getByRole("button", { name: "connect" }).click();
    await expect(vnc.locator(".vnc-title")).toContainText("fixture-mac");
    const canvas = vnc.getByTestId("vnc-screen").locator("canvas");
    await expect(canvas).toBeVisible();
    await expect(canvas).toHaveJSProperty("width", 64);
    // The screen is drawn in the fixture's colour.
    await expect
      .poll(() => canvas.evaluate((element: HTMLCanvasElement) => Array.from(element.getContext("2d")!.getImageData(10, 10, 1, 1).data)))
      .toEqual([48, 144, 208, 255]);
    await tool.close();

    // The saved login connects at once next time.
    tool = await toolWindow(context, () => forward.getByRole("button", { name: "Attach" }).click());
    await expect(tool.getByTestId("vnc").locator(".vnc-title")).toContainText("fixture-mac");
    await expect(tool.getByTestId("vnc").locator(".vnc-form")).toHaveCount(0);
  });

  test("the host's own screen sharing is offered only when found", async ({ page }) => {
    await openMain(page);
    await connect(page, "prod-db");
    const attach = tunnel(page, "prod-db").getByTestId("attach");
    await expect(attach).toHaveAttribute("aria-haspopup", "menu", { timeout: 15_000 });
    await attach.click();
    const menu = tunnel(page, "prod-db").getByRole("menu");
    await expect(menu.getByRole("menuitem")).not.toContainText([/Screen sharing/]);
  });
});
