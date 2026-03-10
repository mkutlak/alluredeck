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

echo "==> Uploading to project '${PROJECT}'..."
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -b "$COOKIE_JAR" \
  -X POST "${API_URL}/projects/${PROJECT}/results?force_project_creation=true" \
  -H "Content-Type: application/gzip" \
  -H "X-CSRF-Token: ${CSRF}" \
  --data-binary @"$ARCHIVE")

if [ "$HTTP_STATUS" != "200" ]; then
  echo "Error: Upload failed (HTTP ${HTTP_STATUS})."
  exit 1
fi

echo "    Upload OK"

echo "==> Triggering report generation..."
REPORT_PARAMS=""
[ -n "${COMMIT_SHA}" ] && REPORT_PARAMS="${REPORT_PARAMS}&ci_commit_sha=${COMMIT_SHA}"
[ -n "${CI_BRANCH}" ]  && REPORT_PARAMS="${REPORT_PARAMS}&ci_branch=${CI_BRANCH}"
# Build query string: replace leading '&' with '?', or leave empty
REPORT_QUERY="${REPORT_PARAMS:+?${REPORT_PARAMS#&}}"

REPORT_RESP=$(curl -s \
  -b "$COOKIE_JAR" \
  -X POST "${API_URL}/projects/${PROJECT}/reports${REPORT_QUERY}" \
  -H "Content-Length: 0" \
  -H "X-CSRF-Token: ${CSRF}")

JOB_ID=$(echo "$REPORT_RESP" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)
echo "    Report generation queued (job_id: ${JOB_ID:-unknown})"
[ -n "${COMMIT_SHA}" ] && echo "    commit: ${COMMIT_SHA}"
[ -n "${CI_BRANCH}" ]  && echo "    branch: ${CI_BRANCH}"
echo ""
echo "View report at: http://localhost:7474"
