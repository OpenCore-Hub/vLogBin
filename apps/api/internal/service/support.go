package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SupportSessionMaxDuration is the maximum allowed duration for a standard
// JIT support session (4 hours). Emergency sessions use a tighter limit.
var SupportSessionMaxDuration = 4 * time.Hour

// SupportSessionEmergencyMaxDuration is the maximum allowed duration for an
// emergency (break-glass) support session (1 hour).
var SupportSessionEmergencyMaxDuration = 1 * time.Hour

// RequestSupportSessionInput is the parameter bundle for creating a JIT
// support session request.
type RequestSupportSessionInput struct {
	ProviderID      uuid.UUID
	EnvironmentID   uuid.UUID
	AccessType      string // "standard" or "emergency"
	RequestedBy     string // operator identity (OIDC sub or "operator")
	Reason          string
	RequestedScopes []string
	Duration        time.Duration
}

// RequestSupportSession creates a pending support session. The operator
// requests time-limited access to a provider's environment. Standard
// requests require provider approval; emergency requests require
// two-person operator authorization.
func (s *Service) RequestSupportSession(ctx context.Context, in RequestSupportSessionInput) (*storegen.SupportSession, error) {
	if in.RequestedBy == "" {
		return nil, fmt.Errorf("%w: requested_by is required", ErrValidation)
	}
	if in.Reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrValidation)
	}
	if !domain.ValidSupportAccessType(in.AccessType) {
		return nil, fmt.Errorf("%w: access_type must be standard or emergency", ErrValidation)
	}
	if in.Duration <= 0 {
		return nil, fmt.Errorf("%w: duration must be positive", ErrValidation)
	}
	maxDur := SupportSessionMaxDuration
	if in.AccessType == domain.SupportAccessEmergency {
		maxDur = SupportSessionEmergencyMaxDuration
	}
	if in.Duration > maxDur {
		return nil, fmt.Errorf("%w: duration exceeds maximum (%s) for %s access", ErrValidation, maxDur, in.AccessType)
	}
	for _, sc := range in.RequestedScopes {
		if !domain.ValidScope(sc) {
			return nil, fmt.Errorf("%w: unknown scope %q", ErrValidation, sc)
		}
	}

	var session storegen.SupportSession
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		// Verify the provider and environment exist.
		if _, err := q.GetProviderByID(ctx, in.ProviderID); err != nil {
			return mapErr(err, "provider %s", in.ProviderID)
		}
		envs, err := q.ListEnvironmentsByProvider(ctx, in.ProviderID)
		if err != nil {
			return err
		}
		envFound := false
		for _, e := range envs {
			if e.ID == in.EnvironmentID {
				envFound = true
				break
			}
		}
		if !envFound {
			return fmt.Errorf("%w: environment %s does not belong to provider %s", ErrNotFound, in.EnvironmentID, in.ProviderID)
		}

		expiresAt := time.Now().Add(in.Duration)
		ss, err := q.CreateSupportSession(ctx, storegen.CreateSupportSessionParams{
			ProviderID:      in.ProviderID,
			EnvironmentID:   in.EnvironmentID,
			AccessType:      in.AccessType,
			RequestedBy:     in.RequestedBy,
			Reason:          in.Reason,
			RequestedScopes: in.RequestedScopes,
			ExpiresAt:       expiresAt,
		})
		if err != nil {
			return mapErr(err, "support session")
		}

		if err := emitOutboxTx(ctx, q, in.ProviderID, in.EnvironmentID, "support_session", ss.ID.String(), "support.requested", map[string]any{
			"session_id":   ss.ID.String(),
			"access_type":  in.AccessType,
			"requested_by": in.RequestedBy,
			"reason":       in.Reason,
			"expires_at":   expiresAt.UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: in.ProviderID, Valid: true},
			uuid.NullUUID{UUID: in.EnvironmentID, Valid: true},
			"operator", in.RequestedBy, "support.request",
			"support_session", ss.ID.String(),
			map[string]any{"access_type": in.AccessType, "reason": in.Reason, "duration_seconds": int64(in.Duration.Seconds())}); err != nil {
			return err
		}
		session = ss
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// ApproveSupportSession allows a provider to approve a pending standard
// support request on their environment. The provider must hold the
// support:approve scope. On approval, the session transitions to active.
func (s *Service) ApproveSupportSession(ctx context.Context, tc tenant.Ctx, sessionID uuid.UUID) (*storegen.SupportSession, error) {
	var session storegen.SupportSession
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Fetch the session to verify it belongs to this tenant and is pending.
		ss, err := q.GetSupportSessionByID(ctx, sessionID)
		if err != nil {
			return mapErr(err, "support session %s", sessionID)
		}
		if err := checkTenantOwnership(ss.ProviderID, ss.EnvironmentID, tc, "support session", sessionID); err != nil {
			return err
		}
		if ss.Status != domain.SupportStatusRequested {
			return fmt.Errorf("%w: support session %s is not pending (status=%s)", ErrConflict, sessionID, ss.Status)
		}
		if ss.AccessType != domain.SupportAccessStandard {
			return fmt.Errorf("%w: emergency sessions cannot be approved by the provider", ErrValidation)
		}

		updated, err := q.ApproveSupportSession(ctx, storegen.ApproveSupportSessionParams{
			ID:         sessionID,
			ApprovedBy: pgtype.Text{String: tc.CredentialID.String(), Valid: true},
		})
		if err != nil {
			return mapErr(err, "approve support session %s", sessionID)
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "support_session", sessionID.String(), "support.approved", map[string]any{
			"session_id":  sessionID.String(),
			"approved_by": tc.CredentialID.String(),
			"access_type": ss.AccessType,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "support.approve",
			"support_session", sessionID.String(),
			map[string]any{"requested_by": ss.RequestedBy, "access_type": ss.AccessType}); err != nil {
			return err
		}
		session = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// DenySupportSession allows a provider to deny a pending support request
// on their environment. The deny reason is persisted in the row.
func (s *Service) DenySupportSession(ctx context.Context, tc tenant.Ctx, sessionID uuid.UUID, reason string) (*storegen.SupportSession, error) {
	var session storegen.SupportSession
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.GetSupportSessionByID(ctx, sessionID)
		if err != nil {
			return mapErr(err, "support session %s", sessionID)
		}
		if err := checkTenantOwnership(ss.ProviderID, ss.EnvironmentID, tc, "support session", sessionID); err != nil {
			return err
		}
		if ss.Status != domain.SupportStatusRequested {
			return fmt.Errorf("%w: support session %s is not pending (status=%s)", ErrConflict, sessionID, ss.Status)
		}

		updated, err := q.DenySupportSession(ctx, storegen.DenySupportSessionParams{
			ID:           sessionID,
			RevokeReason: pgtype.Text{String: reason, Valid: reason != ""},
		})
		if err != nil {
			return mapErr(err, "deny support session %s", sessionID)
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "support_session", sessionID.String(), "support.denied", map[string]any{
			"session_id":  sessionID.String(),
			"denied_by":   tc.CredentialID.String(),
			"deny_reason": reason,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "support.deny",
			"support_session", sessionID.String(),
			map[string]any{"requested_by": ss.RequestedBy, "deny_reason": reason}); err != nil {
			return err
		}
		session = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// EmergencyFirstApprove records the first operator approval for an
// emergency support request. The session stays in "requested" status
// until a second operator provides the second approval.
func (s *Service) EmergencyFirstApprove(ctx context.Context, sessionID uuid.UUID, approver string) (*storegen.SupportSession, error) {
	if approver == "" {
		return nil, fmt.Errorf("%w: approver is required", ErrValidation)
	}
	var session storegen.SupportSession
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.GetSupportSessionByID(ctx, sessionID)
		if err != nil {
			return mapErr(err, "support session %s", sessionID)
		}
		if ss.Status != domain.SupportStatusRequested {
			return fmt.Errorf("%w: support session %s is not pending (status=%s)", ErrConflict, sessionID, ss.Status)
		}
		if ss.AccessType != domain.SupportAccessEmergency {
			return fmt.Errorf("%w: first approval is only for emergency sessions", ErrValidation)
		}
		if ss.ApprovedBy.Valid {
			return fmt.Errorf("%w: first approval already recorded", ErrConflict)
		}
		// The requester cannot be the first approver (two-person rule).
		if ss.RequestedBy == approver {
			return fmt.Errorf("%w: requester cannot be the approver (two-person rule)", ErrValidation)
		}

		updated, err := q.SetEmergencyFirstApprover(ctx, storegen.SetEmergencyFirstApproverParams{
			ID:         sessionID,
			ApprovedBy: pgtype.Text{String: approver, Valid: true},
		})
		if err != nil {
			return mapErr(err, "first approve support session %s", sessionID)
		}

		if err := emitOutboxTx(ctx, q, ss.ProviderID, ss.EnvironmentID, "support_session", sessionID.String(), "support.first_approved", map[string]any{
			"session_id":     sessionID.String(),
			"first_approver": approver,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: ss.ProviderID, Valid: true},
			uuid.NullUUID{UUID: ss.EnvironmentID, Valid: true},
			"operator", approver, "support.first_approve",
			"support_session", sessionID.String(),
			map[string]any{"requested_by": ss.RequestedBy}); err != nil {
			return err
		}
		session = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// EmergencySecondApprove records the second operator approval for an
// emergency support request, transitioning the session to active. The
// second approver must be different from both the requester and the
// first approver (enforced by the database query and service check).
func (s *Service) EmergencySecondApprove(ctx context.Context, sessionID uuid.UUID, approver string) (*storegen.SupportSession, error) {
	if approver == "" {
		return nil, fmt.Errorf("%w: approver is required", ErrValidation)
	}
	var session storegen.SupportSession
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.GetSupportSessionByID(ctx, sessionID)
		if err != nil {
			return mapErr(err, "support session %s", sessionID)
		}
		if ss.Status != domain.SupportStatusRequested {
			return fmt.Errorf("%w: support session %s is not pending (status=%s)", ErrConflict, sessionID, ss.Status)
		}
		if ss.AccessType != domain.SupportAccessEmergency {
			return fmt.Errorf("%w: second approval is only for emergency sessions", ErrValidation)
		}
		if !ss.ApprovedBy.Valid {
			return fmt.Errorf("%w: first approval not yet recorded", ErrValidation)
		}
		if ss.RequestedBy == approver {
			return fmt.Errorf("%w: requester cannot be the approver (two-person rule)", ErrValidation)
		}
		if ss.ApprovedBy.String == approver {
			return fmt.Errorf("%w: first approver cannot be the second approver (two-person rule)", ErrValidation)
		}

		updated, err := q.ApproveEmergencySupportSession(ctx, storegen.ApproveEmergencySupportSessionParams{
			ID:             sessionID,
			SecondApprover: pgtype.Text{String: approver, Valid: true},
		})
		if err != nil {
			return mapErr(err, "second approve support session %s", sessionID)
		}

		if err := emitOutboxTx(ctx, q, ss.ProviderID, ss.EnvironmentID, "support_session", sessionID.String(), "support.approved", map[string]any{
			"session_id":      sessionID.String(),
			"second_approver": approver,
			"first_approver":  ss.ApprovedBy.String,
			"access_type":     ss.AccessType,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: ss.ProviderID, Valid: true},
			uuid.NullUUID{UUID: ss.EnvironmentID, Valid: true},
			"operator", approver, "support.emergency_approve",
			"support_session", sessionID.String(),
			map[string]any{"requested_by": ss.RequestedBy, "first_approver": ss.ApprovedBy.String}); err != nil {
			return err
		}
		session = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// RevokeSupportSession terminates an active session early. This is the
// operator path: it uses WithOperator and can revoke any active session.
// The revokedBy parameter identifies who terminated the session.
func (s *Service) RevokeSupportSession(ctx context.Context, sessionID uuid.UUID, revokedBy, reason string) (*storegen.SupportSession, error) {
	if revokedBy == "" {
		return nil, fmt.Errorf("%w: revoked_by is required", ErrValidation)
	}
	var session storegen.SupportSession
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.GetSupportSessionByID(ctx, sessionID)
		if err != nil {
			return mapErr(err, "support session %s", sessionID)
		}
		if ss.Status != domain.SupportStatusActive {
			return fmt.Errorf("%w: support session %s is not active (status=%s)", ErrConflict, sessionID, ss.Status)
		}

		updated, err := q.RevokeSupportSession(ctx, storegen.RevokeSupportSessionParams{
			ID:           sessionID,
			RevokedBy:    pgtype.Text{String: revokedBy, Valid: true},
			RevokeReason: pgtype.Text{String: reason, Valid: reason != ""},
		})
		if err != nil {
			return mapErr(err, "revoke support session %s", sessionID)
		}

		if err := emitOutboxTx(ctx, q, ss.ProviderID, ss.EnvironmentID, "support_session", sessionID.String(), "support.revoked", map[string]any{
			"session_id":    sessionID.String(),
			"revoked_by":    revokedBy,
			"revoke_reason": reason,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: ss.ProviderID, Valid: true},
			uuid.NullUUID{UUID: ss.EnvironmentID, Valid: true},
			"operator", revokedBy, "support.revoke",
			"support_session", sessionID.String(),
			map[string]any{"reason": reason}); err != nil {
			return err
		}
		session = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// RevokeSupportSessionAsProvider terminates an active session on the
// provider's own environment. Uses WithTenant (RLS-enforced) so the
// provider can only revoke sessions that belong to their tenant.
func (s *Service) RevokeSupportSessionAsProvider(ctx context.Context, tc tenant.Ctx, sessionID uuid.UUID, reason string) (*storegen.SupportSession, error) {
	var session storegen.SupportSession
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.GetSupportSessionByID(ctx, sessionID)
		if err != nil {
			return mapErr(err, "support session %s", sessionID)
		}
		if err := checkTenantOwnership(ss.ProviderID, ss.EnvironmentID, tc, "support session", sessionID); err != nil {
			return err
		}
		if ss.Status != domain.SupportStatusActive {
			return fmt.Errorf("%w: support session %s is not active (status=%s)", ErrConflict, sessionID, ss.Status)
		}

		updated, err := q.RevokeSupportSession(ctx, storegen.RevokeSupportSessionParams{
			ID:           sessionID,
			RevokedBy:    pgtype.Text{String: tc.CredentialID.String(), Valid: true},
			RevokeReason: pgtype.Text{String: reason, Valid: reason != ""},
		})
		if err != nil {
			return mapErr(err, "revoke support session %s", sessionID)
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "support_session", sessionID.String(), "support.revoked", map[string]any{
			"session_id":    sessionID.String(),
			"revoked_by":    tc.CredentialID.String(),
			"revoke_reason": reason,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "support.revoke",
			"support_session", sessionID.String(),
			map[string]any{"reason": reason}); err != nil {
			return err
		}
		session = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// ListSupportSessions returns all support sessions for the caller's
// tenant (provider view). The provider can see who has accessed or
// requested access to their environment.
func (s *Service) ListSupportSessions(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.SupportSession, error) {
	var out []storegen.SupportSession
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.ListSupportSessionsByTenant(ctx, storegen.ListSupportSessionsByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: limit,
		})
		out = ss
		return err
	})
	return out, err
}

// ListSupportSessionsByProvider returns all support sessions for a
// provider (operator view, cross-environment).
func (s *Service) ListSupportSessionsByProvider(ctx context.Context, providerID uuid.UUID, limit int32) ([]storegen.SupportSession, error) {
	var out []storegen.SupportSession
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.ListSupportSessionsByProvider(ctx, storegen.ListSupportSessionsByProviderParams{
			ProviderID: providerID, Limit: limit,
		})
		out = ss
		return err
	})
	return out, err
}

// ListAllSupportSessions returns the operator-facing JIT access queue across
// all providers. Keeps the review console at O(1) requests instead of N+1.
func (s *Service) ListAllSupportSessions(ctx context.Context, limit int32) ([]storegen.SupportSession, error) {
	var out []storegen.SupportSession
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.ListAllSupportSessions(ctx, limit)
		if err != nil {
			return err
		}
		out = ss
		return nil
	})
	return out, err
}

// ListActiveSupportSessions returns all currently active sessions for
// the caller's tenant. Used by the provider to see who has live access.
func (s *Service) ListActiveSupportSessions(ctx context.Context, tc tenant.Ctx) ([]storegen.SupportSession, error) {
	var out []storegen.SupportSession
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		ss, err := q.ListActiveSupportSessions(ctx, storegen.ListActiveSupportSessionsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = ss
		return err
	})
	return out, err
}

// ExpireSupportSessions batch-expires all sessions past their expiry.
// Called by the background sweeper goroutine.
func (s *Service) ExpireSupportSessions(ctx context.Context) (int64, error) {
	var n int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		count, err := q.ExpireSupportSessions(ctx)
		n = count
		return err
	})
	return n, err
}

// NewSupportExpirySweeper creates a background sweeper that expires
// past-due support sessions at the given interval.
func NewSupportExpirySweeper(svc *Service, interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("support_session", svc.ExpireSupportSessions, interval, log)
}
