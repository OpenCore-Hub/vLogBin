package archive

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestObjectKeyDeterministic(t *testing.T) {
	// The object key must be a pure function of the anchor id: re-publishing
	// the same anchor (e.g. after a crash between upload and mark) writes the
	// same key, which combined with the DB-side published_at guard makes the
	// whole publish protocol idempotent.
	if got, want := objectKey(42), "audit/anchors/42.json"; got != want {
		t.Fatalf("objectKey(42) = %q, want %q", got, want)
	}
	if a, b := objectKey(1), objectKey(1); a != b {
		t.Fatalf("objectKey not deterministic: %q vs %q", a, b)
	}
}

func TestAnchorRecordJSONShape(t *testing.T) {
	// The archived object is the tamper-evidence artifact: its fields must be
	// exactly what a verifier re-reads from the DB chain (tail_event_id,
	// tail_hash, operator, created_at), plus a self-describing content digest.
	rec := AnchorRecord{
		AnchorID:    7,
		TailEventID: 12345,
		TailHash:    "abc123",
		Operator:    "sweeper",
		CreatedAt:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	for _, want := range []string{`"anchor_id":7`, `"tail_event_id":12345`, `"tail_hash":"abc123"`,
		`"operator":"sweeper"`, `"created_at":`, `"content_sha256":""`} {
		if !strings.Contains(s, want) {
			t.Fatalf("anchor JSON missing %q: %s", want, s)
		}
	}
}

func TestNewArchiverRejectsBadEndpoint(t *testing.T) {
	// minio.New fails fast on malformed endpoints; the archiver surfaces that
	// as a config error instead of panicking at the first sweep.
	a, err := NewArchiver("://bad-endpoint", "bucket", "ak", "sk", "", true, nil)
	if err == nil {
		t.Fatal("NewArchiver with malformed endpoint must fail")
	}
	if a != nil {
		t.Fatalf("NewArchiver returned non-nil client on error: %+v", a)
	}
}
