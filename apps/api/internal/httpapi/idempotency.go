package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// idempotencyMaxKeyLength caps the Idempotency-Key header value (Stripe's
	// documented limit of 255 printable ASCII characters).
	idempotencyMaxKeyLength = 255
	// defaultIdempotencyTTL is how long completed idempotency responses are
	// replayed before the record expires. Overridable via IDEMPOTENCY_TTL.
	defaultIdempotencyTTL = 24 * time.Hour
)

// SetIdempotencyTTL controls how long idempotency records are retained before
// the sweeper purges them. A non-positive value keeps the 24h default. Must be
// called before serving requests.
func (s *Server) SetIdempotencyTTL(ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultIdempotencyTTL
	}
	s.idempotencyTTL = ttl
}

// validIdempotencyKey enforces Stripe's Idempotency-Key contract: 1-255
// printable ASCII characters (0x21-0x7e, no control characters).
func validIdempotencyKey(key string) bool {
	if key == "" || len(key) > idempotencyMaxKeyLength {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x21 || key[i] > 0x7e {
			return false
		}
	}
	return true
}

// idempotencyScope derives the isolation scope for the authenticated caller:
// 'provider:<uuid>' for tenant requests, 'operator:<sub>' for operator ones.
func idempotencyScope(r *http.Request) string {
	if tc, ok := tenant.FromContext(r.Context()); ok {
		return "provider:" + tc.ProviderID.String()
	}
	return "operator:" + operatorIdentity(r)
}

// idempotencyMiddleware implements Stripe-style Idempotency-Key semantics for
// mutating requests: the first execution's response is persisted and replayed
// verbatim for identical retries, protecting against duplicate billing or
// duplicate resource creation caused by network retries.
//
// Contract:
//   - Requests without the header, and non-mutating methods, pass through.
//   - The key must be 1-255 printable ASCII characters (400 otherwise).
//   - The key is scoped to the authenticated identity (provider or operator),
//     method and path; a key reused for a different request is treated as a
//     fresh key.
//   - A completed response (2xx/4xx) is replayed with Idempotency-Replayed.
//   - A duplicate in-flight request for the same key gets 409 concurrent.
//   - 5xx responses are NOT cached so the client may retry the same key.
//
// Bookkeeping is fail-open: lookup/claim/complete errors never block the
// request, they only degrade to "no replay protection" for that attempt.
func (s *Server) idempotencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		if !validIdempotencyKey(key) {
			writeError(w, http.StatusBadRequest, "invalid_idempotency_key",
				"Idempotency-Key must be 1-255 printable ASCII characters", reqIDFromRequest(r))
			return
		}

		keyHash := sha256.Sum256([]byte(key))
		scope := idempotencyScope(r)
		path := r.URL.Path
		ctx := r.Context()

		// Fast path: an earlier attempt already completed → replay verbatim.
		// A still in_progress record means a concurrent duplicate is running.
		if rec, err := s.getIdempotencyKey(ctx, scope, keyHash[:], r.Method, path); err == nil {
			if rec.Status == "completed" {
				s.replayIdempotency(w, rec)
				return
			}
			writeError(w, http.StatusConflict, "concurrent",
				"a request with this Idempotency-Key is already in progress", reqIDFromRequest(r))
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			s.log.Error("idempotency lookup failed", "error", err, "request_id", reqIDFromRequest(r))
			next.ServeHTTP(w, r)
			return
		}

		// Claim the key. Concurrent identical requests race here; the unique
		// constraint lets exactly one INSERT win, the loser gets ErrNoRows.
		rec, created, err := s.claimIdempotencyKey(ctx, scope, keyHash[:], r.Method, path,
			reqIDFromRequest(r), time.Now().UTC().Add(s.idempotencyTTL))
		if err != nil {
			s.log.Error("idempotency claim failed", "error", err, "request_id", reqIDFromRequest(r))
			next.ServeHTTP(w, r)
			return
		}
		if !created {
			// Lost the race. The winner may already have completed.
			if cur, err := s.getIdempotencyKey(ctx, scope, keyHash[:], r.Method, path); err == nil && cur.Status == "completed" {
				s.replayIdempotency(w, cur)
				return
			}
			writeError(w, http.StatusConflict, "concurrent",
				"a request with this Idempotency-Key is already in progress", reqIDFromRequest(r))
			return
		}

		// Execute the request, capturing status/body for the record.
		sr := &idempotencyRecorder{ResponseWriter: w, body: &bytes.Buffer{}}
		next.ServeHTTP(sr, r)

		// No response was written (e.g. panic recovered upstream) or the
		// handler failed: drop the claim so the client can retry.
		if sr.status == 0 || sr.status >= http.StatusInternalServerError {
			if _, err := s.deleteIdempotencyKey(ctx, rec.ID); err != nil {
				s.log.Error("idempotency delete failed", "error", err, "request_id", reqIDFromRequest(r))
			}
			return
		}
		if _, err := s.completeIdempotencyKey(ctx, rec.ID, sr.status,
			sr.Header().Get("Content-Type"), sr.body.Bytes()); err != nil {
			s.log.Error("idempotency complete failed", "error", err, "request_id", reqIDFromRequest(r))
		}
	})
}

