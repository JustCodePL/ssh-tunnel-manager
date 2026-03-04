/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{svelte,js,ts}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        hacker: {
          bg: '#0a0a0a',
          surface: '#111111',
          surface2: '#161616',
          border: '#1e1e1e',
          accent: '#00ff88',
          accent2: '#00d4ff',
          text: '#e0e0e0',
          muted: '#555555',
          error: '#ff4444',
          warning: '#ffaa00',
        },
      },
      fontFamily: {
        mono: ['"JetBrains Mono"', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '2px',
        sm: '2px',
        md: '2px',
        lg: '2px',
        xl: '2px',
      },
    },
  },
  plugins: [],
}
