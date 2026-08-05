-- Phase 1 billing core queries. All statements run inside tenant-scoped
-- (WithTenant) or operator (WithOperator) transactions; RLS enforces the
-- provider/environment boundary.

-- ---- catalog versions ----

-- name: NextCatalogVersionNumber :one
SELECT COALESCE(MAX(version), 0) + 1 FROM catalog_versions
WHERE provider_id = $1 AND environment_id = $2;

-- name: InsertCatalogVersion :one
INSERT INTO catalog_versions (provider_id, environment_id, version)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetCatalogVersionByID :one
SELECT * FROM catalog_versions WHERE id = $1;

-- name: ListCatalogVersionsByTenant :many
SELECT * FROM catalog_versions
WHERE provider_id = $1 AND environment_id = $2
ORDER BY version DESC;

-- name: UpdateCatalogVersionState :one
UPDATE catalog_versions
SET state = $2,
    validated_at = CASE WHEN $2 = 'validated' THEN now() ELSE validated_at END,
    published_at = CASE WHEN $2 = 'published' THEN now() ELSE published_at END,
    retired_at   = CASE WHEN $2 = 'retired'   THEN now() ELSE retired_at END
WHERE id = $1
RETURNING *;

-- ---- metrics ----

-- name: InsertMetric :one
INSERT INTO metrics (catalog_version_id, provider_id, environment_id, code, name, aggregation_type, field_name, billable)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListMetricsByVersion :many
SELECT * FROM metrics WHERE catalog_version_id = $1 ORDER BY code;

-- name: DeleteMetricsByVersion :exec
DELETE FROM metrics WHERE catalog_version_id = $1;

-- name: GetMetricByVersionAndCode :one
SELECT * FROM metrics WHERE catalog_version_id = $1 AND code = $2;

-- ---- plans ----

-- name: InsertPlan :one
INSERT INTO plans (catalog_version_id, provider_id, environment_id, code, name, interval, currency)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPlansByVersion :many
SELECT * FROM plans WHERE catalog_version_id = $1 ORDER BY code;

-- name: DeletePlansByVersion :exec
DELETE FROM plans WHERE catalog_version_id = $1;

-- name: GetPlanByVersionAndCode :one
SELECT * FROM plans WHERE catalog_version_id = $1 AND code = $2;

-- name: UpdatePlan :one
UPDATE plans
SET name = $2, interval = $3, currency = $4
WHERE id = $1
RETURNING *;

-- name: DeletePlanByVersionAndCode :execrows
DELETE FROM plans WHERE catalog_version_id = $1 AND code = $2;

-- name: GetDraftCatalogVersionByTenant :one
SELECT * FROM catalog_versions
WHERE provider_id = $1 AND environment_id = $2 AND state = 'draft'
ORDER BY version DESC
LIMIT 1
FOR UPDATE;

-- name: GetLatestPublishedCatalogVersionByTenant :one
SELECT * FROM catalog_versions
WHERE provider_id = $1 AND environment_id = $2 AND state = 'published'
ORDER BY version DESC
LIMIT 1;

-- ---- prices ----

-- name: InsertPrice :one
INSERT INTO prices (plan_id, catalog_version_id, provider_id, environment_id, metric_id, charge_model, properties)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPricesByVersion :many
SELECT * FROM prices WHERE catalog_version_id = $1 ORDER BY id;

-- name: ListPricesByPlan :many
SELECT * FROM prices WHERE plan_id = $1;

-- name: DeletePricesByVersion :exec
DELETE FROM prices WHERE catalog_version_id = $1;

-- name: DeletePricesByPlan :exec
DELETE FROM prices WHERE plan_id = $1;

-- ---- entitlement grants ----

-- name: InsertEntitlementGrant :one
INSERT INTO entitlement_grants (plan_id, catalog_version_id, provider_id, environment_id, key, value_type, value)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListGrantsByVersion :many
SELECT * FROM entitlement_grants WHERE catalog_version_id = $1 ORDER BY key;

