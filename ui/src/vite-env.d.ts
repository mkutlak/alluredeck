/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL: string
  readonly VITE_APP_TITLE: string
  readonly VITE_APP_VERSION: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

// Ambient Window extension — no import/export so declare global is not needed
interface Window {
  __env__?: {
    VITE_API_URL?: string
    VITE_APP_TITLE?: string
    VITE_APP_VERSION?: string
  }
}
