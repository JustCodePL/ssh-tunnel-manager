import { OpenTool } from "../../bindings/ssh-tunnel-manager/app";
import { showToast } from "../stores/toast";

export interface ToolSpec {
  tool: string;
  tunnelId: string;
  remoteHost?: string;
  remotePort?: number;
  path?: string;
}

/** openTool opens a tool in its window. Builds without native windows use a tab. */
export async function openTool(spec: ToolSpec): Promise<void> {
  try {
    const result = await OpenTool(spec as any);
    if (result.external) window.open(result.url, result.window);
  } catch (e: any) {
    showToast(String(e?.message ?? e), "error");
  }
}
