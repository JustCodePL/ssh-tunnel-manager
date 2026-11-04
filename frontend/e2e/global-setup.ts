import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, "../..");

export const APP_PORT = Number(process.env.STM_E2E_PORT ?? 34115);
export const CONTROL_PORT = Number(process.env.STM_E2E_CONTROL_PORT ?? 34116);

async function waitFor(url: string, name: string, child: ChildProcess) {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`${name} exited with code ${child.exitCode}`);
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch {
      // Not listening yet.
    }
    await new Promise((done) => setTimeout(done, 200));
  }
  throw new Error(`${name} did not start`);
}

function build(args: string[], env: NodeJS.ProcessEnv = process.env) {
  const result = spawnSync("go", args, { cwd: repo, env, stdio: "inherit" });
  if (result.status !== 0) throw new Error(`go ${args.join(" ")} failed`);
}

export default async function globalSetup() {
  const work = mkdtempSync(join(tmpdir(), "stm-e2e-"));
  const home = join(work, "home");
  const bin = (name: string) => join(work, process.platform === "win32" ? `${name}.exe` : name);

  if (!process.env.STM_E2E_SKIP_FRONTEND_BUILD) {
    const vite = spawnSync("npx", ["vite", "build"], { cwd: resolve(repo, "frontend"), stdio: "inherit", shell: process.platform === "win32" });
    if (vite.status !== 0) throw new Error("frontend build failed");
  }
  build(["build", "-o", bin("e2e-fixture"), "./cmd/e2e-fixture"]);
  build(["build", "-tags", "server", "-o", bin("stm-server"), "."]);

  const fixture = spawn(bin("e2e-fixture"), [home, String(CONTROL_PORT)], { stdio: "inherit" });
  await waitFor(`http://127.0.0.1:${CONTROL_PORT}/health`, "fixture", fixture);

  const app = spawn(bin("stm-server"), [], {
    cwd: work,
    env: { ...process.env, HOME: home, USERPROFILE: home, STM_E2E_PORT: String(APP_PORT) },
    stdio: process.env.STM_E2E_VERBOSE ? "inherit" : "ignore",
  });
  await waitFor(`http://127.0.0.1:${APP_PORT}/`, "app", app);

  writeFileSync(join(here, ".state.json"), JSON.stringify({ pids: [fixture.pid, app.pid], work, home }));
}
