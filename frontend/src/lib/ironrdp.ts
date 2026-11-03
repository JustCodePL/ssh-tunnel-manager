// Loads the IronRDP web client on first use. It weighs about 5.5 MB because it
// embeds the WebAssembly module, so it stays out of the startup bundle.

type RemoteDesktopModule = typeof import("../../vendor/ironrdp/iron-remote-desktop-rdp").Backend;

let loading: Promise<RemoteDesktopModule> | null = null;

export function loadIronRdp(): Promise<RemoteDesktopModule> {
  if (!loading) {
    loading = (async () => {
      const [, rdp] = await Promise.all([
        import("../../vendor/ironrdp/iron-remote-desktop.js"),
        import("../../vendor/ironrdp/iron-remote-desktop-rdp.js"),
      ]);
      await rdp.init("WARN");
      return rdp.Backend;
    })().catch((e) => {
      loading = null;
      throw e;
    });
  }
  return loading;
}
