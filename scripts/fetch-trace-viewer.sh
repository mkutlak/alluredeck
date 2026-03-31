#!/usr/bin/env bash
# fetch-trace-viewer.sh — Download Playwright trace viewer static assets
# and copy them to api/static/trace/ for Go embed.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET_DIR="${SCRIPT_DIR}/../api/static/trace"
TMP_DIR="$(mktemp -d)"

cleanup() {
    echo "Cleaning up temp directory: ${TMP_DIR}"
    rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

echo "Fetching Playwright trace viewer assets..."
echo "Temp dir: ${TMP_DIR}"
echo "Target dir: ${TARGET_DIR}"

# Install playwright-core in a temp directory to extract the trace viewer
cd "${TMP_DIR}"
npm init -y > /dev/null
npm install playwright-core

# Locate the trace viewer source
TRACE_SRC="${TMP_DIR}/node_modules/playwright-core/lib/vite/traceViewer"
if [ ! -d "${TRACE_SRC}" ]; then
    echo "ERROR: Trace viewer source not found at ${TRACE_SRC}" >&2
    exit 1
fi

# Prepare target directory (remove stale files, keep .gitkeep)
mkdir -p "${TARGET_DIR}"
find "${TARGET_DIR}" -mindepth 1 ! -name '.gitkeep' -delete

# Copy trace viewer assets
cp -r "${TRACE_SRC}/." "${TARGET_DIR}/"

echo "Done. Trace viewer assets written to: ${TARGET_DIR}"
