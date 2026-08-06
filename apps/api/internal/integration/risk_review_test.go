package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// allChecks returns the full 8-item go-live checklist map (architecture §15).
func allChecks() map[string]bool {
	return map[string]bool{
		"email_and_company_domain": true,
		"tos_dpa":                  true,
		"custom_domain_ownership":  true,
		"payment_tax_connection":   true,
		"webhook_destination":      true,
		"initial_quota":            true,
		"security_contact":         true,
	}
}

func toLiveReview(t *testing.T, providerID string) {
	t.Helper()
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
		operatorToken, map[string]any{"to": "LIVE_REVIEW"})
	if status != http.StatusOK {
		t.Fatalf("to LIVE_REVIEW: status %d, body %v", status, body)
	}
}

// TestRiskReviewGoLiveGate: a provider in LIVE_REVIEW cannot enter LIVE_ACTIVE
// until the operator records an approved risk review. Rejections and missing
// reviews both keep the provider in LIVE_REVIEW; an approval unlocks go-live.
func TestRiskReviewGoLiveGate(t *testing.T) {
	providerID := insertRegisteredProvider(t)
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode})
	if status != http.StatusOK {
		t.Fatalf("activate: status %d, body %v", status, body)
	}
	toLiveReview(t, providerID)

	// 1. No review yet: go-live must be refused with live_review_required.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
		operatorToken, map[string]any{"to": "LIVE_ACTIVE"})
	if status != http.StatusConflict {
		t.Fatalf("go-live without review: status %d, want 409; body %v", status, body)
	}
	if errObj := body["error"].(map[string]any); errObj["code"] != "live_review_required" {
		t.Fatalf("error code = %v, want live_review_required", errObj["code"])
	}

	// 2. A rejected review must still block go-live.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/risk-review", operatorToken,
		map[string]any{
			"risk_score":  70,
			"checks":      map[string]bool{"email_and_company_domain": true},
			"decision":    "rejected",
			"reason":      "company domain not verified",
			"reviewed_by": "op-risk-1",
		})
	if status != http.StatusCreated {
		t.Fatalf("submit rejected review: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
		operatorToken, map[string]any{"to": "LIVE_ACTIVE"})
	if status != http.StatusConflict {
		t.Fatalf("go-live with rejected review: status %d, want 409; body %v", status, body)
	}
	if errObj := body["error"].(map[string]any); errObj["code"] != "live_review_required" {
		t.Fatalf("error code = %v, want live_review_required", errObj["code"])
	}

	// 3. An approved review unlocks go-live.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/risk-review", operatorToken,
		map[string]any{
			"risk_score":  25,
			"checks":      allChecks(),
			"decision":    "approved",
			"reason":      "all checklist items verified",
			"reviewed_by": "op-risk-2",
		})
	if status != http.StatusCreated {
		t.Fatalf("submit approved review: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
		operatorToken, map[string]any{"to": "LIVE_ACTIVE"})
	if status != http.StatusOK {
		t.Fatalf("go-live after approval: status %d, want 200; body %v", status, body)
	}

	// 4. Review history must be visible to the operator, newest first.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/risk-reviews", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list risk reviews: status %d, body %v", status, body)
	}
	reviews, _ := body["reviews"].([]any)
	if len(reviews) != 2 {
		t.Fatalf("review count = %d, want 2; body %v", len(reviews), body)
	}
	latest, _ := reviews[0].(map[string]any)
	if latest["decision"] != "approved" {
		t.Errorf("latest review decision = %v, want approved (newest first)", latest["decision"])
	}
}

// TestRiskReviewAggregateLatestPerProvider: the operator review queue returns
// exactly one row per provider, always the newest review.
func TestRiskReviewAggregateLatestPerProvider(t *testing.T) {
	ids := make([]string, 2)
	for i := range ids {
		ids[i] = insertRegisteredProvider(t)
		status, body := apiReq(t, "POST", "/v1/operator/providers/"+ids[i]+"/activate",
			operatorToken, map[string]any{"home_region_code": regionCode})
		if status != http.StatusOK {
			t.Fatalf("activate %d: status %d, body %v", i, status, body)
		}
		toLiveReview(t, ids[i])
	}

	submitReview := func(providerID, decision, reviewedBy string) {
		t.Helper()
		status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/risk-review", operatorToken,
			map[string]any{
				"risk_score":  30,
				"checks":      allChecks(),
				"decision":    decision,
				"reason":      decision + " review",
				"reviewed_by": reviewedBy,
			})
		if status != http.StatusCreated {
			t.Fatalf("submit %s: status %d, body %v", decision, status, body)
		}
	}

	// Provider A gets rejected then approved; provider B gets one rejection.
	submitReview(ids[0], "rejected", "op-agg-1")
	submitReview(ids[0], "approved", "op-agg-2")
	submitReview(ids[1], "rejected", "op-agg-3")

	status, body := apiReq(t, "GET", "/v1/operator/risk-reviews", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("aggregate risk reviews: status %d, body %v", status, body)
	}
	reviews := body["reviews"].([]any)
	byProvider := make(map[string]map[string]any, len(reviews))
	for _, item := range reviews {
		review := item.(map[string]any)
		providerID := review["provider_id"].(string)
		if providerID == ids[0] || providerID == ids[1] {
			byProvider[providerID] = review
		}
	}
	if len(byProvider) != 2 {
		t.Fatalf("aggregate rows = %d, want 1 per provider (2); body %v", len(byProvider), body)
	}
	if byProvider[ids[0]]["decision"] != "approved" {
		t.Errorf("provider A latest decision = %v, want approved", byProvider[ids[0]]["decision"])
	}
	if byProvider[ids[1]]["decision"] != "rejected" {
		t.Errorf("provider B latest decision = %v, want rejected", byProvider[ids[1]]["decision"])
	}
}

