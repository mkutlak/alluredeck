#!/bin/sh
# Inject runtime environment variables into env.js via envsubst.
# Any variable not set defaults to an empty string.
set -eu

VITE_API_URL="${VITE_API_URL:-http://localhost:5050}"
VITE_APP_TITLE="${VITE_APP_TITLE:-AllureDeck}"

export VITE_API_URL VITE_APP_TITLE

# Replace template placeholders; write to the final env.js
envsubst '${VITE_API_URL} ${VITE_APP_TITLE}' \
  < /usr/share/nginx/html/env.js \
  > /tmp/env.js.tmp && mv /tmp/env.js.tmp /usr/share/nginx/html/env.js

exec nginx -g 'daemon off;'
