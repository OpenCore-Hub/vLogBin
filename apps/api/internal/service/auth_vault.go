package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/jackc/pgx/v5"
)

var ErrAuthVaultNotFound = errors.New("auth vault not found")
var ErrAuthVaultEncryptionDisabled = errors.New("auth vault encryption disabled")

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
	if s.encryptor == nil {
		return AuthVault{}, ErrAuthVaultEncryptionDisabled
	}
	idBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return AuthVault{}, fmt.Errorf("generate auth vault id: %w", err)
	}
	id := hex.EncodeToString(idBytes)

	encryptedAccess, err := s.encryptor.Encrypt(input.AccessToken)
	if err != nil {
		return AuthVault{}, fmt.Errorf("encrypt access token: %w", err)
	}
	encryptedRefresh, err := s.encryptor.Encrypt(input.RefreshToken)
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
		vault = authVaultFromRow(row, input.AccessToken, input.RefreshToken)
		return nil
	})
	if err != nil {
		return AuthVault{}, fmt.Errorf("create auth vault: %w", err)
	}
	return vault, nil
}

func (s *Service) GetAuthVault(ctx context.Context, id string) (AuthVault, error) {
	if s.encryptor == nil {
		return AuthVault{}, ErrAuthVaultEncryptionDisabled
	}
	var row storegen.AuthSessionVault
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		row, err = q.GetAuthVault(ctx, id)
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthVault{}, ErrAuthVaultNotFound
		}
		return AuthVault{}, fmt.Errorf("get auth vault: %w", err)
	}

	accessToken, err := s.encryptor.Decrypt(row.AccessToken)
	if err != nil {
		return AuthVault{}, fmt.Errorf("decrypt access token: %w", err)
	}
	refreshToken := ""
	if row.RefreshToken != "" {
		refreshToken, err = s.encryptor.Decrypt(row.RefreshToken)
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
		return q.DeleteAuthVault(ctx, id)
	})
}

func (s *Service) DeleteExpiredAuthVaults(ctx context.Context) error {
	return s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.DeleteExpiredAuthVaults(ctx)
	})
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
