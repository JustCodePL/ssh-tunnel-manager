// Lazy CodeMirror language loaders keyed by file extension / name. Each entry
// is dynamically imported so the language grammars are code-split and only
// pulled in when a file of that type is actually opened.
import type { Extension } from "@codemirror/state";

type Loader = () => Promise<Extension>;

async function legacy(mode: () => Promise<any>): Promise<Extension> {
  const { StreamLanguage } = await import("@codemirror/language");
  return StreamLanguage.define(await mode());
}

// Keyed by lowercased extension (no dot).
const byExt: Record<string, Loader> = {
  // web
  js: async () => (await import("@codemirror/lang-javascript")).javascript(),
  mjs: async () => (await import("@codemirror/lang-javascript")).javascript(),
  cjs: async () => (await import("@codemirror/lang-javascript")).javascript(),
  jsx: async () => (await import("@codemirror/lang-javascript")).javascript({ jsx: true }),
  ts: async () => (await import("@codemirror/lang-javascript")).javascript({ typescript: true }),
  tsx: async () => (await import("@codemirror/lang-javascript")).javascript({ typescript: true, jsx: true }),
  html: async () => (await import("@codemirror/lang-html")).html(),
  htm: async () => (await import("@codemirror/lang-html")).html(),
  vue: async () => (await import("@codemirror/lang-html")).html(),
  svelte: async () => (await import("@codemirror/lang-html")).html(),
  css: async () => (await import("@codemirror/lang-css")).css(),
  scss: async () => (await import("@codemirror/lang-css")).css(),
  less: async () => (await import("@codemirror/lang-css")).css(),
  php: async () => (await import("@codemirror/lang-php")).php(),
  json: async () => (await import("@codemirror/lang-json")).json(),
  jsonc: async () => (await import("@codemirror/lang-json")).json(),
  xml: async () => (await import("@codemirror/lang-xml")).xml(),
  svg: async () => (await import("@codemirror/lang-xml")).xml(),
  md: async () => (await import("@codemirror/lang-markdown")).markdown(),
  markdown: async () => (await import("@codemirror/lang-markdown")).markdown(),

  // devops / backend
  yaml: async () => (await import("@codemirror/lang-yaml")).yaml(),
  yml: async () => (await import("@codemirror/lang-yaml")).yaml(),
  py: async () => (await import("@codemirror/lang-python")).python(),
  go: async () => (await import("@codemirror/lang-go")).go(),
  sql: async () => (await import("@codemirror/lang-sql")).sql(),
  rs: async () => (await import("@codemirror/lang-rust")).rust(),

  // legacy stream modes
  sh: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/shell")).shell),
  bash: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/shell")).shell),
  zsh: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/shell")).shell),
  env: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/shell")).shell),
  toml: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/toml")).toml),
  ini: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/properties")).properties),
  conf: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/properties")).properties),
  cfg: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/properties")).properties),
  properties: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/properties")).properties),
  dockerfile: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/dockerfile")).dockerFile),
  nginx: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/nginx")).nginx),
};

// Keyed by lowercased exact file name, for files identified by name rather
// than extension.
const byName: Record<string, Loader> = {
  dockerfile: byExt.dockerfile,
  "nginx.conf": byExt.nginx,
  ".bashrc": byExt.sh,
  ".zshrc": byExt.sh,
  ".profile": byExt.sh,
  ".env": byExt.env,
  makefile: () => legacy(async () => (await import("@codemirror/legacy-modes/mode/shell")).shell),
};

// languageForName resolves a CodeMirror language extension for the given file
// name, or null when no grammar matches (plain text editing).
export async function languageForName(name: string): Promise<Extension | null> {
  const lower = name.toLowerCase();
  if (byName[lower]) return byName[lower]();

  const dot = lower.lastIndexOf(".");
  if (dot >= 0) {
    const ext = lower.slice(dot + 1);
    if (byExt[ext]) return byExt[ext]();
  }
  return null;
}
