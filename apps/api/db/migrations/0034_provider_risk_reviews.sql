-- +goose Up
-- 0034_provider_risk_reviews.sql
-- Live Provider 风险审核门禁。
--
-- 架构设计 §15「Provider 生命周期与风险控制」：provider 从 LIVE_REVIEW 进入
-- LIVE_ACTIVE（正式上线）前，operator 必须完成至少 8 项验证并给出审批结论。
-- 本表记录每次风险审核（可多次审核，最新一条决定是否放行）。
--
-- 8 项验证清单（checks jsonb，键均为布尔）：
--   email_and_company_domain   邮箱与企业域名归属
--   tos_dpa                    服务条款与数据处理协议
--   custom_domain_ownership    自定义域所有权
--   payment_tax_connection     Payment/Tax Connection 有效
--   webhook_destination        Webhook 目的地验证
--   initial_quota              初始配额已分配
--   security_contact           安全联系人已登记
-- risk_score 单独一列（0=低风险，100=高风险），即第 8 项「风险评分」。
--
-- RLS：风险评分为 operator 内部评级，仅 operator 可见；provider 不可见。
CREATE TABLE IF NOT EXISTS provider_risk_reviews (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id uuid NOT NULL REFERENCES providers(id),
    risk_score  smallint NOT NULL CHECK (risk_score BETWEEN 0 AND 100),
    checks      jsonb NOT NULL DEFAULT '{}'::jsonb,
    decision    text NOT NULL CHECK (decision IN ('approved', 'rejected')),
    reason      text NOT NULL DEFAULT '',
    reviewed_by text NOT NULL,
    reviewed_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS provider_risk_reviews_provider_reviewed_idx
    ON provider_risk_reviews (provider_id, reviewed_at DESC);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON provider_risk_reviews TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE provider_risk_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE provider_risk_reviews FORCE ROW LEVEL SECURITY;

-- operator 可见全部风险审核记录；provider 不可见（无 provider 策略：
-- 风险评分属 operator 内部评级，provider 不可读）。
CREATE POLICY operator_all ON provider_risk_reviews
    FOR ALL
    USING (current_setting('app.is_operator', true) = 'on')
    WITH CHECK (current_setting('app.is_operator', true) = 'on');

-- +goose Down
DROP TABLE IF EXISTS provider_risk_reviews;
