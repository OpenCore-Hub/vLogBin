-- +goose Up
-- Phase 1 invoice lifecycle: invoices synced from the billing engine (Lago)
-- with catalog version traceability. Invoice lines store the metric code,
-- item type and amounts for replay (spec Testing #9). Finalized invoices
-- are immutable at the DB level (trigger guard).

CREATE TABLE invoices (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id         uuid NOT NULL,
    environment_id      uuid NOT NULL,
    lago_id             text NOT NULL,
    number              text NOT NULL DEFAULT '',
    customer_account_id uuid NOT NULL REFERENCES customer_accounts(id),
    subscription_id     uuid REFERENCES subscriptions(id),
    catalog_version_id  uuid,
    issuing_date        date NOT NULL,
    invoice_type        text NOT NULL CHECK (invoice_type IN ('subscription', 'add_on', 'credit', 'one_off', 'progressive_billing')),
    status              text NOT NULL CHECK (status IN ('draft', 'finalized', 'voided', 'pending', 'failed')),
    payment_status      text NOT NULL DEFAULT 'pending' CHECK (payment_status IN ('pending', 'succeeded', 'failed')),
    currency            text NOT NULL CHECK (char_length(currency) = 3),
    fees_amount_cents                   bigint NOT NULL DEFAULT 0,
    coupons_amount_cents                bigint NOT NULL DEFAULT 0,
    credit_notes_amount_cents           bigint NOT NULL DEFAULT 0,
    sub_total_excluding_taxes_amount_cents bigint NOT NULL DEFAULT 0,
    taxes_amount_cents                  bigint NOT NULL DEFAULT 0,
    sub_total_including_taxes_amount_cents bigint NOT NULL DEFAULT 0,
    total_amount_cents                  bigint NOT NULL DEFAULT 0,
    file_url            text,
    web_url             text,
    lago_created_at     timestamptz,
    synced_at           timestamptz NOT NULL DEFAULT now(),
    finalized_at        timestamptz,
    voided_at           timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, lago_id)
);

CREATE TABLE invoice_lines (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id          uuid NOT NULL REFERENCES invoices(id),
    provider_id         uuid NOT NULL,
    environment_id      uuid NOT NULL,
    lago_fee_id         text NOT NULL,
    metric_code         text NOT NULL DEFAULT '',
    item_type           text NOT NULL DEFAULT '',
    item_name           text NOT NULL DEFAULT '',
    units               text NOT NULL DEFAULT '0',
    precise_unit_amount text NOT NULL DEFAULT '0',
    amount_cents        bigint NOT NULL DEFAULT 0,
    taxes_amount_cents  bigint NOT NULL DEFAULT 0,
    total_amount_cents  bigint NOT NULL DEFAULT 0,
    currency            text NOT NULL CHECK (char_length(currency) = 3),
    event_transaction_id text,
    from_date           timestamptz,
    to_date             timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (invoice_id, lago_fee_id)
);

CREATE INDEX idx_invoices_tenant ON invoices (provider_id, environment_id, issuing_date DESC);
CREATE INDEX idx_invoices_customer ON invoices (customer_account_id, issuing_date DESC);
CREATE INDEX idx_invoice_lines_invoice ON invoice_lines (invoice_id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON invoices, invoice_lines TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices FORCE ROW LEVEL SECURITY;
ALTER TABLE invoice_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_lines FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON invoices
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON invoice_lines
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

-- Invoice lines of finalized/voided invoices are immutable (no UPDATE/DELETE).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_finalized_invoice() RETURNS trigger AS $fn$
DECLARE
    v_status text;
BEGIN
    SELECT status INTO v_status FROM invoices WHERE id = COALESCE(NEW.invoice_id, OLD.invoice_id);
    IF v_status IN ('finalized', 'voided') THEN
        RAISE EXCEPTION 'invoice % is %: % not allowed', COALESCE(NEW.invoice_id, OLD.invoice_id), v_status, TG_OP;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER invoice_lines_finalized_guard
    BEFORE UPDATE OR DELETE ON invoice_lines
    FOR EACH ROW EXECUTE FUNCTION guard_finalized_invoice();

-- Invoices that are finalized/voided may only change payment_status,
-- file_url, web_url, synced_at, updated_at (financial fields are immutable).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION invoices_finalized_guard() RETURNS trigger AS $fn$
BEGIN
    IF OLD.status IN ('finalized', 'voided') THEN
        IF NEW.status IS DISTINCT FROM OLD.status
           OR NEW.lago_id IS DISTINCT FROM OLD.lago_id
           OR NEW.number IS DISTINCT FROM OLD.number
           OR NEW.customer_account_id IS DISTINCT FROM OLD.customer_account_id
           OR NEW.subscription_id IS DISTINCT FROM OLD.subscription_id
           OR NEW.catalog_version_id IS DISTINCT FROM OLD.catalog_version_id
           OR NEW.issuing_date IS DISTINCT FROM OLD.issuing_date
           OR NEW.invoice_type IS DISTINCT FROM OLD.invoice_type
           OR NEW.currency IS DISTINCT FROM OLD.currency
           OR NEW.fees_amount_cents IS DISTINCT FROM OLD.fees_amount_cents
           OR NEW.coupons_amount_cents IS DISTINCT FROM OLD.coupons_amount_cents
           OR NEW.credit_notes_amount_cents IS DISTINCT FROM OLD.credit_notes_amount_cents
           OR NEW.sub_total_excluding_taxes_amount_cents IS DISTINCT FROM OLD.sub_total_excluding_taxes_amount_cents
           OR NEW.taxes_amount_cents IS DISTINCT FROM OLD.taxes_amount_cents
       OR NEW.sub_total_including_taxes_amount_cents IS DISTINCT FROM OLD.sub_total_including_taxes_amount_cents
       OR NEW.total_amount_cents IS DISTINCT FROM OLD.total_amount_cents
       OR (OLD.finalized_at IS NOT NULL AND NEW.finalized_at IS DISTINCT FROM OLD.finalized_at)
       OR (OLD.voided_at IS NOT NULL AND NEW.voided_at IS DISTINCT FROM OLD.voided_at) THEN
            RAISE EXCEPTION 'invoice % is %: financial fields are immutable', OLD.id, OLD.status;
        END IF;
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER invoices_finalized_guard
    BEFORE UPDATE ON invoices
    FOR EACH ROW EXECUTE FUNCTION invoices_finalized_guard();

-- +goose Down
DROP TRIGGER IF EXISTS invoices_finalized_guard ON invoices;
DROP FUNCTION IF EXISTS invoices_finalized_guard();
DROP TRIGGER IF EXISTS invoice_lines_finalized_guard ON invoice_lines;
DROP FUNCTION IF EXISTS guard_finalized_invoice();

DROP POLICY IF EXISTS tenant_isolation ON invoice_lines;
DROP POLICY IF EXISTS tenant_isolation ON invoices;

DROP TABLE IF EXISTS invoice_lines;
DROP TABLE IF EXISTS invoices;
