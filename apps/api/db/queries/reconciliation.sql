-- name: CountActiveSubsWithoutSnapshot :one
SELECT count(*)::bigint FROM subscriptions s
WHERE s.status = 'active'
AND NOT EXISTS (
    SELECT 1 FROM entitlement_snapshots es
    WHERE es.subscription_id = s.id
    AND es.computed_at > now() - interval '24 hours'
);

-- name: CountStuckUsageOutbox :one
SELECT count(*)::bigint FROM outbox_events
WHERE event_type = 'usage.accepted'
AND status = 'failed'
AND (next_attempt_at IS NULL OR next_attempt_at < now() - interval '1 hour');

-- name: CountInvoicesWithoutCatalogVersion :one
SELECT count(*)::bigint FROM invoices
WHERE subscription_id IS NOT NULL
AND catalog_version_id IS NULL;

-- name: CountDeadLetterOutbox :one
SELECT count(*)::bigint FROM outbox_events WHERE status = 'dead_letter';

-- name: CountOrphanedUsageEvents :one
SELECT count(*)::bigint FROM usage_events ue
WHERE NOT EXISTS (
    SELECT 1 FROM customer_accounts ca WHERE ca.id = ue.customer_account_id
);

-- name: InsertReconciliationResult :exec
INSERT INTO reconciliation_results (check_name, status, expected_count, actual_count, drift_count, details)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListRecentReconciliationResults :many
SELECT * FROM reconciliation_results ORDER BY checked_at DESC LIMIT $1;
