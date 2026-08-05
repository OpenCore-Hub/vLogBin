package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/jackc/pgx/v5"
)

// SetupHostedAuthInput configures ZITADEL Hosted Auth for a provider.
type SetupHostedAuthInput struct {
	Name         string   // ZITADEL project + app name
	RedirectURIs []string // OIDC redirect URIs
}

// AuthConfig is the Hosted Auth configuration returned to the provider.
type AuthConfig struct {
	Config    storegen.ProviderAuthConfig `json:"config"`
	ClientID  string                      `json:"client_id"`  // plaintext, shown once
	IssuerURL string                      `json:"issuer_url"` // ZITADEL issuer for OIDC discovery
}

// SetupHostedAuth creates a ZITADEL project + OIDC application for the
// provider and stores the configuration. The client secret is encrypted
// at rest; the plaintext client_id is returned to the caller.
func (s *Service) SetupHostedAuth(ctx context.Context, tc tenant.Ctx, in SetupHostedAuthInput) (*AuthConfig, error) {
	if s.zitadelMgmt == nil {
		return nil, fmt.Errorf("%w: ZITADEL management not configured", ErrValidation)
	}
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: encryption not configured", ErrValidation)
	}
	if in.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if len(in.RedirectURIs) == 0 {
		return nil, fmt.Errorf("%w: at least one redirect_uri is required", ErrValidation)
	}

	projectName := fmt.Sprintf("provider-%s-%s", tc.ProviderID, in.Name)
	projectID, err := s.zitadelMgmt.CreateProject(ctx, projectName)
	if err != nil {
		return nil, fmt.Errorf("create zitadel project: %w", err)
	}

	appName := projectName + "-oidc"
	appID, clientID, clientSecret, err := s.zitadelMgmt.CreateOIDCApp(ctx, projectID, appName, in.RedirectURIs)
	if err != nil {
		// Clean up the project if app creation failed.
		_ = s.zitadelMgmt.DeleteProject(ctx, projectID)
		return nil, fmt.Errorf("create zitadel oidc app: %w", err)
	}

	// Encrypt the client secret for storage.
	encSecret, err := s.encryptor.Encrypt(clientSecret)
	if err != nil {
		_ = s.zitadelMgmt.DeleteProject(ctx, projectID)
		return nil, fmt.Errorf("encrypt client secret: %w", err)
	}

	encRedirectURIs, err := json.Marshal(in.RedirectURIs)
	if err != nil {
		_ = s.zitadelMgmt.DeleteProject(ctx, projectID)
		return nil, fmt.Errorf("marshal redirect uris: %w", err)
	}

	var cfg storegen.ProviderAuthConfig
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Delete any existing config (re-setup replaces it).
		_ = q.DeleteAuthConfig(ctx, storegen.DeleteAuthConfigParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})

		c, err := q.CreateAuthConfig(ctx, storegen.CreateAuthConfigParams{
			ProviderID:          tc.ProviderID,
			EnvironmentID:       tc.EnvironmentID,
			Name:                in.Name,
			ZitadelProjectID:    projectID,
			ZitadelAppID:        appID,
			ZitadelClientID:     clientID,
			ZitadelClientSecret: encSecret,
			RedirectUris:        encRedirectURIs,
		})
		if err != nil {
			return mapErr(err, "create auth config")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "provider_auth_config", c.ID.String(), "auth.hosted_auth_setup", map[string]any{
			"project_id": projectID, "app_id": appID,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "auth.hosted_auth_setup",
			"provider_auth_config", c.ID.String(),
			map[string]any{"project_id": projectID, "app_id": appID}); err != nil {
			return err
		}
		cfg = c
		return nil
	})
	if err != nil {
		_ = s.zitadelMgmt.DeleteProject(ctx, projectID)
		return nil, err
	}

	return &AuthConfig{
		Config:    cfg,
		ClientID:  clientID,
		IssuerURL: s.zitadelIssuer,
	}, nil
}

// GetHostedAuthConfig returns the provider's Hosted Auth configuration.
// The client secret is NOT returned (encrypted at rest).
func (s *Service) GetHostedAuthConfig(ctx context.Context, tc tenant.Ctx) (*AuthConfig, error) {
	var cfg storegen.ProviderAuthConfig
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.GetAuthConfigByTenant(ctx, storegen.GetAuthConfigByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "auth config")
		}
		cfg = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &AuthConfig{
		Config:    cfg,
		ClientID:  cfg.ZitadelClientID,
		IssuerURL: s.zitadelIssuer,
	}, nil
}

// DisableHostedAuth removes the ZITADEL project and deletes the local config.
func (s *Service) DisableHostedAuth(ctx context.Context, tc tenant.Ctx) error {
	if s.zitadelMgmt == nil {
		return fmt.Errorf("%w: ZITADEL management not configured", ErrValidation)
	}

	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		cfg, err := q.GetAuthConfigByTenant(ctx, storegen.GetAuthConfigByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "auth config")
		}

		// Delete the ZITADEL project (removes all apps).
		if err := s.zitadelMgmt.DeleteProject(ctx, cfg.ZitadelProjectID); err != nil {
			return fmt.Errorf("delete zitadel project: %w", err)
		}

		// Delete the local config.
		if err := q.DeleteAuthConfig(ctx, storegen.DeleteAuthConfigParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		}); err != nil {
			return mapErr(err, "delete auth config")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "provider_auth_config", cfg.ID.String(), "auth.hosted_auth_disabled", nil); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "auth.hosted_auth_disable",
			"provider_auth_config", cfg.ID.String(), nil); err != nil {
			return err
		}
		return nil
	})
}

