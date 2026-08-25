-- +goose Up
-- Intentionally a no-op.
--
-- This migration originally deleted the bare "shell" test_results twins left
-- behind by double ingestion (InsertBatch stability rows vs InsertBatchFull
-- enriched rows landing on two different history_id schemes — see
-- historyIDsByFullName in api/internal/runner/allure.go for the forward fix).
--
-- The single set-based DELETE ran on startup, before the HTTP listener binds.
-- On production-sized tables it exceeded the connection's statement_timeout
-- (SQLSTATE 57014) and crash-looped the pod: migrations must stay cheap on the
-- pre-bind path. The cleanup now runs as a batched background job instead —
-- ShellTwinCleanupWorker (api/internal/runner/shelltwin_cleanup_worker.go),
-- scheduled by the River job manager on startup and daily thereafter, deleting
-- twins in small bounded batches via TestResultStore.DeleteShellTwinBatch.
-- The MCP read path additionally dedupes defensively by full_name until the
-- cleanup has caught up (merged_history_ids in diagnose_failure output).
--
-- The version number is kept so environments that already recorded 0049 (from
-- the earlier content) and environments that failed mid-flight both converge:
-- goose does not checksum SQL migrations, so re-shipping this file as a no-op
-- lets stuck environments proceed instantly and changes nothing for the rest.
SELECT 1;

-- +goose Down
-- The Up step is a no-op; so is Down.
SELECT 1;
