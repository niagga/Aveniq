// Package tenantapp holds the tenant use cases: the application layer. It
// orchestrates domain rules and port contracts, and owns transaction-level
// decisions. It must never import infrastructure.
package tenantapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// CreateInput is the data a caller may choose. Anything the system must decide
// itself (id, status, timestamps) is deliberately absent.
type CreateInput struct {
	Slug string
	Name string
}

// CreateUseCase registers a new tenant.
type CreateUseCase struct {
	repo  port.TenantRepository
	clock port.Clock
	ids   port.IDGenerator
}

// NewCreateUseCase wires the use case through constructor injection: every
// dependency is explicit and replaceable, which is what keeps this layer testable
// without a database.
func NewCreateUseCase(repo port.TenantRepository, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{repo: repo, clock: clock, ids: ids}
}

// Execute creates the tenant or returns a domain error. Validation happens before
// persistence, so an invalid request never reaches the database.
func (uc *CreateUseCase) Execute(ctx context.Context, in CreateInput) (TenantView, error) {
	subject, err := tenant.New(tenant.ID(uc.ids.NewID()), in.Slug, in.Name, uc.clock.Now())
	if err != nil {
		return TenantView{}, err
	}

	if err := uc.repo.Create(ctx, subject); err != nil {
		return TenantView{}, err
	}

	return toView(subject), nil
}
