-- name: CreateProviderRiskReview :one
INSERT INTO provider_risk_reviews (
    provider_id, risk_score, checks, decision, reason, reviewed_by
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id, provider_id, risk_score, checks, decision, reason, reviewed_by, reviewed_at, created_at;

-- name: LatestProviderRiskReview :one
SELECT id, provider_id, risk_score, checks, decision, reason, reviewed_by, reviewed_at, created_at
FROM provider_risk_reviews
WHERE provider_id = $1
ORDER BY reviewed_at DESC, id DESC
LIMIT 1;

-- name: ListProviderRiskReviews :many
SELECT id, provider_id, risk_score, checks, decision, reason, reviewed_by, reviewed_at, created_at
FROM provider_risk_reviews
WHERE provider_id = $1
ORDER BY reviewed_at DESC, id DESC;
