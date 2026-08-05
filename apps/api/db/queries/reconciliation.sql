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

-- name: CountInvoiceAmountMismatches :one
-- Finalized/voided invoices whose financial fields violate Lago's arithmetic
-- invariants: sub_total_including = sub_total_excluding + taxes, and
-- total = sub_total_including - credit_notes. Credit invoices carry a
-- negative total, so they are excluded from the non-negative check.
SELECT count(*)::bigint FROM invoices
WHERE status IN ('finalized', 'voided')
AND (
    sub_total_including_taxes_amount_cents <> sub_total_excluding_taxes_amount_cents + taxes_amount_cents
    OR total_amount_cents <> sub_total_including_taxes_amount_cents - credit_notes_amount_cents
    OR (invoice_type <> 'credit' AND total_amount_cents < 0)
);

-- name: CountInvoiceLinesTotalMismatch :one
-- Finalized/voided invoices whose line totals do not sum to the invoice
-- header total. Line rows are immutable once the invoice is finalized, so
-- any mismatch is a financial integrity violation.
SELECT count(*)::bigint FROM (
    SELECT i.id
    FROM invoices i
    JOIN invoice_lines il ON il.invoice_id = i.id
    WHERE i.status IN ('finalized', 'voided')
    GROUP BY i.id, i.total_amount_cents
    HAVING sum(il.total_amount_cents) <> i.total_amount_cents
) mismatches;

-- name: CountUnpaidFinalizedOverdue :one
-- Finalized invoices not marked paid within 7 days. Signals collection risk
-- and payment-status divergence from the Lago billing system.
SELECT count(*)::bigint FROM invoices
WHERE status = 'finalized'
AND payment_status <> 'succeeded'
AND finalized_at IS NOT NULL
AND finalized_at < now() - interval '7 days';

-- name: InsertReconciliationResult :exec
INSERT INTO reconciliation_results (check_name, status, expected_count, actual_count, drift_count, details)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListRecentReconciliationResults :many
SELECT * FROM reconciliation_results ORDER BY checked_at DESC LIMIT $1;
