-- +goose Up
-- 0032: WORM audit anchor archive (candidate 25).
--
-- 0031 introduced a DB-internal, tamper-evident audit hash chain. Its anchors
-- (audit_chain_anchors) checkpoint the chain tail, but a database superuser
-- could still rewrite both the chain and the anchors. This migration makes the
-- anchors *externally verifiable*: it adds a publish state (published_at /
-- object_key) so the audit-archiver worker can push each anchor to WORM object
-- storage (S3 / MinIO) outside the DB. Once an object exists in the bucket,
-- the archived (tail_event_id, tail_hash) pair can be re-checked against the
-- live chain by a separate tool; the two copies must agree, so a DB-side
-- rewrite no longer goes unnoticed (tamper-proof in practice).
--
-- Idempotency: publishing is a two-step, retry-safe protocol. The archiver
-- first PUTs the anchor object to the bucket, then marks the row published
-- (UPDATE ... WHERE published_at IS NULL). A crash between the two steps
-- leaves the row unpublished, so the next sweep re-uploads the same object
-- under the same key (overwrite is a no-op for identical content). Duplicate
-- objects are therefore impossible; object_key is deterministic.
--
-- The DB role never learns bucket credentials; the archiver process (the API
-- binary) holds them. The bucket is expected to be configured with object
-- lock / retention so archived anchors cannot be deleted or overwritten.

-- 1. Publish state columns ---------------------------------------------------

ALTER TABLE audit_chain_anchors
    ADD COLUMN published_at timestamptz,
    ADD COLUMN object_key   text;

-- Partial index: only unpublished rows are scanned by the archiver, and the
-- WHERE published_at IS NULL predicate guarantees MarkAuditAnchorPublished
-- touches at most one row per anchor_id (concurrent sweepers safe).
CREATE INDEX idx_audit_chain_anchors_unpublished
    ON audit_chain_anchors (id)
    WHERE published_at IS NULL;

-- 2. Permissions -------------------------------------------------------------
-- mirror 0031: revoke from PUBLIC, grant to platform_app. UPDATE is now needed
-- so the archiver can mark anchors published; the published_at guard keeps
-- marking idempotent. Columns stay readable by platform_app as before.

REVOKE ALL ON TABLE audit_chain_anchors FROM PUBLIC;
GRANT SELECT, UPDATE ON TABLE audit_chain_anchors TO platform_app;

-- +goose Down
-- The publish state is metadata only: dropping it loses nothing of the chain
-- itself, so the migration is reversible without weakening the anchor data.

DROP INDEX IF EXISTS idx_audit_chain_anchors_unpublished;
ALTER TABLE audit_chain_anchors
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS object_key;
