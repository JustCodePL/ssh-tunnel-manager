// Loads the IronRDP web client on first use. It weighs about 5.5 MB because it
// embeds the WebAssembly module, so it stays out of the startup bundle.

export interface IronRdpModule {
  backend: typeof import("../../vendor/ironrdp/iron-remote-desktop-rdp").Backend;
  exports: typeof import("../../vendor/ironrdp/iron-remote-desktop-rdp");
}

let loading: Promise<IronRdpModule> | null = null;

export function loadIronRdp(): Promise<IronRdpModule> {
  if (!loading) {
    loading = (async () => {
      const [, rdp] = await Promise.all([
        import("../../vendor/ironrdp/iron-remote-desktop.js"),
        import("../../vendor/ironrdp/iron-remote-desktop-rdp.js"),
      ]);
      await rdp.init("WARN");
      return { backend: rdp.Backend, exports: rdp };
    })().catch((e) => {
      loading = null;
      throw e;
    });
  }
  return loading;
}
