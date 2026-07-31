package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreatePSPCredentialInput holds the plaintext values for a new PSP
// credential. The API key and webhook secret are encrypted before
// storage; the plaintext API key is returned exactly once to the caller.
type CreatePSPCredentialInput struct {
	PSPType        string // stripe, aden, mollie, paypal, other
	Label          string
	APIKey         string
	WebhookSecret  string // optional
}

// PSPCredentialResult is returned after creation or rotation. The
// PlaintextAPIKey is shown exactly once; subsequent reads return
// metadata only.
type PSPCredentialResult struct {
	Credential      storegen.PspCredential `json:"credential"`
	PlaintextAPIKey string                 `json:"plaintext_api_key"`
}

// CreatePSPCredential encrypts the API key and webhook secret and stores
// them. Returns the plaintext API key exactly once.
func (s *Service) CreatePSPCredential(ctx context.Context, tc tenant.Ctx, in CreatePSPCredentialInput) (*PSPCredentialResult, error) {
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: PSP encryption not configured", ErrValidation)
	}
	if in.APIKey == "" {
		return nil, fmt.Errorf("%w: api_key is required", ErrValidation)
	}
	if !validPSPType(in.PSPType) {
		return nil, fmt.Errorf("%w: unknown psp_type %q", ErrValidation, in.PSPType)
	}

	encKey, err := s.encryptor.Encrypt(in.APIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt api key: %w", err)
	}
	var encSecret string
	if in.WebhookSecret != "" {
		encSecret, err = s.encryptor.Encrypt(in.WebhookSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypt webhook secret: %w", err)
		}
	}

	// Determine the next key version for this provider/env/psp_type.
	version := int32(1)
	var cred storegen.PspCredential
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		count, err := q.CountPSPCredentialsByType(ctx, storegen.CountPSPCredentialsByTypeParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, PspType: in.PSPType,
		})
		if err != nil {
			return mapErr(err, "count psp credentials")
		}
		version = int32(count) + 1

		c, err := q.CreatePSPCredential(ctx, storegen.CreatePSPCredentialParams{
			ProviderID:             tc.ProviderID,
			EnvironmentID:          tc.EnvironmentID,
			PspType:                in.PSPType,
			Label:                  in.Label,
			EncryptedApiKey:        encKey,
			EncryptedWebhookSecret: toText(encSecret),
			KeyVersion:             version,
		})
		if err != nil {
			return mapErr(err, "create psp credential")
		}
		cred = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &PSPCredentialResult{Credential: cred, PlaintextAPIKey: in.APIKey}, nil
}

// RotatePSPCredential revokes the existing credential and creates a new
// version with the new API key. Returns the new plaintext API key once.
func (s *Service) RotatePSPCredential(ctx context.Context, tc tenant.Ctx, id uuid.UUID, newAPIKey, newWebhookSecret string) (*PSPCredentialResult, error) {
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: PSP encryption not configured", ErrValidation)
	}
	if newAPIKey == "" {
		return nil, fmt.Errorf("%w: new api_key is required", ErrValidation)
	}

	encKey, err := s.encryptor.Encrypt(newAPIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt api key: %w", err)
	}
	var encSecret string
	if newWebhookSecret != "" {
		encSecret, err = s.encryptor.Encrypt(newWebhookSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypt webhook secret: %w", err)
		}
	}

	var cred storegen.PspCredential
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Get the existing credential to determine type and version.
		existing, err := q.GetPSPCredentialByID(ctx, storegen.GetPSPCredentialByIDParams{
			ID: id, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "psp credential %s", id)
		}

		// Revoke the old credential.
		if err := q.RevokePSPCredential(ctx, storegen.RevokePSPCredentialParams{
			ID: id, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		}); err != nil {
			return mapErr(err, "revoke psp credential %s", id)
		}

		// Create new version.
		c, err := q.CreatePSPCredential(ctx, storegen.CreatePSPCredentialParams{
			ProviderID:             tc.ProviderID,
			EnvironmentID:          tc.EnvironmentID,
			PspType:                existing.PspType,
			Label:                  existing.Label,
			EncryptedApiKey:        encKey,
			EncryptedWebhookSecret: toText(encSecret),
			KeyVersion:             existing.KeyVersion + 1,
		})
		if err != nil {
			return mapErr(err, "create rotated psp credential")
		}
		cred = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &PSPCredentialResult{Credential: cred, PlaintextAPIKey: newAPIKey}, nil
}

// ListPSPCredentials returns all PSP credentials for the tenant. The
// encrypted fields are NOT decrypted — only metadata is returned.
func (s *Service) ListPSPCredentials(ctx context.Context, tc tenant.Ctx) ([]storegen.PspCredential, error) {
	var creds []storegen.PspCredential
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.ListPSPCredentials(ctx, storegen.ListPSPCredentialsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "psp credentials")
		}
		creds = c
		return nil
	})
	return creds, err
}

// RevokePSPCredential deactivates a PSP credential.
func (s *Service) RevokePSPCredential(ctx context.Context, tc tenant.Ctx, id uuid.UUID) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if err := q.RevokePSPCredential(ctx, storegen.RevokePSPCredentialParams{
			ID: id, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		}); err != nil {
			return mapErr(err, "revoke psp credential %s", id)
		}
		return nil
	})
}

// validPSPType checks whether s is a supported PSP type.
func validPSPType(s string) bool {
	switch s {
	case "stripe", "adyen", "mollie", "paypal", "other":
		return true
	}
	return false
}

// toText converts a string to pgtype.Text (nullable).
func toText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// WithCryptoEncryptor injects the encryptor used for PSP credential
// encryption. When nil (no PSP_MASTER_KEY configured), PSP credential
// creation returns an error.
func WithCryptoEncryptor(e *crypto.Encryptor) Option {
	return func(s *Service) { s.encryptor = e }
}

// GenerateMasterKey generates a random 32-byte hex string suitable for
// use as PSP_MASTER_KEY.
func GenerateMasterKey() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
