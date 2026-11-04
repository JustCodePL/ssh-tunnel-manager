import { expect, type BrowserContext, type Page } from "@playwright/test";

const CONTROL_PORT = Number(process.env.STM_E2E_CONTROL_PORT ?? 34116);

export async function dropConnections() {
  await fetch(`http://127.0.0.1:${CONTROL_PORT}/drop`);
}

export async function openMain(page: Page) {
  await page.goto("/");
  await expect(page.getByTestId("tunnel-list")).toBeVisible();
  await expect(tunnel(page, "prod-web")).toBeVisible();
}

export function tunnel(page: Page, name: string) {
  return page.locator(`[data-testid="tunnel"][data-name="${name}"]`);
}

export function toggle(page: Page, name: string) {
  return tunnel(page, name).getByRole("switch");
}

export async function connect(page: Page, name: string) {
  const row = tunnel(page, name);
  if ((await row.getAttribute("data-status")) !== "connected") {
    await toggle(page, name).click();
  }
  await expect(row).toHaveAttribute("data-status", "connected", { timeout: 20_000 });
}

export async function disconnect(page: Page, name: string) {
  const row = tunnel(page, name);
  if ((await row.getAttribute("data-status")) === "connected") {
    await toggle(page, name).click();
  }
  await expect(row).toHaveAttribute("data-status", "disconnected");
}

export async function expand(page: Page, name: string) {
  const row = tunnel(page, name).locator(".row");
  if ((await row.getAttribute("aria-expanded")) !== "true") await row.click();
}

/** Picks an option in the app's own dropdown. */
export async function choose(page: Page, testid: string, label: string | RegExp, index = 0) {
  await page.getByTestId(testid).nth(index).click();
  await page.getByRole("option", { name: label, exact: typeof label === "string" }).click();
}

/** Runs an action that opens a tool window and returns that window. */
export async function toolWindow(context: BrowserContext, action: () => Promise<unknown>): Promise<Page> {
  const opened = context.waitForEvent("page");
  await action();
  const page = await opened;
  await page.waitForLoadState();
  await expect(page.getByTestId("tool-window")).toBeVisible();
  return page;
}

export async function closeTools(context: BrowserContext, keep: Page) {
  for (const page of context.pages()) {
    if (page === keep) continue;
    await page.evaluate(() => (window as any).stmCloseWindow?.()).catch(() => {});
    await page.close();
  }
}
