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
    const pixel = () => canvas.evaluate((element: HTMLCanvasElement) => Array.from(element.getContext("2d")!.getImageData(10, 10, 1, 1).data));
    // The fixture announces itself as a Mac, so the screen comes in 16-bit
    // colour: the fixture's colour, rounded to 5-6-5 bits.
    const color = vnc.getByTestId("vnc-color");
    await expect(color).toHaveText(/full color|16-bit color/);
    // A failed earlier run may have left full colour chosen.
    if ((await color.textContent()) === "16-bit color") await color.click();
    await expect(color).toHaveText("full color");
    await expect.poll(pixel).toEqual([49, 146, 206, 255]);
    // Full colour reconnects and shows the fixture's colour exactly.
    await color.click();
    await expect(color).toHaveText("16-bit color");
    await expect.poll(pixel).toEqual([48, 144, 208, 255]);
    await tool.close();

    // The saved login connects at once next time, and the target keeps full
    // colour.
    tool = await toolWindow(context, () => forward.getByRole("button", { name: "Attach" }).click());
    const again = tool.getByTestId("vnc");
    await expect(again.locator(".vnc-title")).toContainText("fixture-mac");
    await expect(again.locator(".vnc-form")).toHaveCount(0);
    await expect(again.getByTestId("vnc-color")).toHaveText("16-bit color");
    // Back to 16-bit colour, which later runs expect.
    await again.getByTestId("vnc-color").click();
    await expect(again.getByTestId("vnc-color")).toHaveText("full color");
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
