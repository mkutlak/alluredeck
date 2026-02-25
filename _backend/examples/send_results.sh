#!/bin/bash
# Send Allure test results to the Allure Docker Service.
#
# Usage:
#   ./send_results.sh [--generate-report]
#
# Environment variables:
#   ALLURE_SERVER        URL of the Allure service (default: http://localhost:5050)
#   ALLURE_PROJECT_ID    Project ID (default: default)
#   ALLURE_RESULTS_DIR   Directory containing allure-results (default: allure-results-example)
#   ALLURE_USERNAME      Username for authentication (optional)
#   ALLURE_PASSWORD      Password for authentication (optional)
#   EXECUTION_NAME       Label for this run shown in the report (optional)
#   EXECUTION_FROM       URL linking back to the CI build (optional)
#   EXECUTION_TYPE       CI system type, e.g. jenkins, github, gitlab (optional)

set -euo pipefail

ALLURE_SERVER="${ALLURE_SERVER:-http://localhost:5050}"
ALLURE_PROJECT_ID="${ALLURE_PROJECT_ID:-default}"
ALLURE_RESULTS_DIR="${ALLURE_RESULTS_DIR:-allure-results-example}"
ALLURE_USERNAME="${ALLURE_USERNAME:-}"
ALLURE_PASSWORD="${ALLURE_PASSWORD:-}"
EXECUTION_NAME="${EXECUTION_NAME:-}"
EXECUTION_FROM="${EXECUTION_FROM:-}"
EXECUTION_TYPE="${EXECUTION_TYPE:-}"

GENERATE_REPORT=false
for arg in "$@"; do
  [[ "$arg" == "--generate-report" ]] && GENERATE_REPORT=true
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_PATH="$SCRIPT_DIR/$ALLURE_RESULTS_DIR"

if [[ ! -d "$RESULTS_PATH" ]]; then
  echo "ERROR: Results directory not found: $RESULTS_PATH" >&2
  exit 1
fi

FILES_TO_SEND=$(find "$RESULTS_PATH" -maxdepth 1 -type f)
if [[ -z "$FILES_TO_SEND" ]]; then
  echo "ERROR: No files found in $RESULTS_PATH" >&2
  exit 1
fi

# --- Authentication (optional) ---
AUTH_HEADER=""
if [[ -n "$ALLURE_USERNAME" && -n "$ALLURE_PASSWORD" ]]; then
  echo "--- LOGIN ---"
  LOGIN_RESPONSE=$(curl -sf -X POST "$ALLURE_SERVER/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\": \"$ALLURE_USERNAME\", \"password\": \"$ALLURE_PASSWORD\"}")

  ACCESS_TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
  if [[ -z "$ACCESS_TOKEN" ]]; then
    echo "ERROR: Login failed. Check credentials." >&2
    exit 1
  fi
  AUTH_HEADER="Authorization: Bearer $ACCESS_TOKEN"
  echo "Login successful."
fi

# --- Build file arguments ---
FILES_ARGS=()
while IFS= read -r FILE; do
  FILES_ARGS+=("-F" "files[]=@$FILE")
done <<< "$FILES_TO_SEND"

# --- Send results ---
echo "--- SEND RESULTS ---"
CURL_AUTH=()
[[ -n "$AUTH_HEADER" ]] && CURL_AUTH=("-H" "$AUTH_HEADER")

curl -sf -X POST "$ALLURE_SERVER/send-results?project_id=$ALLURE_PROJECT_ID" \
  -H "Content-Type: multipart/form-data" \
  "${CURL_AUTH[@]}" \
  "${FILES_ARGS[@]}"

echo "Results sent successfully."

# --- Generate report (optional) ---
if [[ "$GENERATE_REPORT" == "true" ]]; then
  echo "--- GENERATE REPORT ---"
  QUERY="project_id=$ALLURE_PROJECT_ID"
  [[ -n "$EXECUTION_NAME" ]] && QUERY+="&execution_name=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$EXECUTION_NAME")"
  [[ -n "$EXECUTION_FROM" ]] && QUERY+="&execution_from=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$EXECUTION_FROM")"
  [[ -n "$EXECUTION_TYPE" ]] && QUERY+="&execution_type=$EXECUTION_TYPE"

  RESPONSE=$(curl -sf -X POST "$ALLURE_SERVER/generate-report?$QUERY" \
    -H "Content-Type: application/json" \
    "${CURL_AUTH[@]}")

  REPORT_URL=$(echo "$RESPONSE" | grep -o '"report_url":"[^"]*' | cut -d'"' -f4)
  echo "Report URL: $REPORT_URL"
fi
