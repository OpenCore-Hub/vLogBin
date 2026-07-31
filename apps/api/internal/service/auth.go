package service

import (
	"context"
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

	var cfg storegen.ProviderAuthConfig
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Delete any existing config (re-setup replaces it).
		_ = q.DeleteAuthConfig(ctx, storegen.DeleteAuthConfigParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})

		c, err := q.CreateAuthConfig(ctx, storegen.CreateAuthConfigParams{
			ProviderID:           tc.ProviderID,
			EnvironmentID:        tc.EnvironmentID,
			ZitadelProjectID:     projectID,
			ZitadelAppID:         appID,
			ZitadelClientID:      clientID,
			ZitadelClientSecret:  encSecret,
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

// WithZITADELManagement injects the ZITADEL Management API client and
// issuer URL for Hosted Auth project management.
func WithZITADELManagement(mgmt *zitadel.ManagementClient, issuerURL string) Option {
	return func(s *Service) {
		s.zitadelMgmt = mgmt
		s.zitadelIssuer = issuerURL
	}
}
