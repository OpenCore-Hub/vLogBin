package service

import (
	"context"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StreamEventsResult is the response for the Enterprise Event Stream API.
// The consumer passes next_cursor as the cursor parameter on the next
// request to resume consumption. If has_more is false, there are no more
// events at this time (new events may arrive later).
type StreamEventsResult struct {
	Events     []storegen.OutboxEvent `json:"events"`
	NextCursor *uuid.UUID             `json:"next_cursor"`
	HasMore    bool                   `json:"has_more"`
}

// StreamEvents returns a page of outbox events for the caller's tenant,
// ordered by (created_at, id) ascending. The cursor is the last event ID
// the consumer processed; pass uuid.Nil to start from the beginning.
// Optional filters by event_type and aggregate_type allow consumers to
// subscribe to specific event categories.
func (s *Service) StreamEvents(ctx context.Context, tc tenant.Ctx, cursor uuid.UUID, eventType, aggregateType string, limit int32) (*StreamEventsResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var result StreamEventsResult
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Fetch limit+1 to determine has_more without a separate count query.
		events, err := q.StreamOutboxEvents(ctx, storegen.StreamOutboxEventsParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Column3:       cursor,
			Column4:       eventType,
			Column5:       aggregateType,
			Limit:         limit + 1,
		})
		if err != nil {
			return err
		}

		result.HasMore = int32(len(events)) > limit
		if result.HasMore {
			events = events[:limit]
		}
		result.Events = events
		if len(events) > 0 {
			lastID := events[len(events)-1].ID
			result.NextCursor = &lastID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
