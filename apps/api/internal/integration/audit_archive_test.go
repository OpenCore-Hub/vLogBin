package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/archive"
)

// fakeArchiver records published anchors in memory, implementing
// archive.AnchorRecord uploads without any object storage.
type fakeArchiver struct {
	mu    sync.Mutex
	keys  []string
	recs  []archive.AnchorRecord
	fail  bool // when set, every upload fails (object-store outage simulation)
	panic bool // when set, panics on first upload (crash-between-upload-and-mark)
}

func (f *fakeArchiver) PublishAnchor(ctx context.Context, rec archive.AnchorRecord) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return "", context.DeadlineExceeded
	}
	key := "audit/anchors/" + uuid.NewString() + ".json"
	f.keys = append(f.keys, key)
	f.recs = append(f.recs, rec)
	// Simulate the archiver's only crash window: the object is already in the
	// bucket (key recorded) but the DB mark has not run yet.
	if f.panic {
		f.panic = false
		panic("simulated crash between upload and mark")
	}
	return key, nil
}

func (f *fakeArchiver) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}

// TestAuditAnchorArchivePublishesAndIsIdempotent drives the archive worker's
// core protocol: unpublished anchors are uploaded (outside any DB tx) and then
// marked published. A second sweep must skip them via the published_at IS NULL
// list filter and must not re-upload anything; the reported counters must
// match.
func TestAuditAnchorArchivePublishesAndIsIdempotent(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ar1-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	now := time.Now().UTC()
	injectChainEvent(t, pid, now, "qa.archive.one")
	injectChainEvent(t, pid, now.Add(time.Second), "qa.archive.two")

	// Create two anchors via the operator API (the same path the sweeper uses).
	if _, err := svc.AnchorAuditChain(testCtx, "test-archiver"); err != nil {
		t.Fatalf("AnchorAuditChain #1: %v", err)
	}
	if _, err := svc.AnchorAuditChain(testCtx, "test-archiver"); err != nil {
		t.Fatalf("AnchorAuditChain #2: %v", err)
	}

	arch := &fakeArchiver{}
	var published, already, listErr, uploadErr, markErr int64
	svc.SetAuditArchiver(arch)
	svc.SetAuditArchiveReporter(func(p, a, l, u, m int64) {
		published, already, listErr, uploadErr, markErr = p, a, l, u, m
	})
	defer func() {
		svc.SetAuditArchiver(nil)
		svc.SetAuditArchiveReporter(nil)
	}()

	// First sweep publishes both anchors.
	n, err := svc.ArchiveAuditAnchors(testCtx, 100)
	if err != nil {
		t.Fatalf("ArchiveAuditAnchors #1: %v", err)
	}
	if n != 2 {
		t.Fatalf("published %d anchors, want 2", n)
	}
	if got := arch.count(); got != 2 {
		t.Fatalf("uploaded %d objects, want 2", got)
	}
	if published != 2 || already != 0 || listErr != 0 || uploadErr != 0 || markErr != 0 {
		t.Fatalf("reporter counters after #1 = (%d,%d,%d,%d,%d), want (2,0,0,0,0)",
			published, already, listErr, uploadErr, markErr)
	}

	// Second sweep: nothing new to publish. The list query filters on
	// published_at IS NULL, so the empty batch returns before any upload/mark,
	// and all counters (including already_published) stay 0. already_published
	// only counts the race path where an upload succeeded but the mark hit 0
	// rows — it is NOT incremented for anchors skipped by the list query.
	// Idempotency here is proven by the object count staying at 2.
	published, already = 0, 0
	n, err = svc.ArchiveAuditAnchors(testCtx, 100)
	if err != nil {
		t.Fatalf("ArchiveAuditAnchors #2: %v", err)
	}
	if n != 0 {
		t.Fatalf("published %d anchors on re-sweep, want 0", n)
	}
	if got := arch.count(); got != 2 {
		t.Fatalf("uploaded %d objects after re-sweep, want still 2 (no duplicates)", got)
	}
	if published != 0 || already != 0 || listErr != 0 || uploadErr != 0 || markErr != 0 {
		t.Fatalf("reporter counters after #2 = (%d,%d,%d,%d,%d), want (0,0,0,0,0)",
			published, already, listErr, uploadErr, markErr)
	}
}

