package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// NotificationConfigInput is the decrypted configuration for a notification channel.
type NotificationConfigInput struct {
	Channel      string         `json:"channel"`
	ProviderType string         `json:"provider_type"`
	Config       map[string]any `json:"config"`
	FromAddress  string         `json:"from_address"`
	Enabled      bool           `json:"enabled"`
}

// NotificationConfigResult is the response for a notification config.
// The config field is decrypted for the provider's own use.
type NotificationConfigResult struct {
	storegen.NotificationConfig
	Config map[string]any `json:"config"`
}

// SetNotificationConfig creates or updates a notification channel configuration.
// The config (containing credentials like SMTP passwords) is encrypted with
// AES-256-GCM before storage (same as PSP credentials).
func (s *Service) SetNotificationConfig(ctx context.Context, tc tenant.Ctx, in NotificationConfigInput) (*NotificationConfigResult, error) {
	if !domain.ValidNotificationChannel(in.Channel) {
		return nil, fmt.Errorf("%w: channel must be email or sms", ErrValidation)
	}
	if in.ProviderType == "" {
		return nil, fmt.Errorf("%w: provider_type is required", ErrValidation)
	}
	if in.FromAddress == "" {
		return nil, fmt.Errorf("%w: from_address is required", ErrValidation)
	}
	if len(in.Config) == 0 {
		return nil, fmt.Errorf("%w: config must not be empty", ErrValidation)
	}

	plaintext, err := json.Marshal(in.Config)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid config: %v", ErrValidation, err)
	}
	encrypted, err := s.encryptor.Encrypt(string(plaintext))
	if err != nil {
		return nil, fmt.Errorf("encrypt notification config: %w", err)
	}

	var result NotificationConfigResult
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		nc, err := q.UpsertNotificationConfig(ctx, storegen.UpsertNotificationConfigParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Channel:       in.Channel,
			ProviderType:  in.ProviderType,
			ConfigEnc:     []byte(encrypted),
			FromAddress:   in.FromAddress,
			Enabled:       in.Enabled,
		})
		if err != nil {
			return mapErr(err, "notification config %q", in.Channel)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "notification_config", nc.ID.String(), "notification.config_set", map[string]any{
			"channel": in.Channel, "provider_type": in.ProviderType,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "notification.config_set",
			"notification_config", nc.ID.String(),
			map[string]any{"channel": in.Channel, "provider_type": in.ProviderType}); err != nil {
			return err
		}
		result.NotificationConfig = nc
		result.Config = in.Config
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetNotificationConfig returns a notification channel configuration.
// The config is decrypted before returning.
func (s *Service) GetNotificationConfig(ctx context.Context, tc tenant.Ctx, channel string) (*NotificationConfigResult, error) {
	var result NotificationConfigResult
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		nc, err := q.GetNotificationConfig(ctx, storegen.GetNotificationConfigParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Channel: channel,
		})
		if err != nil {
			return mapErr(err, "notification config %q", channel)
		}
		plaintext, err := s.encryptor.Decrypt(string(nc.ConfigEnc))
		if err != nil {
			return fmt.Errorf("decrypt notification config: %w", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(plaintext), &cfg); err != nil {
			return fmt.Errorf("invalid stored config: %w", err)
		}
		result.NotificationConfig = nc
		result.Config = cfg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListNotificationConfigs returns all notification channel configurations.
func (s *Service) ListNotificationConfigs(ctx context.Context, tc tenant.Ctx) ([]NotificationConfigResult, error) {
	var out []NotificationConfigResult
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		configs, err := q.ListNotificationConfigs(ctx, storegen.ListNotificationConfigsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return err
		}
		for _, nc := range configs {
			plaintext, err := s.encryptor.Decrypt(string(nc.ConfigEnc))
			if err != nil {
				slog.Error("failed to decrypt notification config", "config_id", nc.ID, "error", err)
				continue
			}
			var cfg map[string]any
			if err := json.Unmarshal([]byte(plaintext), &cfg); err != nil {
				slog.Error("invalid stored notification config", "config_id", nc.ID, "error", err)
				continue
			}
			out = append(out, NotificationConfigResult{NotificationConfig: nc, Config: cfg})
		}
		return nil
	})
	return out, err
}

// DeleteNotificationConfig removes a notification channel configuration.
func (s *Service) DeleteNotificationConfig(ctx context.Context, tc tenant.Ctx, channel string) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.DeleteNotificationConfig(ctx, storegen.DeleteNotificationConfigParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Channel: channel,
		})
		if err != nil {
			return mapErr(err, "notification config %q", channel)
		}
		if rows == 0 {
			return fmt.Errorf("%w: notification config %q", ErrNotFound, channel)
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "notification.config_delete",
			"notification_config", strings.TrimSpace(channel),
			map[string]any{"channel": channel})
	})
}
