import { readFileSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));

export default async function globalTeardown() {
  const state = JSON.parse(readFileSync(join(here, ".state.json"), "utf8"));
  for (const pid of state.pids) {
    try {
      process.kill(pid, "SIGKILL");
    } catch {
      // Already gone.
    }
  }
  rmSync(state.work, { recursive: true, force: true });
  rmSync(join(here, ".state.json"), { force: true });
}
