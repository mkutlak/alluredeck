// Runtime environment – overwritten by docker-entrypoint.sh via envsubst.
// Values here are defaults used when the file is NOT processed (e.g. `npm run dev`).
window.__env__ = {
  VITE_API_URL: '$VITE_API_URL',
  VITE_APP_TITLE: '$VITE_APP_TITLE',
  VITE_APP_VERSION: '$VITE_APP_VERSION',
}
