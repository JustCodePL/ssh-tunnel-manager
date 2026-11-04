// Thin layer over the Wails 3 runtime, so components do not depend on its event
// object shape. Every subscription returns its own unsubscribe function: several
// components in one window can listen to the same event.
import { Application, Browser, Clipboard, Events, Window } from "@wailsio/runtime";

export function EventsOn(name: string, callback: (data: any) => void): () => void {
  return Events.On(name, (event: any) => callback(event.data));
}

export const ClipboardGetText = (): Promise<string> => Clipboard.Text();
export const ClipboardSetText = (text: string): Promise<void> => Clipboard.SetText(text);

export const WindowMinimise = (): Promise<void> => Window.Minimise();
export const WindowToggleMaximise = (): Promise<void> => Window.ToggleMaximise();
export const WindowClose = (): Promise<void> => Window.Close();
export const WindowName = (): Promise<string> => Window.Name();
export const Quit = (): Promise<void> => Application.Quit();

export interface EventScope {
  on(name: string, callback: (data: any) => void): void;
  off(): void;
}

/** createEventScope groups a component's subscriptions so it can drop only its own. */
export function createEventScope(): EventScope {
  const offs: Array<() => void> = [];
  return {
    on(name, callback) {
      offs.push(EventsOn(name, callback));
    },
    off() {
      offs.splice(0).forEach((off) => off());
    },
  };
}

/**
 * Resolves once events from the backend reach this window. In the HTTP build,
 * which the end-to-end tests use, they arrive over a socket that connects after
 * the page loads, and a session's first output would be lost before that. Gives
 * up after a few seconds, because a slow socket should not block a session.
 */
export function waitForEvents(ping: (token: string) => Promise<void>, timeoutMs = 3000): Promise<void> {
  const token = String(Math.random());
  return new Promise((resolve) => {
    const finish = () => {
      clearInterval(timer);
      clearTimeout(limit);
      off();
      resolve();
    };
    const off = EventsOn("events:ping", (received: string) => {
      if (received === token) finish();
    });
    const limit = setTimeout(finish, timeoutMs);
    const timer = setInterval(() => void ping(token).catch(() => {}), 100);
    void ping(token).catch(() => {});
  });
}

export const BrowserOpenURL = (url: string): Promise<void> => Browser.OpenURL(url);

/** errorText returns a bound method's error without the runtime's class prefix. */
export function errorText(error: unknown): string {
  const text = String((error as any)?.message ?? error ?? "");
  return text.replace(/^(RuntimeError|Error):\s*/, "");
}
