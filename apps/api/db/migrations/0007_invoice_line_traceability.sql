-- +goose Up
-- Phase 1: Invoice line per-line traceability (Testing #9). Each line can
-- now be linked to the exact metric and price in the catalog version that
-- was active when the subscription was created. Both columns are nullable:
-- one-off invoices, credit notes, and subscription base fees may not have
-- a metric or price linkage.

ALTER TABLE invoice_lines
    ADD COLUMN metric_id uuid REFERENCES metrics(id),
    ADD COLUMN price_id uuid REFERENCES prices(id);

CREATE INDEX idx_invoice_lines_metric ON invoice_lines (metric_id) WHERE metric_id IS NOT NULL;
CREATE INDEX idx_invoice_lines_price ON invoice_lines (price_id) WHERE price_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_invoice_lines_price;
DROP INDEX IF EXISTS idx_invoice_lines_metric;
ALTER TABLE invoice_lines DROP COLUMN IF EXISTS price_id;
ALTER TABLE invoice_lines DROP COLUMN IF EXISTS metric_id;