// ListHostedAuthConfigs returns all OIDC applications for the provider's
// tenant context. The client secrets are NOT returned (encrypted at rest).
func (s *Service) ListHostedAuthConfigs(ctx context.Context, tc tenant.Ctx) ([]storegen.ProviderAuthConfig, error) {
	var cfgs []storegen.ProviderAuthConfig
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.ListAuthConfigsByTenant(ctx, storegen.ListAuthConfigsByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "list auth configs")
		}
		cfgs = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cfgs, nil
}

// RotateHostedAuthSecret generates a new client secret on the ZITADEL
// side, re-encrypts it, and updates the local record. The plaintext
// secret is returned once (R17: keys shown only once).
func (s *Service) RotateHostedAuthSecret(ctx context.Context, tc tenant.Ctx) (*RotatedAuthConfig, error) {
	if s.zitadelMgmt == nil {
		return nil, fmt.Errorf("%w: ZITADEL management not configured", ErrValidation)
	}
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: encryption not configured", ErrValidation)
	}

	var cfg storegen.ProviderAuthConfig
	var newSecret string
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.GetAuthConfigByTenant(ctx, storegen.GetAuthConfigByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "auth config")
		}

		secret, err := s.zitadelMgmt.RotateOIDCAppSecret(ctx, c.ZitadelProjectID, c.ZitadelAppID)
		if err != nil {
			return fmt.Errorf("rotate zitadel secret: %w", err)
		}

		encSecret, err := s.encryptor.Encrypt(secret)
		if err != nil {
			return fmt.Errorf("encrypt client secret: %w", err)
		}

		updated, err := q.UpdateAuthConfigSecret(ctx, storegen.UpdateAuthConfigSecretParams{
			ProviderID:          tc.ProviderID,
			EnvironmentID:       tc.EnvironmentID,
			ZitadelClientSecret: encSecret,
		})
		if err != nil {
			return mapErr(err, "update auth config secret")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "provider_auth_config", updated.ID.String(), "auth.hosted_auth_secret_rotated", map[string]any{
			"project_id": updated.ZitadelProjectID, "app_id": updated.ZitadelAppID,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "auth.hosted_auth_secret_rotate",
			"provider_auth_config", updated.ID.String(),
			map[string]any{"project_id": updated.ZitadelProjectID, "app_id": updated.ZitadelAppID}); err != nil {
			return err
		}
		cfg = updated
		newSecret = secret
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &RotatedAuthConfig{
		Config:       cfg,
		ClientID:     cfg.ZitadelClientID,
		ClientSecret: newSecret,
		IssuerURL:    s.zitadelIssuer,
	}, nil
}

// RotatedAuthConfig is the response for RotateHostedAuthSecret, carrying
// the new plaintext client_secret (shown once).
type RotatedAuthConfig struct {
	Config       storegen.ProviderAuthConfig `json:"config"`
	ClientID     string                      `json:"client_id"`
	ClientSecret string                      `json:"client_secret"` // plaintext, shown once
	IssuerURL    string                      `json:"issuer_url"`
}

// UpdateHostedAuthRedirectURIsInput configures the new redirect URIs.
type UpdateHostedAuthRedirectURIsInput struct {
	RedirectURIs []string
}

// UpdateHostedAuthRedirectURIs updates the OIDC application's redirect
// URIs on both the ZITADEL side and the local mirror.
func (s *Service) UpdateHostedAuthRedirectURIs(ctx context.Context, tc tenant.Ctx, in UpdateHostedAuthRedirectURIsInput) (*AuthConfig, error) {
	if s.zitadelMgmt == nil {
		return nil, fmt.Errorf("%w: ZITADEL management not configured", ErrValidation)
	}
	if len(in.RedirectURIs) == 0 {
		return nil, fmt.Errorf("%w: at least one redirect_uri is required", ErrValidation)
	}

	var cfg storegen.ProviderAuthConfig
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.GetAuthConfigByTenant(ctx, storegen.GetAuthConfigByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "auth config")
		}

		if err := s.zitadelMgmt.UpdateOIDCAppRedirectURIs(ctx, c.ZitadelProjectID, c.ZitadelAppID, in.RedirectURIs); err != nil {
			return fmt.Errorf("update zitadel redirect uris: %w", err)
		}

		encURIs, err := json.Marshal(in.RedirectURIs)
		if err != nil {
			return fmt.Errorf("marshal redirect uris: %w", err)
		}

		updated, err := q.UpdateAuthConfigRedirectURIs(ctx, storegen.UpdateAuthConfigRedirectURIsParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			RedirectUris:  encURIs,
		})
		if err != nil {
			return mapErr(err, "update auth config redirect uris")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "provider_auth_config", updated.ID.String(), "auth.hosted_auth_redirect_uris_updated", map[string]any{
			"redirect_uris": in.RedirectURIs,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "auth.hosted_auth_redirect_uris_update",
			"provider_auth_config", updated.ID.String(),
			map[string]any{"redirect_uris": in.RedirectURIs}); err != nil {
			return err
		}
		cfg = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &AuthConfig{
		Config:    cfg,
		ClientID:  cfg.ZitadelClientID,
		IssuerURL: s.zitadelIssuer,
	}, nil
}

// WithZITADELManagement injects the ZITADEL Management API client and
// issuer URL for Hosted Auth project management.
func WithZITADELManagement(mgmt *zitadel.ManagementClient, issuerURL string) Option {
	return func(s *Service) {
		s.zitadelMgmt = mgmt
		s.zitadelIssuer = issuerURL
	}
}
