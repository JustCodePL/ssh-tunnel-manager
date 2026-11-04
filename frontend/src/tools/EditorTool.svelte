<script lang="ts">
  import { errorText } from "../lib/runtime";
  import { createEventDispatcher, onDestroy, onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { OpenSFTP, OpenForwardSFTP, CloseSFTP, BindTabSession } from "../../bindings/ssh-tunnel-manager/app";
  import TextEditor from "../components/TextEditor.svelte";
  import type { PortForward, TunnelConfig } from "../types";

  export let tunnel: TunnelConfig;
  export let forward: PortForward | null = null;
  export let tabId: string;
  export let path: string;

  const dispatch = createEventDispatcher<{ close: void }>();
  let sessionId = "";
  let error = "";

  onMount(async () => {
    try {
      const result = forward
        ? await OpenForwardSFTP(tunnel.id, { remoteHost: forward.remoteHost, remotePort: forward.remotePort } as any)
        : await OpenSFTP(tunnel.id);
      sessionId = result?.sessionId ?? "";
      if (sessionId) void BindTabSession(tabId, "sftp", sessionId);
    } catch (e: any) {
      error = errorText(e);
    }
  });

  onDestroy(() => {
    if (sessionId) void CloseSFTP(sessionId).catch(() => {});
  });

  function saved() {
    void Events.Emit("files:changed");
  }
</script>

{#if error}
  <p class="message">{error}</p>
{:else if sessionId}
  <TextEditor {sessionId} {path} name={path.split("/").pop() ?? path} on:saved={saved} on:close={() => dispatch("close")} />
{:else}
  <p class="message">Opening {path}</p>
{/if}

<style>
  .message {
    padding: 24px;
    text-align: center;
    color: var(--muted);
  }
</style>
