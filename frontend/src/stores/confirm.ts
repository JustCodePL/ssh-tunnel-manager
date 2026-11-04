import { writable } from "svelte/store";

export interface ConfirmRequest {
  title: string;
  message: string;
  confirmLabel: string;
  danger: boolean;
  resolve: (accepted: boolean) => void;
}

export const confirmRequest = writable<ConfirmRequest | null>(null);

/**
 * askConfirm shows an in-app dialog. Webviews do not implement window.confirm
 * reliably: on macOS it returns false without showing anything.
 */
export function askConfirm(options: { title: string; message: string; confirmLabel?: string; danger?: boolean }): Promise<boolean> {
  return new Promise((resolve) => {
    confirmRequest.set({
      title: options.title,
      message: options.message,
      confirmLabel: options.confirmLabel ?? "Confirm",
      danger: options.danger ?? false,
      resolve,
    });
  });
}
