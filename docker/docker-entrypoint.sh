#!/bin/sh
# Inject runtime environment variables into env.js via envsubst.
# Any variable not set defaults to an empty string.
set -eu

VITE_API_URL="${VITE_API_URL:-http://localhost:5050}"
VITE_APP_TITLE="${VITE_APP_TITLE:-AllureDeck}"
VITE_APP_VERSION="${VITE_APP_VERSION:-}"

export VITE_API_URL VITE_APP_TITLE VITE_APP_VERSION

# Replace template placeholders; write rendered output to /tmp (writable by non-root)
envsubst '${VITE_API_URL} ${VITE_APP_TITLE} ${VITE_APP_VERSION}' \
  < /usr/share/nginx/html/env.js.template \
  > /tmp/env.js

exec nginx -g 'daemon off;'