// TestRiskReviewValidation: invalid submissions are rejected before touching
// the database — unknown decisions, out-of-range scores, incomplete checklists
// on approval, and missing reviewers.
func TestRiskReviewValidation(t *testing.T) {
	providerID := insertRegisteredProvider(t)
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode})
	if status != http.StatusOK {
		t.Fatalf("activate: status %d, body %v", status, body)
	}
	toLiveReview(t, providerID)

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "unknown decision",
			body: map[string]any{"risk_score": 10, "checks": allChecks(), "decision": "maybe", "reviewed_by": "op"},
		},
		{
			name: "score below range",
			body: map[string]any{"risk_score": -1, "checks": allChecks(), "decision": "approved", "reviewed_by": "op"},
		},
		{
			name: "score above range",
			body: map[string]any{"risk_score": 101, "checks": allChecks(), "decision": "approved", "reviewed_by": "op"},
		},
		{
			name: "incomplete checklist on approval",
			body: map[string]any{"risk_score": 10, "checks": map[string]bool{"tos_dpa": true}, "decision": "approved", "reviewed_by": "op"},
		},
		{
			name: "missing reviewer",
			body: map[string]any{"risk_score": 10, "checks": allChecks(), "decision": "approved"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/risk-review", operatorToken, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status %d, want 400; body %v", status, body)
			}
		})
	}

	// Validation failures must not create rows.
	var n int
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM provider_risk_reviews WHERE provider_id = $1`, providerID).Scan(&n); err != nil {
		t.Fatalf("count reviews: %v", err)
	}
	if n != 0 {
		t.Errorf("reviews stored after invalid submissions = %d, want 0", n)
	}
}

// TestRiskReviewRequiresLiveReview: a risk review can only be recorded while
// the provider is in LIVE_REVIEW. Recording one for a registered provider is
// refused as a conflict.
func TestRiskReviewRequiresLiveReview(t *testing.T) {
	providerID := insertRegisteredProvider(t)
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/risk-review", operatorToken,
		map[string]any{"risk_score": 10, "checks": allChecks(), "decision": "approved", "reviewed_by": "op"})
	if status != http.StatusConflict {
		t.Fatalf("review on registered provider: status %d, want 409; body %v", status, body)
	}
	if errObj := body["error"].(map[string]any); errObj["code"] != "risk_review_conflict" {
		t.Fatalf("error code = %v, want risk_review_conflict", errObj["code"])
	}
}

// TestRiskReviewOperatorOnly: risk reviews are operator-only resources. A
// provider-scoped token cannot see them and RLS hides the rows entirely.
func TestRiskReviewOperatorOnly(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "risk-op-"+uuid.NewString()[:8])
	toLiveReview(t, providerID)
	submitApprovedRiskReview(t, providerID)

	// Provider token: the operator route must be inaccessible.
	status, body := apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/risk-reviews", apiKey, nil)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("provider token on operator route: status %d, body %v", status, body)
	}

	// RLS: the review row exists (super context) but no provider-context can
	// read it — risk reviews are operator-internal ratings.
	pid := uuid.MustParse(providerID)
	var n int
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM provider_risk_reviews WHERE provider_id = $1`, pid).Scan(&n); err != nil {
		t.Fatalf("count reviews in super context: %v", err)
	}
	if n != 1 {
		t.Fatalf("reviews stored = %d, want 1", n)
	}
	withTenantTx(t, tenantOf(t, pid, uuid.New()), func(tx pgx.Tx) {
		if n := countWhere(t, tx, `SELECT count(*) FROM provider_risk_reviews WHERE provider_id = $1`, pid); n != 0 {
			t.Errorf("tenant-context can see %d risk review rows, want 0", n)
		}
	})
}
