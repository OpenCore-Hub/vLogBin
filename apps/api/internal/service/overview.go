package service

import (
	"context"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/jackc/pgx/v5"
)

// TrendPoint is a single daily value in an overview trend series.
// Date is an ISO-8601 date (YYYY-MM-DD) in UTC.
type TrendPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

// OverviewTrends holds the daily trend series rendered by the console
// overview charts (M2). Revenue is summed from finalized invoices by
// issuing date; usage events are ingestion events counted by created day.
// Both windows are the trailing trendDays days, zero-filled per day so the
// frontend can draw a continuous axis without re-binning.
type OverviewTrends struct {
	Revenue     []TrendPoint `json:"revenue"`
	UsageEvents []TrendPoint `json:"usage_events"`
}

// OverviewStats aggregates cross-provider counts for the console overview
// panel. Single SQL round-trip: eliminates the web-side N+1 fan-out that
// previously issued one HTTP request per provider per metric class and blew
// through the endpoint/credential rate-limit buckets (R29 fix).
type OverviewStats struct {
	PublishedVersions   int64          `json:"published_versions"`
	ActiveSubscriptions int64          `json:"active_subscriptions"`
	Customers           int64          `json:"customers"`
	RevenueCents        int64          `json:"revenue_cents"`
	Trends              OverviewTrends `json:"trends"`
}

const overviewStatsSQL = `
SELECT
  (SELECT count(*) FROM catalog_versions WHERE state = 'published')            AS published_versions,
  (SELECT count(*) FROM subscriptions     WHERE status   = 'active')           AS active_subscriptions,
  (SELECT count(DISTINCT external_id) FROM customer_accounts)                  AS customers,
  (SELECT COALESCE(sum(total_amount_cents), 0) FROM invoices
     WHERE status = 'finalized')                                               AS revenue_cents
`

// trendDays is the trailing window (inclusive of today) for overview charts.
const trendDays = 30

// revenueTrendSQL aggregates finalized invoice amounts by issuing date.
// The invoices.issuing_date column is a bare date; comparing against a UTC
// midnight argument keeps the window aligned with the Go-side series.
const revenueTrendSQL = `
SELECT i.issuing_date::text AS day,
       COALESCE(SUM(i.total_amount_cents), 0)::bigint AS value
FROM invoices i
WHERE i.status = 'finalized' AND i.issuing_date >= $1::date
GROUP BY i.issuing_date
ORDER BY i.issuing_date`

// usageTrendSQL counts ingestion events by created day. Reversals (kind =
// 'reversal') are negative corrections and intentionally excluded; the
// created_at timestamptz is bucketed in UTC so day labels match the Go-side
// zero-fill series.
const usageTrendSQL = `
SELECT (ue.created_at AT TIME ZONE 'UTC')::date::text AS day,
       COUNT(*)::bigint AS value
FROM usage_events ue
WHERE ue.kind = 'ingestion' AND ue.created_at >= $1::timestamptz
GROUP BY (ue.created_at AT TIME ZONE 'UTC')::date
ORDER BY day`

// OverviewStats returns operator-context aggregates across all providers and
// environments in a single transaction (RLS set to operator in WithOperator).
func (s *Service) OverviewStats(ctx context.Context) (OverviewStats, error) {
	var out OverviewStats
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, _ *store.Queries) error {
		if err := tx.QueryRow(ctx, overviewStatsSQL).Scan(
			&out.PublishedVersions,
			&out.ActiveSubscriptions,
			&out.Customers,
			&out.RevenueCents,
		); err != nil {
			return err
		}
		trends, err := loadTrends(ctx, tx)
		if err != nil {
			return err
		}
		out.Trends = trends
		return nil
	})
	if err != nil {
		return OverviewStats{}, err
	}
	return out, nil
}

// loadTrends fetches both trend series within the caller's transaction and
// zero-fills them over the full trailing window.
func loadTrends(ctx context.Context, tx pgx.Tx) (OverviewTrends, error) {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(trendDays - 1))

	var out OverviewTrends
	revenue, err := queryDaily(ctx, tx, revenueTrendSQL, start)
	if err != nil {
		return OverviewTrends{}, err
	}
	usage, err := queryDaily(ctx, tx, usageTrendSQL, start)
	if err != nil {
		return OverviewTrends{}, err
	}
	out.Revenue = fillDaily(start, revenue)
	out.UsageEvents = fillDaily(start, usage)
	return out, nil
}

// queryDaily scans a (day text, value bigint) result set into a day map.
func queryDaily(ctx context.Context, tx pgx.Tx, sql string, start time.Time) (map[string]int64, error) {
	rows, err := tx.Query(ctx, sql, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make(map[string]int64, trendDays)
	for rows.Next() {
		var day string
		var v int64
		if err := rows.Scan(&day, &v); err != nil {
			return nil, err
		}
		values[day] = v
	}
	return values, rows.Err()
}

// fillDaily materializes a zero-filled, chronologically ordered series over
// the trailing window, so sparse days render as 0 instead of gaps.
func fillDaily(start time.Time, values map[string]int64) []TrendPoint {
	points := make([]TrendPoint, 0, trendDays)
	for i := 0; i < trendDays; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		points = append(points, TrendPoint{Date: day, Value: values[day]})
	}
	return points
}
