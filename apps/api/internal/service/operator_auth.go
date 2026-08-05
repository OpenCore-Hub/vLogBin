package service

import (
	"context"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ResolveProviderEnvironment returns the provider's environment for the given
// kind (test/live). The provider must exist; an unknown provider or a
// provider without the requested environment yields ErrNotFound so the
// Console surfaces a clear 404 instead of silently operating on the wrong
// tenant.
func (s *Service) ResolveProviderEnvironment(ctx context.Context, providerID uuid.UUID, kind string) (*storegen.Environment, error) {
	var env *storegen.Environment
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *storegen.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		envs, err := q.ListEnvironmentsByProvider(ctx, providerID)
		if err != nil {
			return err
		}
		for i := range envs {
			if envs[i].Kind == kind {
				env = &envs[i]
				return nil
			}
		}
		return fmt.Errorf("%w: environment %s", ErrNotFound, kind)
	})
	if err != nil {
		return nil, err
	}
	return env, nil
}

// OperatorAuthContext builds the tenant context used by the Console-facing
// operator control-plane auth endpoints. The environment comes from an
// explicit ?env= query parameter (never from a provider credential), and the
// provider must own the environment.
func OperatorAuthContext(providerID uuid.UUID, env *storegen.Environment) tenant.Ctx {
	return tenant.Ctx{
		ProviderID:      providerID,
		ProviderSlug:    "",
		EnvironmentID:   env.ID,
		EnvironmentKind: env.Kind,
		Issuer:          env.Issuer,
	}
}
