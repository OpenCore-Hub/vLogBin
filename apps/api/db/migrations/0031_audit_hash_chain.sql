-- +goose Up
-- 0031: Tamper-evident audit hash chain (candidate 20).
--
-- Every audit_events row is chained to its predecessor by a SHA-256 hash that
-- is computed inside a BEFORE INSERT trigger. Application code cannot forge the
-- hash because it never touches prev_hash / event_hash; any UPDATE to the row
-- breaks the chain and is detected by audit_chain_verify(). This makes the
-- append-only guarantee (0002) *verifiable* instead of merely declarative.
--
-- Concurrency: a single-row audit_chain_tail is locked FOR UPDATE inside the
-- trigger, serializing concurrent appends so the chain never forks.
--
-- Retention interaction: purge_audit_events (0030) is replaced here so it
-- records how far the chain head was pruned; verification then resumes after
-- the pruned boundary instead of failing on deleted head rows.
--
-- Honesty note: the DB-internal chain + anchors table are *tamper-evident*,
-- not *tamper-proof* — a database superuser could rewrite the whole chain and
-- anchors. External anchoring (WORM object storage, direction backlog M1)
-- turns this into tamper-proof by publishing anchors outside the DB.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1. Chain columns ----------------------------------------------------------

ALTER TABLE audit_events
    ADD COLUMN prev_hash text,
    ADD COLUMN event_hash text;

-- Event hashes must never repeat: a duplicate signals replayed or forged rows.
CREATE UNIQUE INDEX idx_audit_events_event_hash ON audit_events (event_hash);

-- 2. Canonical per-event hash (deterministic, field-collision safe) ---------
-- ASCII unit separator (0x1f) separates fields; inputs are guaranteed not to
-- contain it. metadata is cast from jsonb whose ::text output is canonical.
-- +goose StatementBegin
CREATE FUNCTION audit_event_hash(p_prev text, ev audit_events)
RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $fn$
SELECT encode(digest(
    COALESCE(p_prev, '') || chr(31) ||
    ev.id::text || chr(31) ||
    COALESCE(ev.provider_id::text, '') || chr(31) ||
    COALESCE(ev.environment_id::text, '') || chr(31) ||
    ev.actor_type || chr(31) ||
    ev.actor_id || chr(31) ||
    ev.action || chr(31) ||
    COALESCE(ev.target_type, '') || chr(31) ||
    COALESCE(ev.target_id, '') || chr(31) ||
    ev.metadata::text || chr(31) ||
    COALESCE(ev.request_id, '') || chr(31) ||
    ev.created_at::text,
    'sha256'), 'hex')
$fn$;
-- +goose StatementEnd

-- 3. Chain tail (single row, serialization point) and anchors ---------------

