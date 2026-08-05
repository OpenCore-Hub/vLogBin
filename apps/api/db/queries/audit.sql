-- name: AuditChainState :one
-- Current tamper-evident chain state: total events and tail hash/id
-- (migration 0031). NULL tail fields mean no events have been written yet.
SELECT
    (SELECT count(*)::bigint FROM audit_events)                AS total_events,
    (SELECT tail_hash FROM audit_chain_tail WHERE id = 1)      AS tail_hash,
    (SELECT tail_event_id FROM audit_chain_tail WHERE id = 1)  AS tail_event_id;

-- name: LatestAuditAnchor :one
-- Most recent anchor checkpoint; pgx.ErrNoRows when none exists yet.
SELECT
    id,
    tail_event_id,
    tail_hash,
    operator,
    created_at
FROM audit_chain_anchors
ORDER BY id DESC
LIMIT 1;

-- name: VerifyAuditChain :one
-- Operator-only chain verification. from_id: 0 (or <= pruned head) starts at
-- the first surviving row; otherwise that row's stored hash is trusted. to_id:
-- 0 means the current tail. Returns ok / range / broken_at / reason.
SELECT
    v.ok::boolean           AS ok,
    v.verified_from::bigint AS verified_from,
    v.verified_to::bigint   AS verified_to,
    v.verified_count::bigint AS verified_count,
    -- broken_at is NULL when the segment is intact; COALESCE keeps the sqlc
    -- column non-nullable so the scan never fails on a healthy chain.
    COALESCE(v.broken_at, 0)::bigint AS broken_at,
    v.reason::text          AS reason
FROM audit_chain_verify(sqlc.arg(from_id)::bigint, sqlc.arg(to_id)::bigint) AS v;

-- name: AnchorAuditChain :one
-- Operator-only checkpoint creation. Records (tail_event_id, tail_hash) for
-- external anchoring (WORM) and incremental verification.
SELECT
    v.anchor_id::bigint      AS anchor_id,
    -- tail_event_id / tail_hash are NULL when the chain is empty; COALESCE keeps
    -- the sqlc columns non-nullable so the scan never fails on an empty chain.
    COALESCE(v.tail_event_id, 0)::bigint AS tail_event_id,
    COALESCE(v.tail_hash, '')::text      AS tail_hash,
    v.events_covered::bigint AS events_covered
FROM anchor_audit_chain(sqlc.arg(operator)::text) AS v;

-- name: PurgeExpiredAuditEvents :one
-- Permanently deletes audit events older than cutoff via the operator-only
-- purge_audit_events function (migration 0030), which is the sole code path
-- allowed to mutate the append-only audit table. Returns the number of rows
-- deleted; max_rows bounds each batch so the retention sweeper can walk a
-- large backlog in short transactions.
SELECT purge_audit_events(sqlc.arg(cutoff)::timestamptz, sqlc.arg(max_rows)::bigint) AS deleted;

