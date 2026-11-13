package tenantapp

import "github.com/niagga/aveniq/backend/internal/domain/tenant"

// TenantView is the read model the application layer exposes to delivery. It is
// intentionally a flat, string-based shape: transports should not have to know
// domain types, and the domain should not have to know JSON.
type TenantView struct {
	ID        string
	Slug      string
	Name      string
	Status    string
	Timezone  string
	CreatedAt string // RFC 3339, always UTC
	// TrialEndsAt is the end of the trial, RFC 3339 in UTC. It is meaningful while
	// the status is trial and is reported for every status so the shape is stable.
	TrialEndsAt string
}

func toView(subject tenant.Tenant) TenantView {
	return TenantView{
		ID:          string(subject.ID),
		Slug:        subject.Slug,
		Name:        subject.Name,
		Status:      string(subject.Status),
		Timezone:    subject.Timezone,
		CreatedAt:   subject.CreatedAt.UTC().Format(createdAtLayout),
		TrialEndsAt: subject.TrialEndsAt.UTC().Format(createdAtLayout),
	}
}

const createdAtLayout = "2006-01-02T15:04:05Z07:00"
