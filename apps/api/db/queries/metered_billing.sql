-- name: UpsertMeteredPricingRule :one
INSERT INTO metered_pricing_rules (provider_id, environment_id, metric_code, pricing_model, base_price_cents, tier_config, minimum_spend_cents, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (provider_id, environment_id, metric_code) DO UPDATE
SET pricing_model = EXCLUDED.pricing_model, base_price_cents = EXCLUDED.base_price_cents,
    tier_config = EXCLUDED.tier_config, minimum_spend_cents = EXCLUDED.minimum_spend_cents,
    enabled = EXCLUDED.enabled, updated_at = now()
RETURNING *;

-- name: GetMeteredPricingRule :one
SELECT * FROM metered_pricing_rules
WHERE provider_id = $1 AND environment_id = $2 AND metric_code = $3;

-- name: ListMeteredPricingRules :many
SELECT * FROM metered_pricing_rules
WHERE provider_id = $1 AND environment_id = $2 AND enabled = true
ORDER BY metric_code;

-- name: DeleteMeteredPricingRule :execrows
DELETE FROM metered_pricing_rules
WHERE provider_id = $1 AND environment_id = $2 AND metric_code = $3;

-- name: CreateBudgetAlert :one
INSERT INTO budget_alerts (provider_id, environment_id, subscription_id, metric_code, budget_cents, threshold_pct)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBudgetAlertByID :one
SELECT * FROM budget_alerts WHERE id = $1;

-- name: ListBudgetAlerts :many
SELECT * FROM budget_alerts
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: UpdateBudgetAlertSpend :exec
UPDATE budget_alerts
SET current_spend_cents = $2, alert_status = CASE
    WHEN $2 >= budget_cents THEN 'exceeded'
    WHEN $2 >= (budget_cents * threshold_pct / 100) THEN 'warning'
    ELSE 'ok'
END, last_alerted_at = CASE
    WHEN $2 >= (budget_cents * threshold_pct / 100) THEN now()
    ELSE last_alerted_at
END, updated_at = now()
WHERE id = $1;

-- name: DeleteBudgetAlert :execrows
DELETE FROM budget_alerts
WHERE provider_id = $1 AND environment_id = $2 AND id = $3;
