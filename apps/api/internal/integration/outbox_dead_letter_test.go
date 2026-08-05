package integration

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TestOutboxRelayDeadLetterMaxAttempts drives the retry → dead-letter path of
// the relay (0033 dead-letter queue formalization): a usage.accepted event
// whose billing adapter always fails is dead-lettered after maxAttempts with
// the final cause recorded in last_error, is never re-claimed afterwards, is
// surfaced by the reconciliation query CountDeadLetterOutbox, and is only
// swept by the retention policy once past the cutoff.
func TestOutboxRelayDeadLetterMaxAttempts(t *testing.T) {
	// Pre-drain accumulated events from prior tests so the relay spends its
	// batches on this test's event.
	preRelay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := 0; i < 10; i++ {
		_ = preRelay.DrainOnce(testCtx)
		time.Sleep(100 * time.Millisecond)
	}

	a := createProvider(t, "dlq-max")
	tc := tenantOf(t, a.Provider.ID, a.Environments[0].ID)

	// Insert a usage.accepted outbox event with a parseable (empty) payload:
	// the payload only needs to unmarshal into billing.UsageEvent, so the
	// failure is attributed to the billing adapter, not the payload.
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if _, err := tx.Exec(testCtx, `
			INSERT INTO outbox_events (provider_id, environment_id, aggregate_type, aggregate_id, event_type, payload, payload_hash, transaction_id, status)
			VALUES ($1, $2, 'usage_event', $3, 'usage.accepted', '{}', 'dead-letter-test', $4, 'pending')`,
			a.Provider.ID, a.Environments[0].ID, uuid.New(), "dlq-max-"+uuid.NewString()[:8]); err != nil {
			t.Fatalf("insert outbox event: %v", err)
		}
	})

	relay := newTestRelay(failingAdapter{})

	// Attempts 1..maxAttempts: each drain fails delivery and schedules a
	// backoff retry; rewind next_attempt_at so the next drain re-claims it.
	for i := 0; i < 5; i++ {
		if err := relay.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain #%d: %v", i+1, err)
		}
		withTenantTx(t, tc, func(tx pgx.Tx) {
			if _, err := tx.Exec(testCtx, `UPDATE outbox_events SET next_attempt_at = now() - interval '1 second'
				WHERE provider_id = $1 AND event_type = 'usage.accepted' AND status != 'dead_letter'`, a.Provider.ID); err != nil {
				t.Fatalf("rewind next_attempt_at: %v", err)
			}
		})
	}

	// A further drain must be a no-op: the dead-lettered row is never claimed.
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("post-dead-letter drain: %v", err)
	}

	// Terminal state + recorded cause.
	var status, lastError string
	var attempts int32
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if err := tx.QueryRow(testCtx, `SELECT status, coalesce(last_error, ''), attempts FROM outbox_events
			WHERE provider_id = $1 AND event_type = 'usage.accepted'`, a.Provider.ID).
			Scan(&status, &lastError, &attempts); err != nil {
			t.Fatalf("read outbox event: %v", err)
		}
	})
	if status != "dead_letter" {
		t.Fatalf("status = %q, want dead_letter", status)
	}
	if attempts < 5 {
		t.Fatalf("attempts = %d, want >= 5", attempts)
	}
	if want := "simulated billing engine outage"; lastError != want {
		t.Fatalf("last_error = %q, want %q", lastError, want)
	}

	// Reconciliation visibility: CountDeadLetterOutbox (status='dead_letter')
	// must now surface the event — this check was a no-op before 0033.
	var deadCount int64
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if err := tx.QueryRow(testCtx, `SELECT count(*) FROM outbox_events WHERE status = 'dead_letter'`).
			Scan(&deadCount); err != nil {
			t.Fatalf("count dead-letter: %v", err)
		}
	})
	if deadCount == 0 {
		t.Fatal("CountDeadLetterOutbox must surface the dead-lettered event")
	}

	// Retention: a fresh dead-letter must NOT be swept (still within the
	// retention window), and once aged past the cutoff it is deleted.
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if _, err := tx.Exec(testCtx, `DELETE FROM outbox_events
			WHERE provider_id = $1 AND status = 'dead_letter' AND created_at < now() - interval '2 hours'`, a.Provider.ID); err != nil {
			t.Fatalf("retention sweep (fresh): %v", err)
		}
		var n int
		if err := tx.QueryRow(testCtx, `SELECT count(*) FROM outbox_events
			WHERE provider_id = $1 AND status = 'dead_letter'`, a.Provider.ID).Scan(&n); err != nil {
			t.Fatalf("count dead-letter after fresh sweep: %v", err)
		}
		if n != 1 {
			t.Fatalf("fresh dead-letter swept prematurely: want 1 row left, got %d", n)
		}
	})
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if _, err := tx.Exec(testCtx, `UPDATE outbox_events SET created_at = now() - interval '30 days'
			WHERE provider_id = $1 AND status = 'dead_letter'`, a.Provider.ID); err != nil {
			t.Fatalf("age dead-letter: %v", err)
		}
	})
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if _, err := tx.Exec(testCtx, `DELETE FROM outbox_events
			WHERE provider_id = $1 AND status = 'dead_letter' AND created_at < now() - interval '24 hours'`, a.Provider.ID); err != nil {
			t.Fatalf("retention sweep (aged): %v", err)
		}
		var n int
		if err := tx.QueryRow(testCtx, `SELECT count(*) FROM outbox_events
			WHERE provider_id = $1 AND status = 'dead_letter'`, a.Provider.ID).Scan(&n); err != nil {
			t.Fatalf("count dead-letter after aged sweep: %v", err)
		}
		if n != 0 {
			t.Fatalf("aged dead-letter not swept: %d rows left", n)
		}
	})
}