-- name: InsertAuditEvent :one
INSERT INTO audit_events (provider_id, environment_id, actor_type, actor_id, action, target_type, target_id, metadata, request_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListAuditEventsByProvider :many
SELECT * FROM audit_events
WHERE provider_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListAuditEventsFiltered :many
-- Keyset-paginated, filterable audit log query (newest first). The cursor is
-- the primary key of the last row from the previous page; tuple comparison
-- on (created_at, id) keeps ordering stable when multiple events share a
-- timestamp within one transaction. Pass 0 as the cursor to start from the
-- newest. Empty strings for action/actor_type/actor_id/target_type/
-- target_id skip that filter; an empty from/to leaves the time bound open.
SELECT ae.* FROM audit_events ae
WHERE ae.provider_id = sqlc.arg(provider_id)
  AND (sqlc.arg(cursor_id)::bigint = 0
       OR (ae.created_at, ae.id) < (
           SELECT ae2.created_at, ae2.id FROM audit_events ae2 WHERE ae2.id = sqlc.arg(cursor_id)))
  AND (sqlc.arg(action)::text = '' OR ae.action = sqlc.arg(action))
  AND (sqlc.arg(actor_type)::text = '' OR ae.actor_type = sqlc.arg(actor_type))
  AND (sqlc.arg(actor_id)::text = '' OR ae.actor_id = sqlc.arg(actor_id))
  AND (sqlc.arg(target_type)::text = '' OR ae.target_type = sqlc.arg(target_type))
  AND (sqlc.arg(target_id)::text = '' OR ae.target_id = sqlc.arg(target_id))
  AND (sqlc.arg(from_time)::text = '' OR ae.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (sqlc.arg(to_time)::text = '' OR ae.created_at <= sqlc.arg(to_time)::timestamptz)
ORDER BY ae.created_at DESC, ae.id DESC
LIMIT sqlc.arg(page_limit);

-- name: CountAuditEventsFiltered :one
-- Total events matching the same filter set as ListAuditEventsFiltered. The
-- audit dashboard renders this as its headline number; the HTTP layer
-- requires bounded from/to so a missing window cannot trigger a full-table
-- scan.
SELECT count(*) AS total
FROM audit_events ae
WHERE ae.provider_id = sqlc.arg(provider_id)
  AND (sqlc.arg(action)::text = '' OR ae.action = sqlc.arg(action))
  AND (sqlc.arg(actor_type)::text = '' OR ae.actor_type = sqlc.arg(actor_type))
  AND (sqlc.arg(actor_id)::text = '' OR ae.actor_id = sqlc.arg(actor_id))
  AND (sqlc.arg(target_type)::text = '' OR ae.target_type = sqlc.arg(target_type))
  AND (sqlc.arg(target_id)::text = '' OR ae.target_id = sqlc.arg(target_id))
  AND (sqlc.arg(from_time)::text = '' OR ae.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (sqlc.arg(to_time)::text = '' OR ae.created_at <= sqlc.arg(to_time)::timestamptz);

-- name: AuditEventActionCounts :many
-- Event counts grouped by action, most frequent first. Shares the filter set
-- with ListAuditEventsFiltered so a dashboard can drill from a count into the
-- matching rows.
SELECT ae.action AS action, count(*) AS event_count
FROM audit_events ae
WHERE ae.provider_id = sqlc.arg(provider_id)
  AND (sqlc.arg(action)::text = '' OR ae.action = sqlc.arg(action))
  AND (sqlc.arg(actor_type)::text = '' OR ae.actor_type = sqlc.arg(actor_type))
  AND (sqlc.arg(actor_id)::text = '' OR ae.actor_id = sqlc.arg(actor_id))
  AND (sqlc.arg(target_type)::text = '' OR ae.target_type = sqlc.arg(target_type))
  AND (sqlc.arg(target_id)::text = '' OR ae.target_id = sqlc.arg(target_id))
  AND (sqlc.arg(from_time)::text = '' OR ae.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (sqlc.arg(to_time)::text = '' OR ae.created_at <= sqlc.arg(to_time)::timestamptz)
GROUP BY ae.action
ORDER BY event_count DESC, ae.action ASC;

-- name: AuditEventActorTypeCounts :many
-- Event counts grouped by actor type (operator/credential/...), most frequent
-- first. Same filter set as the other dashboard queries.
SELECT ae.actor_type AS actor_type, count(*) AS event_count
FROM audit_events ae
WHERE ae.provider_id = sqlc.arg(provider_id)
  AND (sqlc.arg(action)::text = '' OR ae.action = sqlc.arg(action))
  AND (sqlc.arg(actor_type)::text = '' OR ae.actor_type = sqlc.arg(actor_type))
  AND (sqlc.arg(actor_id)::text = '' OR ae.actor_id = sqlc.arg(actor_id))
  AND (sqlc.arg(target_type)::text = '' OR ae.target_type = sqlc.arg(target_type))
  AND (sqlc.arg(target_id)::text = '' OR ae.target_id = sqlc.arg(target_id))
  AND (sqlc.arg(from_time)::text = '' OR ae.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (sqlc.arg(to_time)::text = '' OR ae.created_at <= sqlc.arg(to_time)::timestamptz)
GROUP BY ae.actor_type
ORDER BY event_count DESC, ae.actor_type ASC;

-- name: AuditEventTimeSeries :many
-- Event counts bucketed by date_trunc granularity (hour | day | week); the
-- granularity is validated at the HTTP layer. Buckets with zero events are
-- filled in by the Go service so charts render a contiguous axis.
SELECT date_trunc(sqlc.arg(granularity)::text, ae.created_at)::timestamptz AS bucket,
       count(*) AS event_count
FROM audit_events ae
WHERE ae.provider_id = sqlc.arg(provider_id)
  AND (sqlc.arg(action)::text = '' OR ae.action = sqlc.arg(action))
  AND (sqlc.arg(actor_type)::text = '' OR ae.actor_type = sqlc.arg(actor_type))
  AND (sqlc.arg(actor_id)::text = '' OR ae.actor_id = sqlc.arg(actor_id))
  AND (sqlc.arg(target_type)::text = '' OR ae.target_type = sqlc.arg(target_type))
  AND (sqlc.arg(target_id)::text = '' OR ae.target_id = sqlc.arg(target_id))
  AND (sqlc.arg(from_time)::text = '' OR ae.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (sqlc.arg(to_time)::text = '' OR ae.created_at <= sqlc.arg(to_time)::timestamptz)
GROUP BY 1
ORDER BY 1;

-- name: ListAuditAnchorsForPublish :many
-- Anchors not yet archived to WORM object storage, oldest first. The archiver
-- fetches one batch per sweep and uploads each anchor before marking it
-- published (see MarkAuditAnchorPublished). Anchors never expire and the set
-- is append-only, so batches advance monotonically by id; a small LIMIT keeps
-- each sweep bounded and the upload loop resumable after crashes.
SELECT *
FROM audit_chain_anchors
WHERE published_at IS NULL
ORDER BY id
LIMIT sqlc.arg(batch_size)::int;

-- name: MarkAuditAnchorPublished :execrows
-- Idempotent publish-marking: only transitions an unpublished anchor. The
-- WHERE published_at IS NULL guard makes the update safe under concurrent
-- sweepers; zero affected rows means another worker already archived it, so
-- callers treat it as success. The deterministic object_key allows re-upload
-- of the same content without creating duplicates.
UPDATE audit_chain_anchors
SET published_at = now(),
    object_key   = sqlc.arg(object_key)::text
WHERE id = sqlc.arg(anchor_id)::bigint
  AND published_at IS NULL;