// TestAuditAnchorArchiveResumesAfterUploadCrash simulates a crash between the
// object upload and the DB mark (the archiver's only crash window). The next
// sweep must re-upload the same anchor (new key) and complete the mark without
// losing or duplicating published state.
func TestAuditAnchorArchiveResumesAfterUploadCrash(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ar2-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	injectChainEvent(t, pid, time.Now().UTC(), "qa.archive.crash")
	if _, err := svc.AnchorAuditChain(testCtx, "test-archiver"); err != nil {
		t.Fatalf("AnchorAuditChain: %v", err)
	}

	arch := &fakeArchiver{panic: true}
	var published, already int64
	svc.SetAuditArchiver(arch)
	svc.SetAuditArchiveReporter(func(p, a, l, u, m int64) { published, already = p, a })
	defer func() {
		svc.SetAuditArchiver(nil)
		svc.SetAuditArchiveReporter(nil)
	}()

	// The first sweep uploads the object then "crashes" before the mark; the
	// panic surfaces here. published stays 0 because the mark never ran.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected crash between upload and mark, none happened")
			}
		}()
		_, _ = svc.ArchiveAuditAnchors(testCtx, 100)
	}()
	if published != 0 {
		t.Fatalf("published = %d after crash, want 0 (mark never ran)", published)
	}
	if got := arch.count(); got != 1 {
		t.Fatalf("uploaded %d objects before crash, want 1", got)
	}

	// Next sweep: the anchor is still unpublished (published_at IS NULL), so it
	// is re-uploaded and finally marked. Idempotency is keyed on published_at,
	// not on the object key, so the new key is fine.
	arch.panic = false
	n, err := svc.ArchiveAuditAnchors(testCtx, 100)
	if err != nil {
		t.Fatalf("ArchiveAuditAnchors after crash: %v", err)
	}
	if n != 1 {
		t.Fatalf("published %d anchors after resume, want 1", n)
	}
	if published != 1 {
		t.Fatalf("published counter = %d after resume, want 1", published)
	}
	if got := arch.count(); got != 2 {
		t.Fatalf("uploaded %d objects total (1 crash + 1 resume), want 2", got)
	}
	if already != 0 {
		t.Fatalf("already_published = %d after resume, want 0", already)
	}
}

// TestAuditAnchorArchiveSurvivesUploadOutage verifies that when the object
// store is unavailable, anchors stay unpublished, the upload error counter
// increments, and a later sweep completes them.
func TestAuditAnchorArchiveSurvivesUploadOutage(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ar3-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	injectChainEvent(t, pid, time.Now().UTC(), "qa.archive.outage")
	if _, err := svc.AnchorAuditChain(testCtx, "test-archiver"); err != nil {
		t.Fatalf("AnchorAuditChain: %v", err)
	}

	arch := &fakeArchiver{fail: true}
	var published, uploadErr int64
	svc.SetAuditArchiver(arch)
	svc.SetAuditArchiveReporter(func(p, a, l, u, m int64) { published, uploadErr = p, u })
	defer func() {
		svc.SetAuditArchiver(nil)
		svc.SetAuditArchiveReporter(nil)
	}()

	n, err := svc.ArchiveAuditAnchors(testCtx, 100)
	if err != nil {
		t.Fatalf("ArchiveAuditAnchors during outage: %v", err)
	}
	if n != 0 {
		t.Fatalf("published %d anchors during outage, want 0", n)
	}
	if uploadErr != 1 {
		t.Fatalf("upload_errors = %d, want 1", uploadErr)
	}
	if got := arch.count(); got != 0 {
		t.Fatalf("uploaded %d objects during outage, want 0", got)
	}

	// Store recovers: the same sweep now succeeds.
	arch.fail = false
	n, err = svc.ArchiveAuditAnchors(testCtx, 100)
	if err != nil {
		t.Fatalf("ArchiveAuditAnchors after recovery: %v", err)
	}
	if n != 1 {
		t.Fatalf("published %d anchors after recovery, want 1", n)
	}
	if published != 1 {
		t.Fatalf("published counter = %d after recovery, want 1", published)
	}
}
