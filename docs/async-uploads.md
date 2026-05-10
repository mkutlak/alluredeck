# Async tar.gz Uploads

AllureDeck supports an **async upload path** for large tar.gz result archives. Instead of blocking the HTTP connection while the server extracts and processes the archive, the client gets a `202 Accepted` response immediately and polls a job endpoint for progress.

Related documentation: [Features](features.md) · [Configuration Reference](configuration.md)

---

## Why async?

Synchronous uploads work well for small result sets (a few hundred files). For large suites — thousands of test files, screenshots, video attachments, or archives over a few hundred MB — the synchronous path can:

- Hit reverse-proxy or load-balancer timeouts
- Hold an HTTP connection open for several minutes
- Consume peak heap for the full decompressed archive

The async path eliminates these issues by streaming the raw gzip bytes straight to storage as a staging blob, returning `202` once the blob is safely written, and delegating extraction and report generation to a River background worker.

---

## How it works

### 1. Client streams the archive

Add `?async=true` to the results upload URL and set `Content-Type: application/gzip` (or `application/x-gzip` / `application/x-tar+gzip`):

```bash
curl -X POST \
  "https://alluredeck.example.com/api/v1/projects/my-project/results?async=true&ci_branch=main&ci_commit_sha=$SHA" \
  -H "Authorization: Bearer $ALLUREDECK_API_KEY" \
  -H "Content-Type: application/gzip" \
  --data-binary @allure-results.tar.gz
```

The server:
1. Peeks the first two bytes to verify the gzip magic (`0x1f 0x8b`). Malformed bodies fail fast with `400`.
2. Streams the body to `staging/{batchID}.tar.gz` in the configured storage backend (local disk or S3/MinIO).
3. Returns `202 Accepted` with a JSON body:

```json
{
  "data": {
    "job_id": "42",
    "batch_id": "01J3QRST..."
  },
  "metadata": { "message": "Results staged for project 'my-project' (async)" }
}
```

> `async=true` is only honoured for `application/gzip` content types. JSON and multipart form-data uploads always use the synchronous path regardless of the query parameter.

### 2. River worker processes the staged blob

A `ParseStagedTarGzWorker` River job picks up the staged blob and executes these phases in order:

| Phase | Description |
|-------|-------------|
| `extracting_staged` | Opens the staging blob from storage; extracts the tar.gz to a pod-local temp directory |
| `preparing_local` | Prepares the local results directory for the Allure report runner |
| `generating_report` | Runs the Allure CLI against the extracted results |
| `publishing_report` | Uploads the generated report back to storage |
| `finalizing` | Persists build metadata, updates the database |
| `completed` | Job finished successfully; staging blob deleted |
| `failed` | Extraction or generation failed; staging blob retained for inspection |

The staging blob is the durability checkpoint: if the worker pod crashes mid-extraction, River retries from the blob. Once the job reaches `completed` the blob is deleted automatically.

Orphaned staging blobs (from jobs that permanently fail) are cleaned up by a periodic `StagingCleanupWorker` that runs hourly and removes blobs older than 7 days.

### 3. Client polls for progress

Poll `GET /api/v1/projects/{project_id}/jobs/{job_id}` until `status` is `completed` or `failed`:

```bash
curl -s "https://alluredeck.example.com/api/v1/projects/my-project/jobs/42" \
  -H "Authorization: Bearer $ALLUREDECK_API_KEY" | jq .
```

Example response while running:

```json
{
  "data": {
    "job_id": "42",
    "project_id": 7,
    "slug": "my-project",
    "status": "running",
    "phase": "generating_report",
    "progress": { "done": 0, "total": 0 },
    "created_at": "2026-05-10T21:00:00Z",
    "started_at": "2026-05-10T21:00:02Z"
  }
}
```

Example response on completion:

```json
{
  "data": {
    "job_id": "42",
    "status": "completed",
    "phase": "completed",
    "report_id": "15",
    "created_at": "2026-05-10T21:00:00Z",
    "started_at": "2026-05-10T21:00:02Z",
    "completed_at": "2026-05-10T21:02:34Z"
  }
}
```

**Job status values:** `pending`, `running`, `retrying`, `completed`, `failed`, `cancelled`.

**Phase values:** `pending`, `extracting_staged`, `preparing_local`, `generating_report`, `publishing_report`, `finalizing`, `completed`, `failed`.

---

## API endpoint

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/projects/{project_id}/results?async=true` | Stage a tar.gz and return `202` with `job_id` |
| `GET` | `/api/v1/projects/{project_id}/jobs/{job_id}` | Poll job status, phase, and progress |

### Query parameters for the upload

| Parameter | Description |
|-----------|-------------|
| `async` | Set to `true` to use the async path. Only honoured for gzip content types |
| `ci_branch` | CI branch name (appears in the report history table) |
| `ci_commit_sha` | Git commit SHA |
| `ci_pipeline_id` | CI pipeline ID (used for Pipeline Runs grouping on parent projects) |
| `ci_pipeline_url` | CI pipeline URL |
| `execution_name` | CI provider name (e.g. `GitHub Actions`) |
| `execution_from` | CI build URL |
| `force_project_creation` | Set to `true` to auto-create the project if it doesn't exist |
| `parent_id` | Parent project ID or slug (used with `force_project_creation`) |

---

## Recommended client polling pattern

```bash
JOB_ID="42"
PROJECT="my-project"
URL="https://alluredeck.example.com"
TOKEN="$ALLUREDECK_API_KEY"

while true; do
  RESP=$(curl -sf "$URL/api/v1/projects/$PROJECT/jobs/$JOB_ID" \
    -H "Authorization: Bearer $TOKEN")
  STATUS=$(echo "$RESP" | jq -r '.data.status')
  PHASE=$(echo "$RESP"  | jq -r '.data.phase // empty')

  echo "status=$STATUS phase=$PHASE"

  if [[ "$STATUS" == "completed" ]]; then
    REPORT_ID=$(echo "$RESP" | jq -r '.data.report_id')
    echo "Report $REPORT_ID ready."
    break
  elif [[ "$STATUS" == "failed" ]]; then
    echo "Job failed: $(echo "$RESP" | jq -r '.data.error')"
    exit 1
  fi
  sleep 5
done
```

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `UPLOAD_WRITE_CONCURRENCY` | `32` | Bounded concurrency for parallel storage writes during synchronous tar.gz extraction. Also applies to the staged worker's extraction phase |
| `MAX_UPLOAD_SIZE_MB` | `100` | Maximum request body size in MB. The async path enforces this before writing the staging blob |
| `MAX_ARCHIVE_FILE_COUNT` | `5000` | Maximum number of files inside a tar.gz. Checked during extraction in the worker; archives exceeding this are rejected |
| `REPORT_GENERATION_TIMEOUT` | `5m` | Wall-clock timeout for the River worker covering extraction + report generation. Raise for very large archives (e.g. `15m` for suites with many video attachments) |

---

## Notes

- The staging blob is always written before `202` is returned. If storage is unavailable, the request fails with `500` and no job is enqueued — the client should retry the upload.
- If the worker fails after partial extraction, the temp directory is cleaned up automatically and the staging blob is retained for River retries (up to the River max-attempts limit, default 25).
- Webhook notifications fire after successful report generation, identical to the synchronous path.
- The Admin System Monitor (`/admin`) shows async jobs (`parse_staged_targz`) alongside regular `generate_report` jobs.