-- name: ListGrantsByPlan :many
SELECT * FROM entitlement_grants WHERE plan_id = $1;

-- name: DeleteGrantsByVersion :exec
DELETE FROM entitlement_grants WHERE catalog_version_id = $1;

-- name: DeleteGrantsByPlan :exec
DELETE FROM entitlement_grants WHERE plan_id = $1;

-- name: UpsertEntitlementGrant :one
INSERT INTO entitlement_grants (plan_id, catalog_version_id, provider_id, environment_id, key, value_type, value)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (plan_id, key) DO UPDATE SET value_type = EXCLUDED.value_type, value = EXCLUDED.value
RETURNING *;

-- name: DeleteEntitlementGrantByKey :execrows
DELETE FROM entitlement_grants WHERE plan_id = $1 AND key = $2;

-- ---- customer accounts ----

-- name: InsertCustomerAccount :one
INSERT INTO customer_accounts (provider_id, environment_id, external_id, account_type, display_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetCustomerByExternalID :one
SELECT * FROM customer_accounts
WHERE provider_id = $1 AND environment_id = $2 AND external_id = $3;

-- name: GetCustomerByID :one
SELECT * FROM customer_accounts WHERE id = $1;

-- name: ListCustomerAccounts :many
SELECT * FROM customer_accounts
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- ---- subscriptions ----

-- name: InsertSubscription :one
INSERT INTO subscriptions (provider_id, environment_id, external_id, customer_account_id, catalog_version_id, plan_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSubscriptionByID :one
SELECT * FROM subscriptions WHERE id = $1;

-- name: GetSubscriptionByExternalID :one
SELECT * FROM subscriptions
WHERE provider_id = $1 AND environment_id = $2 AND external_id = $3;

-- name: GetActiveSubscriptionByCustomer :one
SELECT * FROM subscriptions
WHERE customer_account_id = $1 AND status = 'active'
ORDER BY started_at DESC
LIMIT 1;

-- name: ListSubscriptionsByTenant :many
SELECT * FROM subscriptions
WHERE provider_id = $1 AND environment_id = $2
ORDER BY started_at DESC
LIMIT $3;

-- name: TerminateSubscription :one
UPDATE subscriptions
SET status = 'terminated', terminated_at = now()
WHERE id = $1
RETURNING *;

-- ---- usage events ----

-- name: InsertUsageEvent :one
INSERT INTO usage_events (provider_id, environment_id, transaction_id, kind, metric_code,
    customer_account_id, subscription_id, event_timestamp, properties, payload_hash, reverses_id, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetUsageEventByTransactionID :one
SELECT * FROM usage_events
WHERE provider_id = $1 AND environment_id = $2 AND transaction_id = $3;

-- name: GetReversalForUsageEvent :one
SELECT * FROM usage_events WHERE reverses_id = $1;

-- name: ListUsageEventsByTenant :many
SELECT * FROM usage_events
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- ---- entitlement overrides ----

-- name: UpsertEntitlementOverride :one
INSERT INTO entitlement_overrides (provider_id, environment_id, subscription_id, key, value_type, value, expires_at, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (subscription_id, key)
DO UPDATE SET value_type = EXCLUDED.value_type,
    value = EXCLUDED.value,
    expires_at = EXCLUDED.expires_at,
    reason = EXCLUDED.reason
RETURNING *;

-- name: ListOverridesBySubscription :many
SELECT * FROM entitlement_overrides WHERE subscription_id = $1 ORDER BY key;

-- name: DeleteEntitlementOverride :execrows
DELETE FROM entitlement_overrides WHERE subscription_id = $1 AND key = $2;

-- ---- entitlement snapshots ----

-- name: InsertEntitlementSnapshot :one
INSERT INTO entitlement_snapshots (provider_id, environment_id, customer_account_id, subscription_id, catalog_version_id, payload)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- ---- operator views (join across environments; operator context only) ----

-- name: ListCatalogVersionsByProvider :many
SELECT cv.id, cv.environment_id, e.kind AS environment_kind, cv.version, cv.state,
    (SELECT count(*) FROM metrics m WHERE m.catalog_version_id = cv.id) AS metrics_count,
    (SELECT count(*) FROM plans p WHERE p.catalog_version_id = cv.id) AS plans_count,
    cv.created_at, cv.published_at
FROM catalog_versions cv
JOIN environments e ON e.id = cv.environment_id
WHERE cv.provider_id = $1
ORDER BY cv.created_at DESC;

-- name: ListSubscriptionsByProvider :many
SELECT s.id, s.external_id, ca.external_id AS customer_external_id, p.code AS plan_code,
    s.catalog_version_id, s.status, e.kind AS environment_kind, s.started_at, s.terminated_at
FROM subscriptions s
JOIN customer_accounts ca ON ca.id = s.customer_account_id
JOIN plans p ON p.id = s.plan_id
JOIN environments e ON e.id = s.environment_id
WHERE s.provider_id = $1
ORDER BY s.started_at DESC;

-- name: ListCustomersByProvider :many
SELECT ca.id, ca.external_id, ca.account_type, ca.display_name, ca.environment_id,
    e.kind AS environment_kind, ca.created_at
FROM customer_accounts ca
JOIN environments e ON e.id = ca.environment_id
WHERE ca.provider_id = $1
ORDER BY ca.created_at DESC;

-- name: ListUsageEventsByProvider :many
SELECT ue.id, ue.transaction_id, ue.kind, ue.metric_code,
    ca.external_id AS customer_external_id, ue.environment_id, e.kind AS environment_kind,
    ue.event_timestamp, ue.created_at
FROM usage_events ue
JOIN customer_accounts ca ON ca.id = ue.customer_account_id
JOIN environments e ON e.id = ue.environment_id
WHERE ue.provider_id = $1
ORDER BY ue.created_at DESC
LIMIT 200;

-- name: ListSubscriptionsByCustomer :many
SELECT s.id, s.external_id, ca.external_id AS customer_external_id, p.code AS plan_code,
    s.catalog_version_id, s.status, e.kind AS environment_kind, s.started_at, s.terminated_at
FROM subscriptions s
JOIN customer_accounts ca ON ca.id = s.customer_account_id
JOIN plans p ON p.id = s.plan_id
JOIN environments e ON e.id = s.environment_id
WHERE s.customer_account_id = $1
ORDER BY s.started_at DESC
LIMIT $2;

-- name: ListUsageEventsByCustomer :many
SELECT ue.id, ue.transaction_id, ue.kind, ue.metric_code,
    ca.external_id AS customer_external_id, ue.environment_id, e.kind AS environment_kind,
    ue.event_timestamp, ue.created_at
FROM usage_events ue
JOIN customer_accounts ca ON ca.id = ue.customer_account_id
JOIN environments e ON e.id = ue.environment_id
WHERE ue.customer_account_id = $1
ORDER BY ue.created_at DESC
LIMIT $2;

-- ---- invoices ----

-- name: UpsertInvoice :one
INSERT INTO invoices (
    provider_id, environment_id, lago_id, number,
    customer_account_id, subscription_id, catalog_version_id,
    issuing_date, invoice_type, status, payment_status, currency,
    fees_amount_cents, coupons_amount_cents, credit_notes_amount_cents,
    sub_total_excluding_taxes_amount_cents, taxes_amount_cents,
    sub_total_including_taxes_amount_cents, total_amount_cents,
    file_url, web_url, lago_created_at, finalized_at, voided_at, synced_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
    $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
) ON CONFLICT (provider_id, environment_id, lago_id) DO UPDATE SET
    number = EXCLUDED.number,
    status = EXCLUDED.status,
    payment_status = EXCLUDED.payment_status,
    currency = EXCLUDED.currency,
    fees_amount_cents = EXCLUDED.fees_amount_cents,
    coupons_amount_cents = EXCLUDED.coupons_amount_cents,
    credit_notes_amount_cents = EXCLUDED.credit_notes_amount_cents,
    sub_total_excluding_taxes_amount_cents = EXCLUDED.sub_total_excluding_taxes_amount_cents,
    taxes_amount_cents = EXCLUDED.taxes_amount_cents,
    sub_total_including_taxes_amount_cents = EXCLUDED.sub_total_including_taxes_amount_cents,
    total_amount_cents = EXCLUDED.total_amount_cents,
    file_url = EXCLUDED.file_url,
    web_url = EXCLUDED.web_url,
    finalized_at = EXCLUDED.finalized_at,
    voided_at = EXCLUDED.voided_at,
    synced_at = EXCLUDED.synced_at
RETURNING *;

-- name: DeleteInvoiceLinesByInvoice :exec
DELETE FROM invoice_lines WHERE invoice_id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: InsertInvoiceLine :one
INSERT INTO invoice_lines (
    invoice_id, provider_id, environment_id, lago_fee_id,
    metric_code, item_type, item_name, units, precise_unit_amount,
    amount_cents, taxes_amount_cents, total_amount_cents, currency,
    event_transaction_id, from_date, to_date, metric_id, price_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
) RETURNING *;

-- name: GetInvoiceByID :one
SELECT * FROM invoices WHERE id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: ListInvoiceLinesByInvoice :many
SELECT * FROM invoice_lines WHERE invoice_id = $1 AND provider_id = $2 AND environment_id = $3 ORDER BY created_at;

-- name: ListInvoicesByTenant :many
SELECT * FROM invoices WHERE provider_id = $1 AND environment_id = $2 ORDER BY issuing_date DESC LIMIT $3;

-- name: ListInvoicesByProvider :many
SELECT i.id, i.number, i.lago_id, i.issuing_date, i.invoice_type, i.status, i.payment_status,
       i.currency, i.total_amount_cents, i.customer_account_id, i.subscription_id, i.catalog_version_id,
       ca.external_id AS customer_external_id, i.environment_id, e.kind AS environment_kind
FROM invoices i
JOIN customer_accounts ca ON ca.id = i.customer_account_id
JOIN environments e ON e.id = i.environment_id
WHERE i.provider_id = $1
ORDER BY i.issuing_date DESC LIMIT 200;

-- name: ListInvoicesByCustomer :many
SELECT i.id, i.number, i.lago_id, i.issuing_date, i.invoice_type, i.status, i.payment_status,
       i.currency, i.total_amount_cents, i.customer_account_id, i.subscription_id, i.catalog_version_id,
       ca.external_id AS customer_external_id, i.environment_id, e.kind AS environment_kind
FROM invoices i
JOIN customer_accounts ca ON ca.id = i.customer_account_id
JOIN environments e ON e.id = i.environment_id
WHERE i.customer_account_id = $1
ORDER BY i.issuing_date DESC LIMIT $2;

-- name: GetInvoiceStatusByLagoID :one
SELECT id, status FROM invoices WHERE provider_id = $1 AND environment_id = $2 AND lago_id = $3;

-- name: CheckUsageInvoiced :one
-- Returns true if any invoice line references the given transaction_id,
-- meaning the usage has been included in an invoice and cannot be directly
-- reversed (Testing #6 post-invoice reversal).
SELECT EXISTS(
    SELECT 1 FROM invoice_lines
    WHERE event_transaction_id = $1
    AND provider_id = $2
    AND environment_id = $3
) AS invoiced;

-- name: CountUninvoicedUsage :one
-- Counts usage events that have not yet been included in an invoice.
-- Used by CompleteFailover to report replay counts (spec Section 14:
-- "切换后重放未确认 Usage").
SELECT count(*)::bigint FROM usage_events ue
WHERE ue.provider_id = $1
    AND NOT EXISTS (
        SELECT 1 FROM invoice_lines il WHERE il.event_transaction_id = ue.transaction_id
        AND il.provider_id = ue.provider_id AND il.environment_id = ue.environment_id
    );
