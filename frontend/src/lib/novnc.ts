// Loads the noVNC client on first use, so it stays out of the startup bundle.

let loading: Promise<any> | null = null;

/** @returns the RFB class of the vendored noVNC core library */
export function loadNoVnc(): Promise<any> {
  if (!loading) {
    loading = import("../../vendor/novnc/core/rfb.js")
      .then((module: any) => module.default)
      .catch((e) => {
        loading = null;
        throw e;
      });
  }
  return loading;
}
