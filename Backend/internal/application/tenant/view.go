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
	CreatedAt string // RFC 3339, always UTC
}

func toView(subject tenant.Tenant) TenantView {
	return TenantView{
		ID:        string(subject.ID),
		Slug:      subject.Slug,
		Name:      subject.Name,
		Status:    string(subject.Status),
		CreatedAt: subject.CreatedAt.UTC().Format(createdAtLayout),
	}
}

const createdAtLayout = "2006-01-02T15:04:05Z07:00"
