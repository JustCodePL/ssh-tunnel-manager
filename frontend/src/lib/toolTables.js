/**
 * @template {{command: string, cpu: number, mem: number, pid: number}} P
 * @param {P[]} list
 * @param {"cpu" | "mem" | "name" | "pid"} sortBy
 * @returns {P[]} a sorted copy
 */
export function sortProcesses(list, sortBy) {
  const copy = [...list];
  if (sortBy === "mem") copy.sort((a, b) => b.mem - a.mem || a.command.localeCompare(b.command));
  else if (sortBy === "name") copy.sort((a, b) => a.command.localeCompare(b.command));
  else if (sortBy === "pid") copy.sort((a, b) => a.pid - b.pid);
  else copy.sort((a, b) => b.cpu - a.cpu || a.command.localeCompare(b.command));
  return copy;
}

/**
 * Bars in a table scale to the row with the highest value, because most
 * processes use a fraction of a percent and a true scale would look empty.
 * @param {number} value
 * @param {number} max
 * @returns {number} width in percent
 */
export function scaleBar(value, max) {
  if (!(max > 0)) return 0;
  return Math.min((value / max) * 100, 100);
}

/** @param {string | undefined} text like "12.5%" */
function percent(text) {
  const value = parseFloat((text ?? "").replace("%", ""));
  return Number.isNaN(value) ? 0 : value;
}

/**
 * @template {{name: string, cpuPercent?: string, memPercent?: string}} C
 * @param {C[]} list
 * @param {"name" | "cpu" | "mem"} sortBy
 * @returns {C[]} a sorted copy
 */
export function sortContainers(list, sortBy) {
  const copy = [...list];
  if (sortBy === "cpu") copy.sort((a, b) => percent(b.cpuPercent) - percent(a.cpuPercent) || a.name.localeCompare(b.name));
  else if (sortBy === "mem") copy.sort((a, b) => percent(b.memPercent) - percent(a.memPercent) || a.name.localeCompare(b.name));
  else copy.sort((a, b) => a.name.localeCompare(b.name));
  return copy;
}

/**
 * @param {string} path a remote path
 * @returns {{label: string, path: string}[]} its crumbs from the root
 */
export function breadcrumbs(path) {
  const parts = path.split("/").filter(Boolean);
  const crumbs = [{ label: "/", path: "/" }];
  let current = "";
  for (const part of parts) {
    current += `/${part}`;
    crumbs.push({ label: part, path: current });
  }
  return crumbs;
}
