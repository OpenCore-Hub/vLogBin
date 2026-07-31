-- +goose Up
-- Phase 4: Analytic Plane (spec Section 18).
-- Pre-materialized analytic views for provider dashboards, MAU,
-- conversion, churn, revenue, usage breakdown, and anomaly detection.
-- These views are read-only projections — they don't affect the
-- transactional database (spec: "分析数据是可重建派生数据").

-- Revenue summary by provider + month.
CREATE VIEW analytics_revenue_summary AS
SELECT
    p.id AS provider_id,
    p.name AS provider_name,
    DATE_TRUNC('month', il.created_at) AS month,
    COUNT(DISTINCT il.invoice_id) AS invoice_count,
    COUNT(DISTINCT il.event_transaction_id) AS subscription_count,
    SUM(il.amount_cents) AS total_revenue_cents,
    AVG(il.amount_cents) AS avg_invoice_line_cents
FROM invoice_lines il
JOIN providers p ON p.id = il.provider_id
GROUP BY p.id, p.name, DATE_TRUNC('month', il.created_at);

-- MAU (Monthly Active Users) by provider.
CREATE VIEW analytics_mau AS
SELECT
    ue.provider_id,
    DATE_TRUNC('month', ue.event_timestamp) AS month,
    COUNT(DISTINCT ue.customer_account_id) AS active_customers,
    COUNT(DISTINCT ue.metric_code) AS unique_metrics,
    COUNT(*) AS total_usage_events
FROM usage_events ue
GROUP BY ue.provider_id, DATE_TRUNC('month', ue.event_timestamp);

-- Customer conversion funnel by provider.
CREATE VIEW analytics_conversion AS
SELECT
    ca.provider_id,
    DATE_TRUNC('month', ca.created_at) AS signup_month,
    COUNT(*) AS new_customers,
    COUNT(DISTINCT s.id) AS customers_with_subscription,
    COUNT(DISTINCT s.id) FILTER (WHERE s.status = 'active') AS active_subscriptions
FROM customer_accounts ca
LEFT JOIN subscriptions s ON s.customer_account_id = ca.id
GROUP BY ca.provider_id, DATE_TRUNC('month', ca.created_at);

-- Churn analysis: customers who had a subscription but terminated.
CREATE VIEW analytics_churn AS
SELECT
    s.provider_id,
    DATE_TRUNC('month', COALESCE(s.terminated_at, s.started_at)) AS churn_month,
    COUNT(*) FILTER (WHERE s.status = 'terminated') AS churned_subscriptions,
    COUNT(*) FILTER (WHERE s.status = 'active') AS retained_subscriptions
FROM subscriptions s
GROUP BY s.provider_id, DATE_TRUNC('month', COALESCE(s.terminated_at, s.started_at));

-- Usage breakdown by metric code.
CREATE VIEW analytics_usage_breakdown AS
SELECT
    ue.provider_id,
    ue.metric_code,
    DATE_TRUNC('day', ue.event_timestamp) AS day,
    COUNT(*) AS event_count,
    SUM(COALESCE((ue.properties->>'quantity')::numeric, 1)) AS total_quantity
FROM usage_events ue
GROUP BY ue.provider_id, ue.metric_code, DATE_TRUNC('day', ue.event_timestamp);

-- Anomaly detection: usage spikes > 3x the 7-day rolling average.
CREATE VIEW analytics_usage_anomalies AS
WITH daily_usage AS (
    SELECT
        ue.provider_id,
        ue.metric_code,
        DATE_TRUNC('day', ue.event_timestamp) AS day,
        COUNT(*) AS event_count
    FROM usage_events ue
    GROUP BY ue.provider_id, ue.metric_code, DATE_TRUNC('day', ue.event_timestamp)
),
rolling_avg AS (
    SELECT
        du.provider_id,
        du.metric_code,
        du.day,
        du.event_count,
        AVG(du.event_count) OVER (
            PARTITION BY du.provider_id, du.metric_code
            ORDER BY du.day
            ROWS BETWEEN 7 PRECEDING AND 1 PRECEDING
        ) AS avg_7d
    FROM daily_usage du
)
SELECT
    ra.provider_id,
    ra.metric_code,
    ra.day,
    ra.event_count,
    ra.avg_7d,
    CASE
        WHEN ra.avg_7d > 0 AND ra.event_count > 3 * ra.avg_7d THEN true
        ELSE false
    END AS is_anomaly
FROM rolling_avg ra
WHERE ra.avg_7d > 0 AND ra.event_count > 3 * ra.avg_7d;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT ON analytics_revenue_summary TO platform_app;
        GRANT SELECT ON analytics_mau TO platform_app;
        GRANT SELECT ON analytics_conversion TO platform_app;
        GRANT SELECT ON analytics_churn TO platform_app;
        GRANT SELECT ON analytics_usage_breakdown TO platform_app;
        GRANT SELECT ON analytics_usage_anomalies TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP VIEW IF EXISTS analytics_usage_anomalies;
DROP VIEW IF EXISTS analytics_usage_breakdown;
DROP VIEW IF EXISTS analytics_churn;
DROP VIEW IF EXISTS analytics_conversion;
DROP VIEW IF EXISTS analytics_mau;
DROP VIEW IF EXISTS analytics_revenue_summary;
