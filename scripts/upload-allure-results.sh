#!/usr/bin/env bash
# upload-allure-results.sh — Upload Allure results to a running Alluredeck instance
#
# Usage:
#   bash scripts/upload-allure-results.sh
#
# Environment variables (all optional, defaults match docker-compose-s3.yml):
#   ALLURE_API_URL       API base URL   (default: http://localhost:5050/api/v1)
#   ALLURE_PROJECT       Project ID     (default: alluredeck-ui)
#   ALLURE_USER          Username       (default: admin)
#   ALLURE_PASS          Password       (default: admin)
#   ALLURE_RESULTS_DIR   Results dir    (default: <script-dir>/../ui/allure-results)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

API_URL="${ALLURE_API_URL:-http://localhost:5050/api/v1}"
PROJECT="${ALLURE_PROJECT:-alluredeck-ui}"
ALLURE_USER="${ALLURE_USER:-admin}"
ALLURE_PASS="${ALLURE_PASS:-admin}"
RESULTS_DIR="${ALLURE_RESULTS_DIR:-${SCRIPT_DIR}/../ui/allure-results}"

# Git metadata (best-effort — silent if git unavailable or not in a repo)
REPO_DIR="${SCRIPT_DIR}/.."
COMMIT_SHA="$(git -C "${REPO_DIR}" rev-parse HEAD 2>/dev/null || true)"
CI_BRANCH="$(git -C "${REPO_DIR}" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"

# Temp files — cleaned up on EXIT
COOKIE_JAR="$(mktemp)"
ARCHIVE="$(mktemp --suffix=.tar.gz)"
trap 'rm -f "$COOKIE_JAR" "$ARCHIVE"' EXIT

echo "==> Logging in to ${API_URL} as '${ALLURE_USER}'..."
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -c "$COOKIE_JAR" \
  -X POST "${API_URL}/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ALLURE_USER}\",\"password\":\"${ALLURE_PASS}\"}")

if [ "$HTTP_STATUS" != "200" ]; then
  echo "Error: Login failed (HTTP ${HTTP_STATUS}). Check credentials and API URL."
  exit 1
fi

# Extract CSRF token from the Netscape cookie jar (last field of the matching line)
CSRF="$(awk '/[[:space:]]csrf_token[[:space:]]/{print $NF}' "$COOKIE_JAR")"
if [ -z "$CSRF" ]; then
  echo "Error: No CSRF token in response. Is SECURITY_ENABLED=1 on the server?"
  exit 1
fi

echo "    Login OK"

# Validate results directory
if [ ! -d "$RESULTS_DIR" ]; then
  echo "Error: Results directory not found: ${RESULTS_DIR}"
  echo "       Run 'make ui-test-allure' first to generate results."
  exit 1
fi

# Normalise path now that we know the directory exists
RESULTS_DIR="$(cd "${RESULTS_DIR}" && pwd)"

FILE_COUNT=$(find "$RESULTS_DIR" -maxdepth 1 -name '*-result.json' | wc -l | tr -d ' ')
if [ "$FILE_COUNT" -eq 0 ]; then
  echo "Error: No *-result.json files found in ${RESULTS_DIR}"
  echo "       Run 'make ui-test-allure' first."
  exit 1
fi

echo "==> Packaging ${FILE_COUNT} result files from ${RESULTS_DIR}..."
tar czf "$ARCHIVE" -C "$RESULTS_DIR" .

echo "==> Uploading to project '${PROJECT}' (report generation will start automatically)..."
UPLOAD_PARAMS="force_project_creation=true"
[ -n "${COMMIT_SHA}" ] && UPLOAD_PARAMS="${UPLOAD_PARAMS}&ci_commit_sha=${COMMIT_SHA}"
[ -n "${CI_BRANCH}" ]  && UPLOAD_PARAMS="${UPLOAD_PARAMS}&ci_branch=${CI_BRANCH}"

UPLOAD_RESP=$(curl -s -w "\n%{http_code}" \
  -b "$COOKIE_JAR" \
  -X POST "${API_URL}/projects/${PROJECT}/results?${UPLOAD_PARAMS}" \
  -H "Content-Type: application/gzip" \
  -H "X-CSRF-Token: ${CSRF}" \
  --data-binary @"$ARCHIVE")

HTTP_STATUS=$(echo "$UPLOAD_RESP" | tail -1)
if [ "$HTTP_STATUS" != "200" ]; then
  echo "Error: Upload failed (HTTP ${HTTP_STATUS})."
  echo "$UPLOAD_RESP" | head -n -1
  exit 1
fi

JOB_ID=$(echo "$UPLOAD_RESP" | head -n -1 | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)
echo "    Upload OK — report generation queued (job_id: ${JOB_ID:-unknown})"
[ -n "${COMMIT_SHA}" ] && echo "    commit: ${COMMIT_SHA}"
[ -n "${CI_BRANCH}" ]  && echo "    branch: ${CI_BRANCH}"
echo ""
echo "View report at: http://localhost:7474"
