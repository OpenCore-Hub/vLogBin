package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrAuthVaultNotFound = errors.New("auth vault not found")
var ErrAuthVaultEncryptionDisabled = errors.New("auth vault encryption disabled")

func WithAuthVaultEncryptor(e *crypto.Encryptor) Option {
	return func(s *Service) { s.authVaultEncryptor = e }
}

type AuthVault struct {
	ID           string
	UserSub      string
	Email        string
	Name         string
	Roles        []string
	WorkspaceID  string
	Env          string
	AccessToken  string
	RefreshToken string
	TokenExp     int64
	ExpiresAt    time.Time
}

type CreateAuthVaultInput struct {
	UserSub      string
	Email        string
	Name         string
	Roles        []string
	WorkspaceID  string
	Env          string
	AccessToken  string
	RefreshToken string
	TokenExp     int64
	TTL          time.Duration
}

func (s *Service) CreateAuthVault(
	ctx context.Context,
	input CreateAuthVaultInput,
) (AuthVault, error) {
	if s.authVaultEncryptor == nil {
		return AuthVault{}, ErrAuthVaultEncryptionDisabled
	}
	idBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return AuthVault{}, fmt.Errorf("generate auth vault id: %w", err)
	}
	id := hex.EncodeToString(idBytes)

	encryptedAccess, err := s.authVaultEncryptor.Encrypt(input.AccessToken)
	if err != nil {
		return AuthVault{}, fmt.Errorf("encrypt access token: %w", err)
	}
	encryptedRefresh, err := s.authVaultEncryptor.Encrypt(input.RefreshToken)
	if err != nil {
		return AuthVault{}, fmt.Errorf("encrypt refresh token: %w", err)
	}
	roles, err := json.Marshal(input.Roles)
	if err != nil {
		return AuthVault{}, fmt.Errorf("encode roles: %w", err)
	}
	expiresAt := time.Now().Add(input.TTL)

	var vault AuthVault
	err = s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		row, err := q.CreateAuthVault(ctx, storegen.CreateAuthVaultParams{
			ID:           id,
			UserSub:      input.UserSub,
			Email:        input.Email,
			Name:         input.Name,
			Roles:        roles,
			WorkspaceID:  input.WorkspaceID,
			Env:          input.Env,
			AccessToken:  encryptedAccess,
			RefreshToken: encryptedRefresh,
			TokenExp:     input.TokenExp,
			ExpiresAt:    expiresAt,
		})
		if err != nil {
			return err
		}
		if err := insertAuthVaultAudit(ctx, q, "auth_vault.create", id, map[string]any{
			"userSub": input.UserSub,
			"env":     input.Env,
		}); err != nil {
			return err
		}
		vault = authVaultFromRow(row, input.AccessToken, input.RefreshToken)
		return nil
	})
	if err != nil {
		return AuthVault{}, fmt.Errorf("create auth vault: %w", err)
	}
	return vault, nil
}

func (s *Service) GetAuthVault(ctx context.Context, id string) (AuthVault, error) {
	if s.authVaultEncryptor == nil {
		return AuthVault{}, ErrAuthVaultEncryptionDisabled
	}
	var row storegen.AuthSessionVault
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		row, err = q.GetAuthVault(ctx, id)
		if err != nil {
			return err
		}
		return insertAuthVaultAudit(ctx, q, "auth_vault.get", id, map[string]any{
			"userSub": row.UserSub,
			"env":     row.Env,
		})
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthVault{}, ErrAuthVaultNotFound
		}
		return AuthVault{}, fmt.Errorf("get auth vault: %w", err)
	}

	accessToken, err := s.authVaultEncryptor.Decrypt(row.AccessToken)
	if err != nil {
		return AuthVault{}, fmt.Errorf("decrypt access token: %w", err)
	}
	refreshToken := ""
	if row.RefreshToken != "" {
		refreshToken, err = s.authVaultEncryptor.Decrypt(row.RefreshToken)
		if err != nil {
			return AuthVault{}, fmt.Errorf("decrypt refresh token: %w", err)
		}
	}
	var roles []string
	if err := json.Unmarshal(row.Roles, &roles); err != nil {
		return AuthVault{}, fmt.Errorf("decode roles: %w", err)
	}
	return authVaultFromRow(row, accessToken, refreshToken), nil
}

func (s *Service) DeleteAuthVault(ctx context.Context, id string) error {
	return s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if err := q.DeleteAuthVault(ctx, id); err != nil {
			return err
		}
		return insertAuthVaultAudit(ctx, q, "auth_vault.delete", id, nil)
	})
}

func (s *Service) DeleteExpiredAuthVaults(ctx context.Context) (int64, error) {
	var deleted int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		deleted, err = q.DeleteExpiredAuthVaults(ctx)
		return err
	})
	return deleted, err
}

func authVaultFromRow(
	row storegen.AuthSessionVault,
	accessToken, refreshToken string,
) AuthVault {
	var roles []string
	_ = json.Unmarshal(row.Roles, &roles)
	return AuthVault{
		ID:           row.ID,
		UserSub:      row.UserSub,
		Email:        row.Email,
		Name:         row.Name,
		Roles:        roles,
		WorkspaceID:  row.WorkspaceID,
		Env:          row.Env,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenExp:     row.TokenExp,
		ExpiresAt:    row.ExpiresAt,
	}
}

func insertAuthVaultAudit(
	ctx context.Context,
	q *store.Queries,
	action, id string,
	metadata map[string]any,
) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = q.InsertAuditEvent(ctx, storegen.InsertAuditEventParams{
		ProviderID:    uuid.NullUUID{},
		EnvironmentID: uuid.NullUUID{},
		ActorType:     "vault-service",
		ActorID:       "auth-vault-service",
		Action:        action,
		TargetType:    pgtype.Text{String: "auth_vault", Valid: true},
		TargetID:      pgtype.Text{String: id, Valid: true},
		Metadata:      raw,
	})
	return err
}

func NewAuthVaultSweeper(
	svc *Service,
	interval time.Duration,
	log *slog.Logger,
) *ExpirySweeper {
	return NewExpirySweeper("auth_vault", svc.DeleteExpiredAuthVaults, interval, log)
}
