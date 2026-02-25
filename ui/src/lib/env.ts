// Reads runtime env vars injected by docker-entrypoint.sh (window.__env__),
// falling back to Vite build-time values, then hardcoded defaults.
export const env = {
  apiUrl:
    window.__env__?.VITE_API_URL && !window.__env__.VITE_API_URL.startsWith('$')
      ? window.__env__.VITE_API_URL
      : (import.meta.env.VITE_API_URL ?? 'http://localhost:5050'),
  appTitle:
    window.__env__?.VITE_APP_TITLE && !window.__env__.VITE_APP_TITLE.startsWith('$')
      ? window.__env__.VITE_APP_TITLE
      : (import.meta.env.VITE_APP_TITLE ?? 'Allure Dashboard'),
} as const
