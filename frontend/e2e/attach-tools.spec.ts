import { expect, test } from "@playwright/test";
import { closeTools, connect, disconnect, dropConnections, expand, openMain, toolWindow, tunnel } from "./helpers";

test.describe("attach and tool windows", () => {
  test.afterEach(async ({ context, page }) => {
    await closeTools(context, page);
  });

  test("Attach opens a terminal directly when it is the only session", async ({ page, context }) => {
    await openMain(page);
    const attach = tunnel(page, "laptop").getByTestId("attach");
    await expect(attach.locator("svg")).toHaveCount(1); // no caret
    const tool = await toolWindow(context, () => attach.click());
    const terminal = tool.getByTestId("terminal");
    await expect(terminal.locator(".xterm-rows")).toContainText("fixture$");
    await terminal.locator(".xterm-helper-textarea").pressSequentially("hello\n");
    await expect(terminal.locator(".xterm-rows")).toContainText("ran: hello");
  });

  test("Attach expands once a remote desktop is detected, and a second terminal is a tab", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "prod-db");
    const attach = tunnel(page, "prod-db").getByTestId("attach");
    await expect(attach).toHaveAttribute("aria-haspopup", "menu", { timeout: 15_000 });
    await attach.click();
    const menu = tunnel(page, "prod-db").getByRole("menu");
    await expect(menu.getByRole("menuitem")).toHaveText([/Terminal/, /Remote desktop \(RDP\)/]);

    const tool = await toolWindow(context, () => menu.getByRole("menuitem", { name: /Terminal/ }).click());
    await expect(tool.getByTestId("terminal").first().locator(".xterm-rows")).toContainText("fixture$");

    await tool.getByTestId("new-tab").click();
    await expect(tool.getByTestId("tab")).toHaveCount(2);
    await tool.getByTestId("tab").first().getByRole("button", { name: /Close/ }).click();
    await expect(tool.getByTestId("tab")).toHaveCount(1);
    await disconnect(page, "prod-db");
  });

  test("forward Attach goes straight to its target, and two servers behind one tunnel get two windows", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const forwards = tunnel(page, "prod-web").getByTestId("forward");

    const first = await toolWindow(context, () => forwards.nth(1).getByRole("button", { name: "Attach" }).click());
    await expect(first.getByTestId("rdp")).toBeVisible();
    await expect(first).toHaveTitle(/10\.0\.0\.1:3389/);

    const second = await toolWindow(context, () => forwards.nth(2).getByRole("button", { name: "Attach" }).click());
    await expect(second).toHaveTitle(/10\.0\.0\.2:3389/);
    expect(first.url()).not.toBe(second.url());

    // A forward without a type offers no Attach.
    await expect(forwards.nth(0).getByRole("button", { name: "Attach" })).toHaveCount(0);
    // An SSH forward offers Attach and Files.
    await expect(forwards.nth(3).getByRole("button", { name: "Files" })).toBeVisible();
  });

  test("htop shows a sortable process table", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const tool = await toolWindow(context, () => tunnel(page, "prod-web").getByRole("button", { name: "htop" }).click());
    const table = tool.getByTestId("process-table");
    await expect(table.locator("tbody tr").first()).toContainText("postgres");
    await table.getByRole("columnheader", { name: "Mem" }).click();
    await expect(table.locator("tbody tr").first()).toContainText("node");
    await expect(tool.getByTestId("tool")).toContainText("0.42 0.38 0.31");
  });

  test("fast typing reaches the terminal in order", async ({ page, context }) => {
    await openMain(page);
    const tool = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("attach").click());
    const terminal = tool.getByTestId("terminal");
    await expect(terminal.locator(".xterm-rows")).toContainText("fixture$");
    const text = "abcdefghijklmnopqrstuvwxyz0123456789";
    await terminal.locator(".xterm-helper-textarea").pressSequentially(`${text}\n`);
    await expect(terminal.locator(".xterm-rows")).toContainText(`ran: ${text}`);
  });

  test("the terminal's last line stays inside the window", async ({ page, context }) => {
    await openMain(page);
    const tool = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("attach").click());
    await tool.setViewportSize({ width: 860, height: 503 });
    const screen = tool.locator(".xterm-screen");
    await expect(screen).toBeVisible();
    await tool.waitForTimeout(400);
    const box = await tool.evaluate(() => {
      const term = document.querySelector(".xterm-screen")!.getBoundingClientRect();
      const host = document.querySelector(".term-host")!.getBoundingClientRect();
      return { termBottom: term.bottom, hostBottom: host.bottom, termRight: term.right, hostRight: host.right, windowBottom: window.innerHeight };
    });
    expect(box.termBottom).toBeLessThanOrEqual(box.hostBottom);
    expect(box.termRight).toBeLessThanOrEqual(box.hostRight);
    expect(box.termBottom).toBeLessThanOrEqual(box.windowBottom - 6);
  });

  test("Docker lists containers and keeps env closed until asked", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const tool = await toolWindow(context, () => tunnel(page, "prod-web").getByRole("button", { name: "Docker" }).click());
    const table = tool.getByTestId("docker-list");
    await expect(table.getByTestId("docker-card").first()).toBeVisible();
    await expect(table).toContainText("postgres:16");
    await expect(table).toContainText("Stopped 1");
    await expect(table).not.toContainText("worker");

    await table.getByText("db", { exact: true }).click();
    const detail = tool.getByTestId("docker-detail");
    await expect(detail).toContainText("postgres:16");
    await expect(detail).toContainText("Env (2)");
    await expect(detail).not.toContainText("do-not-show");
    await detail.getByTestId("docker-env").click();
    await expect(detail).toContainText("do-not-show");
  });

  test("Disk shows usage and flags a full disk", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const tool = await toolWindow(context, () => tunnel(page, "prod-web").getByRole("button", { name: "Disk" }).click());
    const table = tool.getByTestId("disk-list");
    await expect(table).toContainText("/data");
    await expect(table.getByTestId("disk-card").nth(1).locator(".meter-fill")).toHaveClass(/crit/);
  });

  test("Files lists, creates and deletes, and Edit opens an editor window", async ({ page, context }) => {
    await openMain(page);
    await expand(page, "laptop");
    const tool = await toolWindow(context, () => tunnel(page, "laptop").getByRole("button", { name: "Files" }).click());
    const table = tool.getByTestId("files-table");
    await expect(table).toContainText("README.md");
    await expect(table).toContainText("deploy.sh");

    await tool.getByRole("button", { name: "New folder" }).click();
    await tool.locator(".modal-input").fill("created-by-test");
    await tool.locator(".modal-actions .btn.primary").click();
    await expect(table).toContainText("created-by-test");

    const created = tool.locator('[data-testid="file-row"][data-name="created-by-test"]');
    await created.hover();
    await created.getByRole("button", { name: /Rename/ }).click();
    await tool.locator(".modal-input").fill("renamed-by-test");
    await tool.locator(".modal-actions .btn.primary").click();
    await expect(table).toContainText("renamed-by-test");

    const row = tool.locator('[data-testid="file-row"][data-name="renamed-by-test"]');
    await row.hover();
    await row.getByRole("button", { name: /Delete/ }).click();
    await tool.getByTestId("confirm-yes").click();
    await expect(table).not.toContainText("renamed-by-test");

    const readme = tool.locator('[data-testid="file-row"][data-name="README.md"]');
    await readme.hover();
    const editor = await toolWindow(context, () => readme.getByRole("button", { name: "Edit" }).click());
    await expect(editor.locator(".cm-content")).toContainText("# Demo");
    await expect(editor).toHaveTitle(/README\.md/);
  });

  test("Logs shows the tunnel's connection attempts", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "stage-a");
    await expand(page, "stage-a");
    const tool = await toolWindow(context, () => tunnel(page, "stage-a").getByRole("button", { name: "Logs" }).click());
    await expect(tool.getByTestId("logs")).toContainText("connect", { ignoreCase: true });
    await disconnect(page, "stage-a");
  });

  test("a tool keeps its window and resumes after the connection drops", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "prod-web");
    await expand(page, "prod-web");
    const tool = await toolWindow(context, () => tunnel(page, "prod-web").getByRole("button", { name: "Disk" }).click());
    await expect(tool.getByTestId("disk-list")).toBeVisible();

    await dropConnections();
    await expect(tool.getByTestId("overlay-reconnecting")).toBeVisible();
    await expect(tool.getByTestId("overlay-reconnecting")).toHaveCount(0, { timeout: 30_000 });
    await expect(tunnel(page, "prod-web")).toHaveAttribute("data-status", "connected");
    await expect(tool.getByTestId("disk-list")).toBeVisible();
    await expect(tool.getByTestId("overlay-lost")).toHaveCount(0);
  });

  test("a disconnected tool offers Reconnect", async ({ page, context }) => {
    await openMain(page);
    await connect(page, "stage-a");
    await expand(page, "stage-a");
    const tool = await toolWindow(context, () => tunnel(page, "stage-a").getByRole("button", { name: "Disk" }).click());
    await expect(tool.getByTestId("disk-list")).toBeVisible();

    await disconnect(page, "stage-a");
    await expect(tool.getByTestId("overlay-lost")).toBeVisible();
    await tool.getByTestId("reconnect").click();
    await expect(tool.getByTestId("disk-list")).toBeVisible({ timeout: 20_000 });
    await expect(tool.getByTestId("overlay-lost")).toHaveCount(0);
    await disconnect(page, "stage-a");
  });

  test("ending a terminal session offers Reconnect", async ({ page, context }) => {
    await openMain(page);
    const tool = await toolWindow(context, () => tunnel(page, "laptop").getByTestId("attach").click());
    const terminal = tool.getByTestId("terminal");
    await expect(terminal.locator(".xterm-rows")).toContainText("fixture$");
    await terminal.locator(".xterm-helper-textarea").pressSequentially("exit\n");
    await expect(tool.getByTestId("overlay-ended")).toBeVisible();
    await tool.getByTestId("reconnect").click();
    await expect(tool.getByTestId("overlay-ended")).toHaveCount(0);
    await expect(tool.getByTestId("terminal").locator(".xterm-rows")).toContainText("fixture$");
  });
});
