package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
)

// NewReencryptionSweeper creates the credential re-encryption worker. The
// caller only starts it after a master-key rotation, while previous keys are
// configured (REENCRYPT_SWEEP_INTERVAL > 0 with PSP_MASTER_KEY_PREVIOUS set);
// without previous keys there is nothing to converge.
func NewReencryptionSweeper(svc *Service, interval time.Duration, batchSize int, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("credential_reencryption", func(ctx context.Context) (int64, error) {
		return svc.ReencryptLegacyCiphertexts(ctx, batchSize)
	}, interval, log)
}

// reencryptBatchSize is the default number of rows enumerated per table per
// transaction by the re-encryption worker. Kept small so a long backlog never
// holds one giant operator-scoped transaction open.
const reencryptBatchSize = 100

// ReencryptLegacyCiphertexts sweeps every encrypted row across
// psp_credentials, notification_configs and provider_auth_configs in the
// operator context and re-seals ciphertext written under a previous
// (rotated-out) master key with the active key — completing the master-key
// rotation loop: once the reencrypted count converges and the fallback read
// metric goes to zero, the old key can be dropped from PSP_MASTER_KEY_PREVIOUS.
//
// Rows that no configured key can open are skipped and reported through the
// re-encrypt reporter as errors: they cannot be converged automatically and
// need operator attention. Each batch runs in its own short operator-scoped
// transaction. Returns the total number of rows re-encrypted this sweep.
func (s *Service) ReencryptLegacyCiphertexts(ctx context.Context, batchSize int) (int64, error) {
	if s.encryptor == nil {
		return 0, nil // encryption disabled — nothing to converge
	}
	if batchSize <= 0 {
		batchSize = reencryptBatchSize
	}

	var total int64
	sweeps := []struct {
		table string
		sweep func(ctx context.Context, q *store.Queries, limit int32) (reencrypted, errors int64, err error)
	}{
		{"psp_credentials", s.reencryptPSPCredentials},
		{"notification_configs", s.reencryptNotificationConfigs},
		{"provider_auth_configs", s.reencryptProviderAuthConfigs},
	}
	for _, sw := range sweeps {
		for {
			var reencrypted, errs int64
			err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
				var err error
				reencrypted, errs, err = sw.sweep(ctx, q, int32(batchSize))
				return err
			})
			if err != nil {
				return total, err
			}
			total += reencrypted
			if s.reencryptReporter != nil {
				s.reencryptReporter(sw.table, reencrypted, errs)
			}
			if reencrypted+errs < int64(batchSize) {
				break // table exhausted
			}
		}
	}
	return total, nil
}

// reencryptField returns ciphertext re-sealed with the active key when it was
// sealed under a previous key; otherwise the input is returned unchanged. An
// error is returned when no configured key can open it (corrupt ciphertext or
// keys lost outside rotation).
func (s *Service) reencryptField(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	needs, err := s.encryptor.NeedsReencryption(ciphertext)
	if err != nil {
		return "", err
	}
	if !needs {
		return ciphertext, nil
	}
	plaintext, err := s.encryptor.DecryptWithoutFallback(ciphertext)
	if err != nil {
		return "", err
	}
	return s.encryptor.Encrypt(plaintext)
}

func (s *Service) reencryptPSPCredentials(ctx context.Context, q *store.Queries, limit int32) (int64, int64, error) {
	rows, err := q.ListAllPSPCredentialCiphertexts(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	var reencrypted, errs int64
	for _, r := range rows {
		apiKey, err := s.reencryptField(r.EncryptedApiKey)
		if err != nil {
			errs++
			continue
		}
		webhook := ""
		if r.EncryptedWebhookSecret.Valid {
			webhook = r.EncryptedWebhookSecret.String
		}
		newWebhook, err := s.reencryptField(webhook)
		if err != nil {
			errs++
			continue
		}
		if apiKey == r.EncryptedApiKey && newWebhook == webhook {
			continue // nothing sealed under a previous key
		}
		if err := q.UpdatePSPCredentialCiphertexts(ctx, storegen.UpdatePSPCredentialCiphertextsParams{
			ID:              r.ID,
			EncryptedApiKey: apiKey,
			Column3:         newWebhook, // NULLIF maps "" back to NULL
		}); err != nil {
			return reencrypted, errs, err
		}
		reencrypted++
	}
	return reencrypted, errs, nil
}

func (s *Service) reencryptNotificationConfigs(ctx context.Context, q *store.Queries, limit int32) (int64, int64, error) {
	rows, err := q.ListAllNotificationConfigCiphertexts(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	var reencrypted, errs int64
	for _, r := range rows {
		if len(r.ConfigEnc) == 0 {
			continue
		}
		newEnc, err := s.reencryptField(string(r.ConfigEnc))
		if err != nil {
			errs++
			continue
		}
		if newEnc == string(r.ConfigEnc) {
			continue
		}
		if err := q.UpdateNotificationConfigCiphertext(ctx, storegen.UpdateNotificationConfigCiphertextParams{
			ID:        r.ID,
			ConfigEnc: []byte(newEnc),
		}); err != nil {
			return reencrypted, errs, err
		}
		reencrypted++
	}
	return reencrypted, errs, nil
}

func (s *Service) reencryptProviderAuthConfigs(ctx context.Context, q *store.Queries, limit int32) (int64, int64, error) {
	rows, err := q.ListAllProviderAuthCiphertexts(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	var reencrypted, errs int64
	for _, r := range rows {
		if r.ZitadelClientSecret == "" {
			continue
		}
		newSecret, err := s.reencryptField(r.ZitadelClientSecret)
		if err != nil {
			errs++
			continue
		}
		if newSecret == r.ZitadelClientSecret {
			continue
		}
		if err := q.UpdateProviderAuthClientSecret(ctx, storegen.UpdateProviderAuthClientSecretParams{
			ID:                  r.ID,
			ZitadelClientSecret: newSecret,
		}); err != nil {
			return reencrypted, errs, err
		}
		reencrypted++
	}
	return reencrypted, errs, nil
}
