package tenantapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// TrialExpiredUseCase answers whether a practice's trial has ended. The HTTP layer asks
// it before a write, so the rule lives in the domain and the clock is injected.
type TrialExpiredUseCase struct {
	repo  port.TenantRepository
	clock port.Clock
}

// NewTrialExpiredUseCase wires the use case.
func NewTrialExpiredUseCase(repo port.TenantRepository, clock port.Clock) *TrialExpiredUseCase {
	return &TrialExpiredUseCase{repo: repo, clock: clock}
}

// Execute reports whether the practice is expired. An unknown practice is an error, not
// "not expired": the caller must not let a write through on a practice it cannot find.
func (uc *TrialExpiredUseCase) Execute(ctx context.Context, tenantID string) (bool, error) {
	subject, err := uc.repo.ByID(ctx, tenant.ID(tenantID))
	if err != nil {
		return false, err
	}
	return subject.TrialExpired(uc.clock.Now()), nil
}
