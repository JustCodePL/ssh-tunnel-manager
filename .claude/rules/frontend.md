---
globs: "frontend/src/**/*.{svelte,ts,js}"
---

# Frontend Conventions

- TypeScript for all frontend code, strict mode
- Components in `frontend/src/components/`
- State management via Svelte stores in `frontend/src/stores/`
- Wails bindings auto-generated — import from `../wailsjs/go/main/App`
- Tailwind CSS for styling
- No direct DOM manipulation — use Svelte reactivity
- Event handlers: subscribe to Wails runtime events for tunnel status