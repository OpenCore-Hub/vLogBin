package service

import (
	"context"
	"fmt"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// RevenueSummary is a monthly revenue summary for a provider.
type RevenueSummary struct {
	ProviderID         string  `json:"provider_id"`
	Month              string  `json:"month"`
	InvoiceCount       int64   `json:"invoice_count"`
	SubscriptionCount  int64   `json:"subscription_count"`
	TotalRevenueCents  int64   `json:"total_revenue_cents"`
	AvgInvoiceLineCents float64 `json:"avg_invoice_line_cents"`
}

// MAUSummary is a Monthly Active Users summary.
type MAUSummary struct {
	ProviderID        string `json:"provider_id"`
	Month             string `json:"month"`
	ActiveCustomers   int64  `json:"active_customers"`
	UniqueMetrics     int64  `json:"unique_metrics"`
	TotalUsageEvents  int64  `json:"total_usage_events"`
}

// ConversionSummary is a customer conversion funnel summary.
type ConversionSummary struct {
	ProviderID             string `json:"provider_id"`
	SignupMonth            string `json:"signup_month"`
	NewCustomers           int64  `json:"new_customers"`
	CustomersWithSubscription int64 `json:"customers_with_subscription"`
	ActiveSubscriptions    int64  `json:"active_subscriptions"`
}

// ChurnSummary is a customer churn analysis summary.
type ChurnSummary struct {
	ProviderID              string `json:"provider_id"`
	ChurnMonth              string `json:"churn_month"`
	ChurnedSubscriptions    int64  `json:"churned_subscriptions"`
	RetainedSubscriptions   int64  `json:"retained_subscriptions"`
}

// UsageBreakdownSummary is a daily usage breakdown by metric.
type UsageBreakdownSummary struct {
	ProviderID    string  `json:"provider_id"`
	MetricCode    string  `json:"metric_code"`
	Day           string  `json:"day"`
	EventCount    int64   `json:"event_count"`
	TotalQuantity float64 `json:"total_quantity"`
}

// AnomalySummary is a usage anomaly detection result.
type AnomalySummary struct {
	ProviderID   string  `json:"provider_id"`
	MetricCode   string  `json:"metric_code"`
	Day          string  `json:"day"`
	EventCount   int64   `json:"event_count"`
	Avg7d        float64 `json:"avg_7d"`
	IsAnomaly    bool    `json:"is_anomaly"`
}

// GetRevenueSummary returns monthly revenue summaries for the provider.
func (s *Service) GetRevenueSummary(ctx context.Context, tc tenant.Ctx, months int) ([]RevenueSummary, error) {
	if months <= 0 || months > 24 {
		months = 12
	}
	var out []RevenueSummary
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := tx.Query(ctx, `
			SELECT provider_id::text, month::text, invoice_count, subscription_count,
			       total_revenue_cents, avg_invoice_line_cents
			FROM analytics_revenue_summary
			WHERE provider_id = $1
				AND month >= DATE_TRUNC('month', now()) - INTERVAL '1 month' * $2
			ORDER BY month DESC
		`, tc.ProviderID, months)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r RevenueSummary
			if err := rows.Scan(&r.ProviderID, &r.Month, &r.InvoiceCount, &r.SubscriptionCount,
				&r.TotalRevenueCents, &r.AvgInvoiceLineCents); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// GetMAUSummary returns Monthly Active Users data for the provider.
func (s *Service) GetMAUSummary(ctx context.Context, tc tenant.Ctx, months int) ([]MAUSummary, error) {
	if months <= 0 || months > 24 {
		months = 12
	}
	var out []MAUSummary
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := tx.Query(ctx, `
			SELECT provider_id::text, month::text, active_customers, unique_metrics, total_usage_events
			FROM analytics_mau
			WHERE provider_id = $1
				AND month >= DATE_TRUNC('month', now()) - INTERVAL '1 month' * $2
			ORDER BY month DESC
		`, tc.ProviderID, months)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r MAUSummary
			if err := rows.Scan(&r.ProviderID, &r.Month, &r.ActiveCustomers, &r.UniqueMetrics,
				&r.TotalUsageEvents); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// GetConversionSummary returns customer conversion funnel data.
func (s *Service) GetConversionSummary(ctx context.Context, tc tenant.Ctx, months int) ([]ConversionSummary, error) {
	if months <= 0 || months > 24 {
		months = 12
	}
	var out []ConversionSummary
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := tx.Query(ctx, `
			SELECT provider_id::text, signup_month::text, new_customers,
			       customers_with_subscription, active_subscriptions
			FROM analytics_conversion
			WHERE provider_id = $1
				AND signup_month >= DATE_TRUNC('month', now()) - INTERVAL '1 month' * $2
			ORDER BY signup_month DESC
		`, tc.ProviderID, months)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ConversionSummary
			if err := rows.Scan(&r.ProviderID, &r.SignupMonth, &r.NewCustomers,
				&r.CustomersWithSubscription, &r.ActiveSubscriptions); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// GetChurnSummary returns customer churn analysis data.
func (s *Service) GetChurnSummary(ctx context.Context, tc tenant.Ctx, months int) ([]ChurnSummary, error) {
	if months <= 0 || months > 24 {
		months = 12
	}
	var out []ChurnSummary
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := tx.Query(ctx, `
			SELECT provider_id::text, churn_month::text, churned_subscriptions, retained_subscriptions
			FROM analytics_churn
			WHERE provider_id = $1
				AND churn_month >= DATE_TRUNC('month', now()) - INTERVAL '1 month' * $2
			ORDER BY churn_month DESC
		`, tc.ProviderID, months)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ChurnSummary
			if err := rows.Scan(&r.ProviderID, &r.ChurnMonth, &r.ChurnedSubscriptions,
				&r.RetainedSubscriptions); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// GetUsageBreakdown returns daily usage breakdown by metric code.
func (s *Service) GetUsageBreakdown(ctx context.Context, tc tenant.Ctx, days int) ([]UsageBreakdownSummary, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	var out []UsageBreakdownSummary
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := tx.Query(ctx, `
			SELECT provider_id::text, metric_code, day::text, event_count, total_quantity
			FROM analytics_usage_breakdown
			WHERE provider_id = $1
				AND day >= DATE_TRUNC('day', now()) - INTERVAL '1 day' * $2
			ORDER BY day DESC, metric_code
		`, tc.ProviderID, days)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r UsageBreakdownSummary
			if err := rows.Scan(&r.ProviderID, &r.MetricCode, &r.Day, &r.EventCount,
				&r.TotalQuantity); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// GetUsageAnomalies returns detected usage anomalies (> 3x 7-day average).
func (s *Service) GetUsageAnomalies(ctx context.Context, tc tenant.Ctx) ([]AnomalySummary, error) {
	var out []AnomalySummary
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := tx.Query(ctx, `
			SELECT provider_id::text, metric_code, day::text, event_count, avg_7d, is_anomaly
			FROM analytics_usage_anomalies
			WHERE provider_id = $1
				AND day >= DATE_TRUNC('day', now()) - INTERVAL '7 days'
			ORDER BY day DESC
		`, tc.ProviderID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r AnomalySummary
			if err := rows.Scan(&r.ProviderID, &r.MetricCode, &r.Day, &r.EventCount,
				&r.Avg7d, &r.IsAnomaly); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// GetProviderDashboard returns a combined dashboard summary for the provider.
type ProviderDashboard struct {
	Revenue   []RevenueSummary     `json:"revenue"`
	MAU       []MAUSummary         `json:"mau"`
	Conversion []ConversionSummary `json:"conversion"`
	Churn     []ChurnSummary       `json:"churn"`
	Anomalies []AnomalySummary     `json:"anomalies"`
	GeneratedAt string             `json:"generated_at"`
}

// GetProviderDashboard returns a combined analytics dashboard for the provider.
func (s *Service) GetProviderDashboard(ctx context.Context, tc tenant.Ctx) (*ProviderDashboard, error) {
	revenue, err := s.GetRevenueSummary(ctx, tc, 6)
	if err != nil {
		return nil, fmt.Errorf("revenue: %w", err)
	}
	mau, err := s.GetMAUSummary(ctx, tc, 6)
	if err != nil {
		return nil, fmt.Errorf("mau: %w", err)
	}
	conversion, err := s.GetConversionSummary(ctx, tc, 6)
	if err != nil {
		return nil, fmt.Errorf("conversion: %w", err)
	}
	churn, err := s.GetChurnSummary(ctx, tc, 6)
	if err != nil {
		return nil, fmt.Errorf("churn: %w", err)
	}
	anomalies, err := s.GetUsageAnomalies(ctx, tc)
	if err != nil {
		return nil, fmt.Errorf("anomalies: %w", err)
	}
	return &ProviderDashboard{
		Revenue:     ensureSlice(revenue),
		MAU:         ensureSlice(mau),
		Conversion:  ensureSlice(conversion),
		Churn:       ensureSlice(churn),
		Anomalies:   ensureSlice(anomalies),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// ensureSlice returns a non-nil slice (for JSON empty array instead of null).
func ensureSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
