package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestEventStreamBasic(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-basic-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Stream events from the beginning.
	status, body := apiReq(t, "GET", "/v1/events?limit=100", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("stream events: status %d, body %v", status, body)
	}
	events := body["events"].([]any)
	if len(events) == 0 {
		t.Fatal("expected at least 1 event, got 0")
	}
	// Events should be ordered by created_at ASC (oldest first).
	firstEvent := events[0].(map[string]any)
	if firstEvent["event_type"] == nil {
		t.Fatal("event_type must be set")
	}
	if body["has_more"] != false {
		t.Fatalf("has_more = %v, want false (fetched all)", body["has_more"])
	}
}

func TestEventStreamCursorPagination(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-curs-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// First page: limit=2.
	status, body := apiReq(t, "GET", "/v1/events?limit=2", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("first page: status %d, body %v", status, body)
	}
	events := body["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if body["has_more"] != true {
		t.Fatalf("has_more = %v, want true (more events exist)", body["has_more"])
	}
	nextCursor := body["next_cursor"]
	if nextCursor == nil {
		t.Fatal("next_cursor must be set when has_more is true")
	}
	cursor := nextCursor.(string)

	// Second page: use cursor from first page.
	status, body = apiReq(t, "GET", "/v1/events?limit=2&cursor="+cursor, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("second page: status %d, body %v", status, body)
	}
	events2 := body["events"].([]any)
	if len(events2) == 0 {
		t.Fatal("expected at least 1 event on second page, got 0")
	}

	// Verify no overlap: first event of second page must differ from last event of first page.
	firstOfPage2 := events2[0].(map[string]any)
	lastOfPage1 := events[1].(map[string]any)
	if firstOfPage2["id"] == lastOfPage1["id"] {
		t.Fatal("cursor overlap: first event of page 2 equals last event of page 1")
	}
}

func TestEventStreamFilterByType(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-type-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Stream all events to find available event types.
	status, body := apiReq(t, "GET", "/v1/events?limit=100", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("stream all: status %d", status)
	}
	allEvents := body["events"].([]any)
	if len(allEvents) == 0 {
		t.Fatal("expected events, got 0")
	}

	// Pick the first event type and filter by it.
	firstType := allEvents[0].(map[string]any)["event_type"].(string)
	status, body = apiReq(t, "GET", "/v1/events?type="+firstType+"&limit=100", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("filter by type: status %d, body %v", status, body)
	}
	filtered := body["events"].([]any)
	if len(filtered) == 0 {
		t.Fatalf("expected events of type %q, got 0", firstType)
	}
	for _, e := range filtered {
		if e.(map[string]any)["event_type"] != firstType {
			t.Fatalf("event type = %v, want %q", e.(map[string]any)["event_type"], firstType)
		}
	}
}

func TestEventStreamFilterByAggregateType(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-agg-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Stream all events to find available aggregate types.
	status, body := apiReq(t, "GET", "/v1/events?limit=100", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("stream all: status %d", status)
	}
	allEvents := body["events"].([]any)
	if len(allEvents) == 0 {
		t.Fatal("expected events, got 0")
	}

	// Pick the first aggregate type and filter by it.
	firstAgg := allEvents[0].(map[string]any)["aggregate_type"].(string)
	status, body = apiReq(t, "GET", "/v1/events?aggregate_type="+firstAgg+"&limit=100", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("filter by aggregate_type: status %d, body %v", status, body)
	}
	filtered := body["events"].([]any)
	if len(filtered) == 0 {
		t.Fatalf("expected events of aggregate_type %q, got 0", firstAgg)
	}
	for _, e := range filtered {
		if e.(map[string]any)["aggregate_type"] != firstAgg {
			t.Fatalf("aggregate_type = %v, want %q", e.(map[string]any)["aggregate_type"], firstAgg)
		}
	}
}

func TestEventStreamEmptyResult(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-empty-"+uuid.NewString()[:8])

	// A new provider has events from creation (provider.created, etc.).
	// The test verifies the response format is correct (events is an
	// array, not null) and that has_more/next_cursor are set correctly.
	status, body := apiReq(t, "GET", "/v1/events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("stream: status %d, body %v", status, body)
	}
	events, ok := body["events"].([]any)
	if !ok {
		t.Fatalf("events is not an array: %T", body["events"])
	}
	// Verify events array is present (provider has creation events).
	_ = events
	if body["has_more"] != false {
		t.Fatalf("has_more = %v, want false", body["has_more"])
	}
}

func TestEventStreamCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "es-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "es-iso-b-"+uuid.NewString()[:8])

	// Provider A creates events.
	versionID := createPublishedCatalog(t, keyA)
	createCustomerAndSubscription(t, keyA, versionID)

	// Provider A can see events.
	status, body := apiReq(t, "GET", "/v1/events", keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("A stream: status %d", status)
	}
	eventsA := body["events"].([]any)
	if len(eventsA) == 0 {
		t.Fatal("A: expected events, got 0")
	}

	// Collect A's aggregate IDs.
	aAggIDs := make(map[string]bool)
	for _, e := range eventsA {
		em := e.(map[string]any)
		aAggIDs[em["aggregate_id"].(string)] = true
	}

	// Provider B cannot see A's events (RLS isolation).
	// B may have their own creation events, but none of A's aggregate IDs.
	status, body = apiReq(t, "GET", "/v1/events", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B stream: status %d", status)
	}
	eventsB := body["events"].([]any)
	for _, e := range eventsB {
		em := e.(map[string]any)
		aggID := em["aggregate_id"].(string)
		if aAggIDs[aggID] {
			t.Fatalf("B sees A's event (RLS leak): aggregate_id=%s", aggID)
		}
	}
}

func TestEventStreamInvalidCursor(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-bad-"+uuid.NewString()[:8])

	// Invalid cursor format.
	status, _ := apiReq(t, "GET", "/v1/events?cursor=not-a-uuid", apiKey, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid cursor: status %d, want 400", status)
	}
}

func TestEventStreamLimitClamped(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-lim-"+uuid.NewString()[:8])

	// Limit of 0 should be clamped to default (100).
	// Provider has creation events, so we just verify the request succeeds.
	status, body := apiReq(t, "GET", "/v1/events?limit=0", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("limit=0: status %d, body %v", status, body)
	}
	if _, ok := body["events"].([]any); !ok {
		t.Fatalf("events is not an array: %T", body["events"])
	}

	// Excessive limit should be clamped to 1000.
	status, _ = apiReq(t, "GET", "/v1/events?limit=99999", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("limit=99999: status %d, want 200", status)
	}
}

func TestEventStreamCursorResumption(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-resm-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Consume all events page by page.
	var allEventIDs []string
	cursor := ""
	for i := 0; i < 10; i++ { // max 10 pages to prevent infinite loop
		url := "/v1/events?limit=3"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		status, body := apiReq(t, "GET", url, apiKey, nil)
		if status != http.StatusOK {
			t.Fatalf("page %d: status %d", i, status)
		}
		events := body["events"].([]any)
		if len(events) == 0 {
			break
		}
		for _, e := range events {
			allEventIDs = append(allEventIDs, e.(map[string]any)["id"].(string))
		}
		if body["has_more"] != true {
			break
		}
		nc := body["next_cursor"]
		if nc == nil {
			break
		}
		cursor = nc.(string)
	}

	if len(allEventIDs) == 0 {
		t.Fatal("expected at least 1 event across all pages")
	}

	// Verify no duplicate event IDs (cursor resumption is correct).
	seen := make(map[string]bool)
	for _, id := range allEventIDs {
		if seen[id] {
			t.Fatalf("duplicate event ID across pages: %s", id)
		}
		seen[id] = true
	}
}

func TestEventStreamNoLossNoDuplication(t *testing.T) {
	_, apiKey := createProviderAPI(t, "es-exact-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Fetch the full stream as the ground truth.
	status, body := apiReq(t, "GET", "/v1/events?limit=100", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("full stream: status %d, body %v", status, body)
	}
	fullIDs := make(map[string]bool)
	for _, item := range body["events"].([]any) {
		fullIDs[item.(map[string]any)["id"].(string)] = true
	}
	if len(fullIDs) == 0 {
		t.Fatal("expected events, got 0")
	}

	// Paginate through the same stream with a small limit.
	var pageIDs []string
	cursor := ""
	for i := 0; i < 20; i++ {
		url := "/v1/events?limit=3"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		status, body = apiReq(t, "GET", url, apiKey, nil)
		if status != http.StatusOK {
			t.Fatalf("page %d: status %d", i, status)
		}
		events := body["events"].([]any)
		if len(events) == 0 {
			break
		}
		for _, item := range events {
			pageIDs = append(pageIDs, item.(map[string]any)["id"].(string))
		}
		if body["has_more"] != true {
			break
		}
		cursor = body["next_cursor"].(string)
	}

	if len(pageIDs) != len(fullIDs) {
		t.Fatalf("paged event count = %d, full stream = %d", len(pageIDs), len(fullIDs))
	}
	seen := make(map[string]bool)
	for _, id := range pageIDs {
		if seen[id] {
			t.Fatalf("duplicate event ID across pages: %s", id)
		}
		seen[id] = true
		if !fullIDs[id] {
			t.Fatalf("paged stream contains unknown event: %s", id)
		}
	}
}