// replayIdempotency returns a previously cached response verbatim, tagging it
// with Idempotency-Replayed so callers can distinguish replays.
func (s *Server) replayIdempotency(w http.ResponseWriter, rec storegen.IdempotencyKey) {
	if rec.ContentType.Valid {
		w.Header().Set("Content-Type", rec.ContentType.String)
	}
	w.Header().Set("Idempotency-Replayed", "true")
	if rec.ResponseStatus.Valid {
		w.WriteHeader(int(rec.ResponseStatus.Int32))
	}
	if len(rec.ResponseBody) > 0 {
		_, _ = w.Write(rec.ResponseBody)
	}
}

// execIdempotency routes an idempotency record operation through the tenant
// RLS context when the request carried a tenant identity, otherwise through
// the operator bypass. This keeps idempotency_keys consistent with the
// tenant_isolation policy.
func (s *Server) execIdempotency(ctx context.Context, fn func(q *store.Queries) error) error {
	if tc, ok := tenant.FromContext(ctx); ok {
		return s.store.WithTenant(ctx, tc, func(_ pgx.Tx, q *store.Queries) error { return fn(q) })
	}
	return s.store.WithOperator(ctx, func(_ pgx.Tx, q *store.Queries) error { return fn(q) })
}

func (s *Server) getIdempotencyKey(ctx context.Context, scope string, keyHash []byte, method, path string) (storegen.IdempotencyKey, error) {
	var rec storegen.IdempotencyKey
	err := s.execIdempotency(ctx, func(q *store.Queries) error {
		var err error
		rec, err = q.GetIdempotencyKey(ctx, storegen.GetIdempotencyKeyParams{
			Scope: scope, KeyHash: keyHash, Method: method, Path: path,
		})
		return err
	})
	return rec, err
}

// claimIdempotencyKey inserts the in_progress placeholder. created is false
// when a concurrent request already claimed the same (scope, key, method,
// path) — the unique constraint turns the losing INSERT into pgx.ErrNoRows.
func (s *Server) claimIdempotencyKey(ctx context.Context, scope string, keyHash []byte, method, path, requestID string, expiresAt time.Time) (storegen.IdempotencyKey, bool, error) {
	var rec storegen.IdempotencyKey
	created := false
	err := s.execIdempotency(ctx, func(q *store.Queries) error {
		var err error
		rec, err = q.InsertIdempotencyKey(ctx, storegen.InsertIdempotencyKeyParams{
			Scope: scope, KeyHash: keyHash, Method: method, Path: path,
			RequestID: requestID, ExpiresAt: expiresAt,
		})
		if err == nil {
			created = true
		}
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return rec, false, nil // lost the claim race
	}
	return rec, created, err
}

func (s *Server) completeIdempotencyKey(ctx context.Context, id uuid.UUID, status int, contentType string, body []byte) (int64, error) {
	var n int64
	err := s.execIdempotency(ctx, func(q *store.Queries) error {
		var err error
		n, err = q.CompleteIdempotencyKey(ctx, storegen.CompleteIdempotencyKeyParams{
			ID:             id,
			ResponseStatus: pgtype.Int4{Int32: int32(status), Valid: true},
			ContentType:    pgtype.Text{String: contentType, Valid: contentType != ""},
			ResponseBody:   body,
		})
		return err
	})
	return n, err
}

func (s *Server) deleteIdempotencyKey(ctx context.Context, id uuid.UUID) (int64, error) {
	var n int64
	err := s.execIdempotency(ctx, func(q *store.Queries) error {
		var err error
		n, err = q.DeleteIdempotencyKey(ctx, id)
		return err
	})
	return n, err
}

// idempotencyRecorder forwards the live response to the client while buffering
// status and body for the idempotency record.
type idempotencyRecorder struct {
	http.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (r *idempotencyRecorder) WriteHeader(code int) {
	if r.status != 0 {
		return
	}
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *idempotencyRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
		r.ResponseWriter.WriteHeader(http.StatusOK)
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
