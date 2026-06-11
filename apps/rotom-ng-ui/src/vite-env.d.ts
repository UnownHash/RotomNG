/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_MOCK?: "true" | "false";
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
