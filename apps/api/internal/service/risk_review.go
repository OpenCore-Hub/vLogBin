package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrLiveReviewRequired is returned when a provider attempts to transition to
// LIVE_ACTIVE without an approved risk review. Per architecture §15, a provider
// must pass the operator's go-live checklist before entering production.
var ErrLiveReviewRequired = errors.New("live review required")

// ErrRiskReviewConflict is returned when a risk review cannot be recorded,
// e.g. the provider is not awaiting go-live review.
var ErrRiskReviewConflict = errors.New("risk review conflict")

// RiskReviewChecks is the 8-item go-live verification checklist (architecture
// §15). Keys are the storage keys in provider_risk_reviews.checks; values are
// the human-readable checklist items.
var RiskReviewChecks = []string{
	"email_and_company_domain", // 邮箱与企业域名归属
	"tos_dpa",                  // 服务条款与数据处理协议
	"custom_domain_ownership",  // 自定义域所有权
	"payment_tax_connection",   // Payment/Tax Connection 有效
	"webhook_destination",      // Webhook 目的地验证
	"initial_quota",            // 初始配额已分配
	"security_contact",         // 安全联系人已登记
	// risk_score 为第 8 项（独立列，0=低风险，100=高风险），不在此清单中。
}

// RiskReviewInput describes a single risk review submission by an operator.
type RiskReviewInput struct {
	RiskScore  int
	Checks     map[string]bool
	Decision   string // domain.RiskDecisionApproved | domain.RiskDecisionRejected
	Reason     string
	ReviewedBy string // operator actor
}

// SubmitRiskReview records a risk review for a provider. The provider must be
// in LIVE_REVIEW (awaiting go-live review) for a review to be recorded; an
// approval also requires every checklist item to be true and a non-zero
// reviewer. The review and its audit/outbox events are committed atomically.
func (s *Service) SubmitRiskReview(ctx context.Context, providerID uuid.UUID, in RiskReviewInput) (*storegen.ProviderRiskReview, error) {
	if err := validateRiskReview(in); err != nil {
		return nil, err
	}
	var out storegen.ProviderRiskReview
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		p, err := q.GetProviderByID(ctx, providerID)
		if err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		if domain.LifecycleState(p.LifecycleState) != domain.StateLiveReview {
			return fmt.Errorf("%w: provider %s is %s, expected %s",
				ErrRiskReviewConflict, providerID, p.LifecycleState, domain.StateLiveReview)
		}
		raw, err := json.Marshal(in.Checks)
		if err != nil {
			return err
		}
		row, err := q.CreateProviderRiskReview(ctx, storegen.CreateProviderRiskReviewParams{
			ProviderID: providerID,
			RiskScore:  int16(in.RiskScore),
			Checks:     raw,
			Decision:   in.Decision,
			Reason:     in.Reason,
			ReviewedBy: in.ReviewedBy,
		})
		if err != nil {
			return mapErr(err, "risk review for provider %s", providerID)
		}
		out = row
		evt := map[string]any{
			"provider_id": providerID.String(),
			"decision":    in.Decision,
			"risk_score":  in.RiskScore,
		}
		meta := map[string]any{
			"decision":   in.Decision,
			"risk_score": in.RiskScore,
			"checks":     in.Checks,
		}
		if in.Reason != "" {
			evt["reason"] = in.Reason
			meta["reason"] = in.Reason
		}
		envID := envOrTest(ctx, q, providerID, uuid.NullUUID{})
		if err := emitOutboxTx(ctx, q, providerID, envID, "provider", providerID.String(), "provider.risk_reviewed", evt); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{},
			"operator", in.ReviewedBy, "provider.risk_review", "provider", providerID.String(), meta); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// validateRiskReview validates a risk review submission before it touches the
// database. Decisions must be one of approved/rejected; an approval requires
// every checklist item to be true and a named reviewer; risk score must be in
// [0,100]. Rejections may carry a reason and do not require the checklist.
func validateRiskReview(in RiskReviewInput) error {
	if in.ReviewedBy == "" {
		return fmt.Errorf("%w: reviewed_by is required", ErrValidation)
	}
	if in.RiskScore < 0 || in.RiskScore > 100 {
		return fmt.Errorf("%w: risk_score must be between 0 and 100", ErrValidation)
	}
	switch in.Decision {
	case domain.RiskDecisionApproved:
		for _, k := range RiskReviewChecks {
			if !in.Checks[k] {
				return fmt.Errorf("%w: checklist item %q must be verified before approval", ErrValidation, k)
			}
		}
	case domain.RiskDecisionRejected:
		// A rejection may be recorded with an empty checklist and no reason
		// requirement, though a reason is strongly encouraged.
	default:
		return fmt.Errorf("%w: decision must be %q or %q", ErrValidation, domain.RiskDecisionApproved, domain.RiskDecisionRejected)
	}
	return nil
}

// ListRiskReviews returns the risk review history for a provider, newest first.
func (s *Service) ListRiskReviews(ctx context.Context, providerID uuid.UUID) ([]storegen.ProviderRiskReview, error) {
	var out []storegen.ProviderRiskReview
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		rows, err := q.ListProviderRiskReviews(ctx, providerID)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListLatestRiskReviews returns the newest risk review per provider (operator
// review queue). Used by /ops/reviews to avoid a per-provider N+1 fan-out.
func (s *Service) ListLatestRiskReviews(ctx context.Context) ([]storegen.ProviderRiskReview, error) {
	var out []storegen.ProviderRiskReview
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.ListLatestProviderRiskReviews(ctx)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	return out, err
}

// requireApprovedReview returns nil when the provider's latest risk review is
// approved, ErrLiveReviewRequired otherwise.
func requireApprovedReview(ctx context.Context, q *store.Queries, providerID uuid.UUID) error {
	row, err := q.LatestProviderRiskReview(ctx, providerID)
	if err != nil {
		if errors.Is(mapErr(err, "risk review for provider %s", providerID), ErrNotFound) {
			return fmt.Errorf("%w: provider %s has no risk review", ErrLiveReviewRequired, providerID)
		}
		return err
	}
	if row.Decision != domain.RiskDecisionApproved {
		return fmt.Errorf("%w: provider %s latest review is %s", ErrLiveReviewRequired, providerID, row.Decision)
	}
	return nil
}
