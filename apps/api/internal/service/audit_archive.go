package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/archive"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/jackc/pgx/v5"
)

// AnchorArchiver publishes one audit anchor's immutable payload to WORM
// object storage and returns the object key. Implemented by archive.Archiver;
// nil disables the audit archiver worker.
type AnchorArchiver interface {
	PublishAnchor(ctx context.Context, rec archive.AnchorRecord) (string, error)
}

// SetAuditArchiver wires the WORM object-storage client into the service.
// Must be called before the audit archiver worker starts; without it the
// worker is a no-op (enabling via config without storage config is rejected
// at Load time, so this nil state only occurs if main forgets the wiring).
func (s *Service) SetAuditArchiver(a AnchorArchiver) {
	s.auditArchiver = a
}

// SetAuditArchiveReporter wires the per-sweep metrics callback. Counters are
// cumulative over the sweep: (published, alreadyPublished, listErrors,
// uploadErrors, markErrors). The error split lets operators distinguish
// object-store problems (upload) from DB write problems (mark) and from a
// failing list step that stalls the whole sweep.
func (s *Service) SetAuditArchiveReporter(fn func(published, alreadyPublished, listErrors, uploadErrors, markErrors int64)) {
	s.auditArchiveReporter = fn
}

// ArchiveAuditAnchors publishes up to batchSize unpublished anchors to WORM
// object storage and marks them published. It is the per-sweep body of the
// audit archiver worker.
//
// Protocol (retry-safe, idempotent):
//  1. List anchors with published_at IS NULL, oldest first (short read-only
//     transaction).
//  2. For each anchor, upload the object OUTSIDE any DB transaction (network
//     I/O must not hold a transaction open; the object key is deterministic,
//     so a crash before step 3 simply re-uploads identical content).
//  3. Mark the row published in a short transaction; the
//     WHERE published_at IS NULL guard makes the update idempotent under
//     concurrent workers (zero affected rows => already published).
//
// A crash after upload but before mark leaves the anchor unpublished, so the
// next sweep re-uploads the same key and completes the mark — no half state,
// no duplicates. Counters reported to the reporter: published (new marks),
// alreadyPublished (skipped), plus error splits per operation.
func (s *Service) ArchiveAuditAnchors(ctx context.Context, batchSize int) (int64, error) {
	if s.auditArchiver == nil {
		return 0, nil
	}
	var published, already, listErrors, uploadErrors, markErrors int64
	defer func() {
		if s.auditArchiveReporter != nil {
			s.auditArchiveReporter(published, already, listErrors, uploadErrors, markErrors)
		}
	}()

	// Step 1: fetch the next unpublished batch (short read-only tx).
	var anchors []storegen.AuditChainAnchor
	if err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.ListAuditAnchorsForPublish(ctx, int32(batchSize))
		if err != nil {
			return err
		}
		anchors = rows
		return nil
	}); err != nil {
		listErrors++
		return 0, fmt.Errorf("audit archive: list unpublished anchors: %w", err)
	}
	if len(anchors) == 0 {
		return 0, nil
	}

	// Steps 2–3: per anchor, upload outside any tx, then mark in a short tx.
	for _, a := range anchors {
		rec := archive.AnchorRecord{
			AnchorID:    a.ID,
			TailEventID: a.TailEventID,
			TailHash:    a.TailHash,
			Operator:    a.Operator,
			CreatedAt:   a.CreatedAt,
		}
		key, err := s.auditArchiver.PublishAnchor(ctx, rec)
		if err != nil {
			uploadErrors++
			s.log.Warn("audit anchor upload failed", "anchor_id", a.ID, "error", err)
			continue
		}
		if err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
			n, err := q.MarkAuditAnchorPublished(ctx, storegen.MarkAuditAnchorPublishedParams{
				ObjectKey: key,
				AnchorID:  a.ID,
			})
			if err != nil {
				return err
			}
			if n > 0 {
				published++
			} else {
				already++
			}
			return nil
		}); err != nil {
			markErrors++
			s.log.Warn("audit anchor mark failed", "anchor_id", a.ID, "object_key", key, "error", err)
			continue
		}
	}
	s.log.Info("audit archive sweep complete",
		"published", published,
		"already_published", already,
		"list_errors", listErrors,
		"upload_errors", uploadErrors,
		"mark_errors", markErrors,
	)
	return published, nil
}

// NewAuditArchiveSweeper creates the background worker that publishes audit
// hash chain anchors to WORM object storage at the given interval. The sweep
// is bounded by batchSize anchors per run, so a large backlog is drained
// across sweeps in short, resumable transactions. With a nil archiver (not
// wired), ArchiveAuditAnchors is a no-op, so main may unconditionally start
// the worker and let config decide whether storage is configured.
func NewAuditArchiveSweeper(svc *Service, batchSize int, interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("audit_archiver", func(ctx context.Context) (int64, error) {
		return svc.ArchiveAuditAnchors(ctx, batchSize)
	}, interval, log)
}
