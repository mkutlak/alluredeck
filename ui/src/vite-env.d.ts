/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL: string
  readonly VITE_APP_TITLE: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

// Ambient Window extension — no import/export so declare global is not needed
interface Window {
  __env__?: {
    VITE_API_URL?: string
    VITE_APP_TITLE?: string
  }
}
