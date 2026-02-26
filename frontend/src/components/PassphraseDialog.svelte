<script lang="ts">
  import { SubmitPassphrase } from "../../wailsjs/go/main/App";
  import { createEventDispatcher, onMount } from "svelte";

  export let keyPath: string;

  const dispatch = createEventDispatcher<{ close: void }>();

  let passphrase = "";
  let submitting = false;
  let inputEl: HTMLInputElement | null = null;

  onMount(() => {
    // Focus the input programmatically to avoid using the autofocus attribute
    inputEl?.focus();
  });

  async function handleSubmit() {
    submitting = true;
    await SubmitPassphrase(passphrase);
    passphrase = "";
    submitting = false;
    dispatch("close");
  }

  async function handleCancel() {
    await SubmitPassphrase("");
    dispatch("close");
  }
</script>

<div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
  <div class="bg-zinc-800 border border-zinc-700 rounded-lg p-5 w-96 shadow-xl">
    <h2 class="text-base font-semibold mb-2">Passphrase Required</h2>
    <p class="text-sm text-zinc-400 mb-4">
      Enter passphrase for key:
      <span class="block font-mono text-xs text-zinc-300 mt-1 break-all">{keyPath}</span>
    </p>

    <form on:submit|preventDefault={handleSubmit}>
      <input
        type="password"
        bind:value={passphrase}
        bind:this={inputEl}
        class="w-full rounded bg-zinc-900 border border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-100 focus:border-blue-500 focus:outline-none mb-4"
        placeholder="Passphrase"
      />
      <div class="flex justify-end gap-2">
        <button
          type="button"
          class="px-3 py-1.5 text-sm rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-300"
          on:click={handleCancel}
        >
          Cancel
        </button>
        <button
          type="submit"
          class="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50"
          disabled={!passphrase || submitting}
        >
          Unlock
        </button>
      </div>
    </form>
  </div>
</div>
