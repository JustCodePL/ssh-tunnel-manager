import { writable } from "svelte/store";

export interface Toast {
  id: number;
  message: string;
  kind: "info" | "success" | "error";
}

export const toasts = writable<Toast[]>([]);

let nextId = 1;

export function showToast(message: string, kind: Toast["kind"] = "info", durationMs = 1800) {
  const id = nextId++;
  toasts.update((list) => [...list, { id, message, kind }]);
  setTimeout(() => {
    toasts.update((list) => list.filter((t) => t.id !== id));
  }, durationMs);
}
