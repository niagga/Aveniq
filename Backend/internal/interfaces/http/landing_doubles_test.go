package http_test

import (
	"context"
	"errors"

	"github.com/niagga/aveniq/backend/internal/domain/landing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// testBaseDomain is the domain the practices' subdomains hang from in the harness: a practice
// with slug "practica-a" answers at practica-a.aveniq.test.
const testBaseDomain = "aveniq.test"

// fakeLandingRepository keeps one landing per practice. Like the policy it stands for, it
// refuses a write that names another practice than the scope's, and reads only the scope's rows.
type fakeLandingRepository struct {
	rows map[string]landing.Profile
}

func newFakeLandingRepository() *fakeLandingRepository {
	return &fakeLandingRepository{rows: map[string]landing.Profile{}}
}

func (r *fakeLandingRepository) Get(ctx context.Context) (landing.Profile, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return landing.Profile{}, landing.ErrTenantScopeMissing
	}
	row, found := r.rows[tenantID]
	if !found {
		return landing.Profile{}, landing.ErrNotFound
	}
	return row, nil
}

func (r *fakeLandingRepository) Save(ctx context.Context, subject landing.Profile) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return landing.ErrTenantScopeMissing
	}
	if string(subject.TenantID) != tenantID {
		return errors.New("row-level security: the row names another practice")
	}
	r.rows[tenantID] = subject
	return nil
}

// fakeTenantDomains is the host map: the custom domains, keyed by their normalized host.
type fakeTenantDomains struct {
	hosts map[string]tenant.ID
}

func (d *fakeTenantDomains) TenantIDByHost(_ context.Context, host string) (tenant.ID, error) {
	id, found := d.hosts[host]
	if !found {
		return "", tenant.ErrNotFound
	}
	return id, nil
}
