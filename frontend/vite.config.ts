import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  // The vendored noVNC awaits at the top level of a module. Current webviews
  // support that, and the module loads only with the VNC viewer, so an older
  // webview could fail only there.
  esbuild: {supported: {'top-level-await': true}},
  optimizeDeps: {esbuildOptions: {supported: {'top-level-await': true}}},
})
