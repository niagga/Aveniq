package tenantapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// PracticeStatusView is the little a session needs to know about its practice to behave: how
// long the trial runs, whether it has ended, and in which time zone the practice reads its days.
// It carries no name, slug or creation date: those belong to tenant:read, which not every
// role holds, and nothing here needs them.
type PracticeStatusView struct {
	Status      string
	Timezone    string
	TrialEndsAt string // RFC 3339, always UTC
	// Expired is the answer of the domain rule on the server's clock, the same one the write
	// guard applies. The screen uses it instead of comparing dates on the browser's clock.
	Expired bool
}

// PracticeStatusUseCase describes the state of the practice the session belongs to.
type PracticeStatusUseCase struct {
	repo  port.TenantRepository
	clock port.Clock
}

// NewPracticeStatusUseCase wires the use case.
func NewPracticeStatusUseCase(repo port.TenantRepository, clock port.Clock) *PracticeStatusUseCase {
	return &PracticeStatusUseCase{repo: repo, clock: clock}
}

// Execute reads the practice named by the verified token, or tenant.ErrNotFound.
func (uc *PracticeStatusUseCase) Execute(ctx context.Context, tenantID string) (PracticeStatusView, error) {
	subject, err := uc.repo.ByID(ctx, tenant.ID(tenantID))
	if err != nil {
		return PracticeStatusView{}, err
	}
	return PracticeStatusView{
		Status:      string(subject.Status),
		Timezone:    subject.Timezone,
		TrialEndsAt: subject.TrialEndsAt.UTC().Format(createdAtLayout),
		Expired:     subject.TrialExpired(uc.clock.Now()),
	}, nil
}
