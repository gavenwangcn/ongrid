/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Force http|https in Devices install one-liner; empty follows browser. */
  readonly VITE_EDGE_INSTALL_SCHEME?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
