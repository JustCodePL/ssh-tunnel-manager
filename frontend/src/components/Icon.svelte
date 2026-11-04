<script lang="ts">
  import { icons } from "../lib/icons";

  export let name: string;
  export let size = 15;
</script>

<svg
  viewBox="0 0 24 24"
  width={size}
  height={size}
  fill="none"
  stroke="currentColor"
  stroke-width="1.7"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>{@html icons[name] ?? ""}</svg>
