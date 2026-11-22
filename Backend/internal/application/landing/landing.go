// Package landingapp holds the use cases of the public landing: the read the public site makes
// by host, and the read and edit the practice's owner or admin makes. The edit is gated by
// tenant:settings at the transport; the public read is gated by the host and the rate limit.
package landingapp

import (
	"context"
	"errors"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/landing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// View is the landing as the API exposes it: flat strings, no domain types. Slug is the
// practice's subdomain label; it is public, because the address is.
type View struct {
	Slug        string
	DisplayName string
	Tagline     string
	About       string
	Address     string
	Phone       string
	Email       string
	Schedule    string
}

func toView(slug string, profile landing.Profile) View {
	return View{
		Slug:        slug,
		DisplayName: profile.DisplayName,
		Tagline:     profile.Tagline,
		About:       profile.About,
		Address:     profile.Address,
		Phone:       profile.Phone,
		Email:       profile.Email,
		Schedule:    profile.Schedule,
	}
}

// DraftInput is the text the owner writes. It is the body of the edit, with no identity in it.
type DraftInput struct {
	DisplayName string
	Tagline     string
	About       string
	Address     string
	Phone       string
	Email       string
	Schedule    string
}

func (in DraftInput) draft() landing.Draft {
	return landing.Draft{
		DisplayName: in.DisplayName,
		Tagline:     in.Tagline,
		About:       in.About,
		Address:     in.Address,
		Phone:       in.Phone,
		Email:       in.Email,
		Schedule:    in.Schedule,
	}
}

// PublicInput names the host the request arrived on.
type PublicInput struct {
	Host string
}

// PublicUseCase answers the landing of the practice that owns a host. It is the only read
// that runs without a session, so it reveals the least: a practice that is unknown, belongs to
// another domain, or is suspended answers the same ErrNotPublished.
type PublicUseCase struct {
	tenants    port.TenantRepository
	domains    port.TenantDomainRepository
	repo       port.LandingRepository
	scope      port.TenantScope
	baseDomain string
}

// NewPublicUseCase wires the public read. baseDomain is the domain the subdomains hang from.
func NewPublicUseCase(
	tenants port.TenantRepository,
	domains port.TenantDomainRepository,
	repo port.LandingRepository,
	scope port.TenantScope,
	baseDomain string,
) *PublicUseCase {
	return &PublicUseCase{tenants: tenants, domains: domains, repo: repo, scope: scope, baseDomain: baseDomain}
}

// Execute resolves the host to its practice and reads the landing inside that practice's scope.
// A practice that has not written a landing shows its own name and nothing else.
func (uc *PublicUseCase) Execute(ctx context.Context, in PublicInput) (View, error) {
	host, err := landing.NormalizeHost(in.Host)
	if err != nil {
		return View{}, err
	}

	subject, err := uc.resolve(ctx, host)
	if errors.Is(err, tenant.ErrNotFound) {
		return View{}, landing.ErrNotPublished
	}
	if err != nil {
		return View{}, err
	}
	if subject.Status == tenant.StatusSuspended {
		return View{}, landing.ErrNotPublished
	}

	var view View
	err = uc.scope.WithTenant(ctx, string(subject.ID), func(ctx context.Context) error {
		profile, err := uc.repo.Get(ctx)
		if errors.Is(err, landing.ErrNotFound) {
			profile = landing.Default(subject.ID, subject.Name)
		} else if err != nil {
			return err
		}
		view = toView(subject.Slug, profile)
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return view, nil
}

// resolve finds the practice of a host: the slug subdomain of the base domain first, then the
// custom domains the map holds. A host that is neither answers tenant.ErrNotFound.
func (uc *PublicUseCase) resolve(ctx context.Context, host string) (tenant.Tenant, error) {
	if slug, ok := landing.SubdomainOf(host, uc.baseDomain); ok {
		return uc.tenants.BySlug(ctx, slug)
	}

	id, err := uc.domains.TenantIDByHost(ctx, host)
	if err != nil {
		return tenant.Tenant{}, err
	}
	return uc.tenants.ByID(ctx, id)
}

// GetInput identifies the practice whose landing the owner or admin reads.
type GetInput struct {
	// TenantID comes from the verified access token, never from the request.
	TenantID string
}

// GetUseCase reads the landing of the caller's practice for the editor.
type GetUseCase struct {
	tenants port.TenantRepository
	repo    port.LandingRepository
	scope   port.TenantScope
}

// NewGetUseCase wires the editor read.
func NewGetUseCase(tenants port.TenantRepository, repo port.LandingRepository, scope port.TenantScope) *GetUseCase {
	return &GetUseCase{tenants: tenants, repo: repo, scope: scope}
}

// Execute returns the landing, or the default one when the practice has not written it yet.
func (uc *GetUseCase) Execute(ctx context.Context, in GetInput) (View, error) {
	subject, err := uc.tenants.ByID(ctx, tenant.ID(in.TenantID))
	if err != nil {
		return View{}, err
	}

	var view View
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		profile, err := uc.repo.Get(ctx)
		if errors.Is(err, landing.ErrNotFound) {
			profile = landing.Default(subject.ID, subject.Name)
		} else if err != nil {
			return err
		}
		view = toView(subject.Slug, profile)
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return view, nil
}

// UpdateInput is the owner's or admin's edit of the landing.
type UpdateInput struct {
	DraftInput
	// TenantID and ActorID come from the verified access token, never from the body.
	TenantID string
	ActorID  string
}

// UpdateUseCase replaces the text of the landing and records who changed it.
type UpdateUseCase struct {
	tenants port.TenantRepository
	repo    port.LandingRepository
	scope   port.TenantScope
	audit   port.AuditLog
	clock   port.Clock
}

// NewUpdateUseCase wires the edit.
func NewUpdateUseCase(
	tenants port.TenantRepository,
	repo port.LandingRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
) *UpdateUseCase {
	return &UpdateUseCase{tenants: tenants, repo: repo, scope: scope, audit: audit, clock: clock}
}

// Execute validates the draft in the domain, saves it, and writes the trail in the same
// transaction as the save: a landing that changed without a record is not possible.
func (uc *UpdateUseCase) Execute(ctx context.Context, in UpdateInput) (View, error) {
	subject, err := uc.tenants.ByID(ctx, tenant.ID(in.TenantID))
	if err != nil {
		return View{}, err
	}

	var view View
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.Get(ctx)
		if errors.Is(err, landing.ErrNotFound) {
			current = landing.Default(subject.ID, subject.Name)
		} else if err != nil {
			return err
		}

		updated, err := current.Change(in.draft(), uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Save(ctx, updated); err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.ActorID,
			Action:   "landing.updated",
			Entity:   "landing",
			EntityID: in.TenantID,
		}); err != nil {
			return err
		}

		view = toView(subject.Slug, updated)
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return view, nil
}