CREATE TABLE audit_chain_tail (
    id                int PRIMARY KEY CHECK (id = 1),
    tail_hash         text,
    tail_event_id     bigint,
    pruned_through_id bigint,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

INSERT INTO audit_chain_tail (id, tail_hash, tail_event_id, pruned_through_id)
VALUES (1, NULL, NULL, NULL);

-- Anchors are operator-created checkpoints of (tail_event_id, tail_hash).
-- External anchoring (WORM) will publish these outside the DB.
CREATE TABLE audit_chain_anchors (
    id            bigserial PRIMARY KEY,
    tail_event_id bigint NOT NULL,
    tail_hash     text NOT NULL,
    operator      text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- 4. Append trigger ---------------------------------------------------------

-- +goose StatementBegin
CREATE FUNCTION audit_events_hash_trigger()
RETURNS trigger
LANGUAGE plpgsql
AS $fn$
DECLARE
    prev text;
    h    text;
BEGIN
    -- Row lock is held until commit: concurrent appends observe the true tail
    -- and the chain cannot fork.
    SELECT tail_hash INTO prev
      FROM audit_chain_tail
     WHERE id = 1
       FOR UPDATE;

    h := audit_event_hash(prev, NEW);

    UPDATE audit_chain_tail
       SET tail_hash     = h,
           tail_event_id = NEW.id,
           updated_at    = now()
     WHERE id = 1;

    NEW.prev_hash  := prev;
    NEW.event_hash := h;
    RETURN NEW;
END;
$fn$;
-- +goose StatementEnd

CREATE TRIGGER audit_events_hash
BEFORE INSERT ON audit_events
FOR EACH ROW EXECUTE FUNCTION audit_events_hash_trigger();

-- 5. Backfill pre-existing rows (migration role is superuser; the append-only
-- trigger must be bypassed exactly like 0030 does) --------------------------

ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only;

-- +goose StatementBegin
DO $$
DECLARE
    prev text := NULL;
    r    audit_events;
    h    text;
BEGIN
    FOR r IN SELECT * FROM audit_events ORDER BY id LOOP
        h := audit_event_hash(prev, r);
        UPDATE audit_events
           SET prev_hash = prev, event_hash = h
         WHERE id = r.id;
        prev := h;
    END LOOP;

    UPDATE audit_chain_tail
       SET tail_hash     = prev,
           tail_event_id = (SELECT max(id) FROM audit_events),
           updated_at    = now()
     WHERE id = 1;
END $$;
-- +goose StatementEnd

ALTER TABLE audit_events ENABLE TRIGGER audit_events_append_only;

-- 6. Replace purge_audit_events (0030) to track the pruned chain head --------

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION purge_audit_events(p_cutoff timestamptz, p_max_rows bigint)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $fn$
DECLARE
    deleted        bigint := 0;
    pruned_through bigint;
BEGIN
    IF current_setting('app.is_operator', true) <> 'on' THEN
        RAISE EXCEPTION 'audit purge requires operator context (app.is_operator = on)';
    END IF;

    -- Capture the largest id being pruned (same predicate as the DELETE).
    SELECT COALESCE(max(id), 0)
      INTO pruned_through
      FROM (SELECT id FROM audit_events
             WHERE created_at < p_cutoff
             ORDER BY created_at
             LIMIT p_max_rows) doomed;

    ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only;
    DELETE FROM audit_events
     WHERE ctid IN (
        SELECT ctid FROM audit_events
         WHERE created_at < p_cutoff
         ORDER BY created_at
         LIMIT p_max_rows
     );
    GET DIAGNOSTICS deleted = ROW_COUNT;
    ALTER TABLE audit_events ENABLE TRIGGER audit_events_append_only;

    IF deleted > 0 THEN
        UPDATE audit_chain_tail
           SET pruned_through_id = GREATEST(COALESCE(pruned_through_id, 0), pruned_through),
               updated_at        = now()
         WHERE id = 1;
    END IF;

    RETURN deleted;
END;
$fn$;
-- +goose StatementEnd

-- 7. Chain verification (operator-only) -------------------------------------
-- p_from_id semantics: 0 (or <= pruned boundary) means "first surviving row
-- after the pruned head"; otherwise the given row's stored hash is trusted as
-- the starting point. p_to_id: 0 means "current tail". The range (start, to]
-- is verified for link integrity and hash correctness.
-- +goose StatementBegin
CREATE FUNCTION audit_chain_verify(p_from_id bigint, p_to_id bigint)
RETURNS TABLE (
    ok             boolean,
    verified_from  bigint,
    verified_to    bigint,
    verified_count bigint,
    broken_at      bigint,
    reason         text
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $fn$
DECLARE
    pruned_through bigint;
    start_id       bigint;
    prev           text;
    r              audit_events;
    expected       text;
    count_         bigint := 0;
BEGIN
    IF current_setting('app.is_operator', true) <> 'on' THEN
        RAISE EXCEPTION 'audit chain verification requires operator context (app.is_operator = on)';
    END IF;

    SELECT COALESCE(pruned_through_id, 0) INTO pruned_through FROM audit_chain_tail WHERE id = 1;

    IF p_to_id IS NULL OR p_to_id <= 0 THEN
        SELECT COALESCE(tail_event_id, 0) INTO p_to_id FROM audit_chain_tail WHERE id = 1;
    END IF;

    IF p_from_id IS NULL OR p_from_id <= pruned_through THEN
        start_id := pruned_through + 1;
    ELSE
        start_id := p_from_id;
    END IF;

    -- Trusted starting hash: the first surviving row's stored hash. It cannot
    -- be self-verified; it is covered by the pruning boundary or an anchor.
    SELECT event_hash INTO prev FROM audit_events WHERE id = start_id;
    IF prev IS NULL THEN
        RETURN QUERY SELECT true, start_id, start_id, 0::bigint, NULL::bigint, 'ok (no events after start)';
        RETURN;
    END IF;

    FOR r IN SELECT * FROM audit_events
              WHERE id > start_id AND id <= p_to_id
              ORDER BY id LOOP
        IF r.prev_hash IS DISTINCT FROM prev THEN
            RETURN QUERY SELECT false, start_id, r.id, count_, r.id, 'link broken: prev_hash mismatch';
            RETURN;
        END IF;
        expected := audit_event_hash(prev, r);
        IF r.event_hash IS DISTINCT FROM expected THEN
            RETURN QUERY SELECT false, start_id, r.id, count_, r.id, 'event hash mismatch (row modified)';
            RETURN;
        END IF;
        prev := r.event_hash;
        count_ := count_ + 1;
    END LOOP;

    RETURN QUERY SELECT true, start_id,
        COALESCE((SELECT max(id) FROM audit_events WHERE id > start_id AND id <= p_to_id), start_id),
        count_, NULL::bigint, 'ok';
END;
$fn$;
-- +goose StatementEnd

-- 8. Anchor creation (operator-only) ----------------------------------------

-- +goose StatementBegin
CREATE FUNCTION anchor_audit_chain(p_operator text)
RETURNS TABLE (
    anchor_id      bigint,
    tail_event_id  bigint,
    tail_hash      text,
    events_covered bigint
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $fn$
DECLARE
    tail_       text;
    last_event  bigint;
    prev_anchor bigint;
    res_id      bigint;
    t_row       audit_chain_tail%ROWTYPE;
BEGIN
    IF current_setting('app.is_operator', true) <> 'on' THEN
        RAISE EXCEPTION 'audit chain anchoring requires operator context (app.is_operator = on)';
    END IF;

    -- %ROWTYPE field access avoids any ambiguity with the RETURNS TABLE output
    -- columns that share these names.
    SELECT * INTO t_row FROM audit_chain_tail WHERE id = 1;
    tail_      := t_row.tail_hash;
    last_event := t_row.tail_event_id;

    IF last_event IS NULL THEN
        RETURN QUERY SELECT 0::bigint, NULL::bigint, NULL::text, 0::bigint;
        RETURN;
    END IF;

    -- The alias is required: an unqualified tail_event_id is ambiguous with the
    -- RETURNS TABLE output parameter of the same name.
    SELECT COALESCE((SELECT a.tail_event_id FROM audit_chain_anchors a ORDER BY a.id DESC LIMIT 1), 0)
      INTO prev_anchor;

    INSERT INTO audit_chain_anchors (tail_event_id, tail_hash, operator)
    VALUES (last_event, tail_, COALESCE(NULLIF(p_operator, ''), 'manual'))
    RETURNING id INTO res_id;

    RETURN QUERY SELECT res_id, last_event, tail_, GREATEST(last_event - prev_anchor, 0);
END;
$fn$;
-- +goose StatementEnd

-- 9. Permissions ------------------------------------------------------------
-- mirror 0030: revoke from PUBLIC, grant only to platform_app.

REVOKE ALL ON TABLE audit_chain_tail FROM PUBLIC;
REVOKE ALL ON TABLE audit_chain_anchors FROM PUBLIC;
REVOKE ALL ON FUNCTION audit_event_hash(text, audit_events) FROM PUBLIC;
REVOKE ALL ON FUNCTION audit_events_hash_trigger() FROM PUBLIC;
REVOKE ALL ON FUNCTION audit_chain_verify(bigint, bigint) FROM PUBLIC;
REVOKE ALL ON FUNCTION anchor_audit_chain(text) FROM PUBLIC;

GRANT SELECT, UPDATE ON TABLE audit_chain_tail TO platform_app;
GRANT SELECT ON TABLE audit_chain_anchors TO platform_app;
GRANT EXECUTE ON FUNCTION audit_event_hash(text, audit_events) TO platform_app;
GRANT EXECUTE ON FUNCTION audit_events_hash_trigger() TO platform_app;
GRANT EXECUTE ON FUNCTION audit_chain_verify(bigint, bigint) TO platform_app;
GRANT EXECUTE ON FUNCTION anchor_audit_chain(text) TO platform_app;
GRANT EXECUTE ON FUNCTION purge_audit_events(timestamptz, bigint) TO platform_app;