// TestOutboxRelayDeadLetterInvalidPayload covers the other dead-letter entry
// point: a syntactically valid JSON payload (the payload column is jsonb, so
// the DB already guarantees valid JSON) whose shape cannot unmarshal into
// billing.UsageEvent is dead-lettered immediately — retrying would never
// succeed — with the unmarshal error recorded in last_error.
func TestOutboxRelayDeadLetterInvalidPayload(t *testing.T) {
	preRelay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := 0; i < 10; i++ {
		_ = preRelay.DrainOnce(testCtx)
		time.Sleep(100 * time.Millisecond)
	}

	a := createProvider(t, "dlq-payload")
	tc := tenantOf(t, a.Provider.ID, a.Environments[0].ID)

	withTenantTx(t, tc, func(tx pgx.Tx) {
		if _, err := tx.Exec(testCtx, `
			INSERT INTO outbox_events (provider_id, environment_id, aggregate_type, aggregate_id, event_type, payload, payload_hash, transaction_id, status)
			VALUES ($1, $2, 'usage_event', $3, 'usage.accepted', '{"timestamp": 123}', 'invalid-shape-test', $4, 'pending')`,
			a.Provider.ID, a.Environments[0].ID, uuid.New(), "dlq-payload-"+uuid.NewString()[:8]); err != nil {
			t.Fatalf("insert outbox event: %v", err)
		}
	})

	relay := newTestRelay(billing.NewNoop(nil)) // adapter is irrelevant: payload fails first
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	var status, lastError string
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if err := tx.QueryRow(testCtx, `SELECT status, coalesce(last_error, '') FROM outbox_events
			WHERE provider_id = $1 AND event_type = 'usage.accepted'`, a.Provider.ID).
			Scan(&status, &lastError); err != nil {
			t.Fatalf("read outbox event: %v", err)
		}
	})
	if status != "dead_letter" {
		t.Fatalf("status = %q, want dead_letter (invalid payload shape)", status)
	}
	if len(lastError) < len("payload unmarshal: ") {
		t.Fatalf("last_error = %q, want payload unmarshal cause", lastError)
	}

	// Clean up: the reconciliation check outbox_dead_letter counts dead-letters
	// globally (no provider filter), so tests must not leave dead-lettered rows
	// behind — otherwise later tests see drift.
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if _, err := tx.Exec(testCtx, `DELETE FROM outbox_events
			WHERE provider_id = $1 AND event_type = 'usage.accepted'`, a.Provider.ID); err != nil {
			t.Fatalf("cleanup dead-letter: %v", err)
		}
	})
}
