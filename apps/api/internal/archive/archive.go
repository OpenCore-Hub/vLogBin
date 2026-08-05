// Package archive publishes audit hash chain anchors to WORM object storage
// (S3-compatible: MinIO or AWS S3 with object lock enabled).
//
// Design: the archiver is the *external anchoring* half of the tamper-proof
// audit chain (0031). Each anchor row in audit_chain_anchors checkpoints the
// chain tail; this package uploads an immutable copy of the checkpoint to a
// bucket outside the database. A DB superuser could rewrite the chain and the
// anchors table, but the archived copies are beyond its reach — any rewrite
// then diverges from the WORM copies and is detectable by re-checking the
// chain against the bucket.
//
// The bucket must be provisioned with object lock / retention enabled (the
// archiver never deletes or overwrites archived objects — object keys are
// deterministic so a re-publish writes identical content under the same key,
// which object lock permits as a content-identical overwrite only when
// versioning is configured; with retention in COMPLIANCE mode such re-puts
// fail, and the caller treats "already exists" as success via the DB-side
// published_at guard). Credentials are held only by the archiver process,
// never by the database role.
package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// AnchorRecord is the immutable payload archived for one anchor. It mirrors
// the audit_chain_anchors row plus a content SHA-256 so a verifier can both
// render the record and validate its own copy against a known digest.
type AnchorRecord struct {
	AnchorID    int64     `json:"anchor_id"`
	TailEventID int64     `json:"tail_event_id"`
	TailHash    string    `json:"tail_hash"`
	Operator    string    `json:"operator"`
	CreatedAt   time.Time `json:"created_at"`
	// ContentSHA256 is the hex SHA-256 of the canonical JSON payload (all
	// fields above). It lets an offline verifier confirm the object it
	// fetched matches the digest it computed, without trusting transport.
	ContentSHA256 string `json:"content_sha256"`
}

// Archiver uploads AnchorRecords to a single WORM bucket. It is safe for
// sequential use by the single archive sweeper worker.
type Archiver struct {
	client *minio.Client
	bucket string
	log    *slog.Logger
}

// NewArchiver builds a client for the configured S3-compatible endpoint.
// It deliberately does NOT create the bucket: WORM buckets must be
// provisioned out-of-band with object lock / retention, and auto-creating
// here would silently produce a non-WORM bucket.
func NewArchiver(endpoint, bucket, accessKey, secretKey, region string, useSSL bool, log *slog.Logger) (*Archiver, error) {
	if log == nil {
		log = slog.Default()
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("archive: build S3 client: %w", err)
	}
	return &Archiver{client: client, bucket: bucket, log: log}, nil
}

// objectKey returns the deterministic object key for an anchor. The anchor id
// is globally unique and monotonic, so keys never collide and a re-publish
// writes the same content to the same key (idempotent).
func objectKey(anchorID int64) string {
	return fmt.Sprintf("audit/anchors/%d.json", anchorID)
}

// PublishAnchor serializes rec to canonical JSON, computes its content
// SHA-256 over the non-self-referential payload, and uploads it under the
// deterministic key audit/anchors/{anchor_id}.json. It returns the object key
// on success. ContentSHA256 is filled in by this method: the digest is
// computed over all fields *except* the digest itself, so a verifier can
// reproduce it deterministically.
func (a *Archiver) PublishAnchor(ctx context.Context, rec AnchorRecord) (string, error) {
	digestInput := struct {
		AnchorID    int64     `json:"anchor_id"`
		TailEventID int64     `json:"tail_event_id"`
		TailHash    string    `json:"tail_hash"`
		Operator    string    `json:"operator"`
		CreatedAt   time.Time `json:"created_at"`
	}{rec.AnchorID, rec.TailEventID, rec.TailHash, rec.Operator, rec.CreatedAt}
	digestBytes, err := json.Marshal(digestInput)
	if err != nil {
		return "", fmt.Errorf("archive: marshal anchor %d digest: %w", rec.AnchorID, err)
	}
	sum := sha256.Sum256(digestBytes)
	rec.ContentSHA256 = hex.EncodeToString(sum[:])

	payload, err := json.Marshal(rec)
	if err != nil {
		return "", fmt.Errorf("archive: marshal anchor %d: %w", rec.AnchorID, err)
	}
	key := objectKey(rec.AnchorID)
	opts := minio.PutObjectOptions{ContentType: "application/json"}
	if _, err := a.client.PutObject(ctx, a.bucket, key, bytes.NewReader(payload), int64(len(payload)), opts); err != nil {
		return "", fmt.Errorf("archive: put object %s: %w", key, err)
	}
	a.log.Info("audit anchor archived",
		"object_key", key,
		"anchor_id", rec.AnchorID,
		"tail_event_id", rec.TailEventID,
		"content_sha256", rec.ContentSHA256,
		"bucket", a.bucket,
	)
	return key, nil
}
