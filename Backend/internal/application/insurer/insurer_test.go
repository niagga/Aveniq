package insurerapp_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	insurerapp "github.com/niagga/aveniq/backend/internal/application/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
)

// ─── doubles ─────────────────────────────────────────────────────────────────

type fakeRepo struct {
	stored map[insurer.ID]insurer.Insurer
	// takenNames simulates the unique index: a name already used in the practice.
	takenNames map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{stored: map[insurer.ID]insurer.Insurer{}, takenNames: map[string]bool{}}
}

func (f *fakeRepo) Create(_ context.Context, subject insurer.Insurer) error {
	if f.takenNames[subject.Name] {
		return insurer.ErrNameTaken
	}
	f.takenNames[subject.Name] = true
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakeRepo) ByID(_ context.Context, id insurer.ID) (insurer.Insurer, error) {
	subject, ok := f.stored[id]
	if !ok {
		return insurer.Insurer{}, insurer.ErrNotFound
	}
	return subject, nil
}

func (f *fakeRepo) List(_ context.Context, activeOnly bool) ([]insurer.Insurer, error) {
	rows := []insurer.Insurer{}
	for _, subject := range f.stored {
		if activeOnly && !subject.IsActive {
			continue
		}
		rows = append(rows, subject)
	}
	return rows, nil
}

func (f *fakeRepo) Update(_ context.Context, subject insurer.Insurer) error {
	if _, ok := f.stored[subject.ID]; !ok {
		return insurer.ErrNotFound
	}
	f.stored[subject.ID] = subject
	return nil
}

// fakeScope records each opening so the tests can prove one scope per operation.
type fakeScope struct{ opened int }

func (s *fakeScope) WithTenant(ctx context.Context, _ string, fn func(ctx context.Context) error) error {
	s.opened++
	return fn(ctx)
}

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time {
	now := c.now
	c.now = c.now.Add(time.Minute)
	return now
}

type seqIDs struct{ n int }

func (s *seqIDs) NewID() string { s.n++; return fmt.Sprintf("insurer-%03d", s.n) }

const tenantID = "tenant-1"

func newWorld() (*fakeRepo, *fakeScope, *insurerapp.CreateUseCase, *insurerapp.UpdateUseCase, *insurerapp.LifecycleUseCase, *insurerapp.LifecycleUseCase, *insurerapp.ListUseCase) {
	repo := newFakeRepo()
	scope := &fakeScope{}
	clock := &fixedClock{now: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	ids := &seqIDs{}
	return repo, scope,
		insurerapp.NewCreateUseCase(repo, scope, clock, ids),
		insurerapp.NewUpdateUseCase(repo, scope, clock),
		insurerapp.NewDeactivateUseCase(repo, scope, clock),
		insurerapp.NewActivateUseCase(repo, scope, clock),
		insurerapp.NewListUseCase(repo, scope)
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestCreateReturnsTheMutualNormalized(t *testing.T) {
	_, scope, create, _, _, _, _ := newWorld()

	view, err := create.Execute(context.Background(), insurerapp.CreateInput{
		TenantID:     tenantID,
		ProfileInput: insurerapp.ProfileInput{Name: "  OSDE  ", TaxID: "30-12345678-9"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.Name != "OSDE" || view.TaxID != "30123456789" || !view.IsActive {
		t.Fatalf("view = %+v, want the trimmed name, the digits of the tax id and active", view)
	}
	if scope.opened != 1 {
		t.Fatalf("scope openings = %d, want exactly 1", scope.opened)
	}
}

func TestCreateWithoutTaxIDLeavesItAbsent(t *testing.T) {
	_, _, create, _, _, _, _ := newWorld()

	view, err := create.Execute(context.Background(), insurerapp.CreateInput{
		TenantID:     tenantID,
		ProfileInput: insurerapp.ProfileInput{Name: "Swiss Medical", TaxID: "   "},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.TaxID != "" {
		t.Fatalf("tax id = %q, want absent", view.TaxID)
	}
}

func TestCreateRefusesABlankNameWithAField(t *testing.T) {
	_, _, create, _, _, _, _ := newWorld()

	_, err := create.Execute(context.Background(), insurerapp.CreateInput{
		TenantID:     tenantID,
		ProfileInput: insurerapp.ProfileInput{Name: "   "},
	})
	var validation *insurer.ValidationError
	if !errors.As(err, &validation) || validation.Field != "name" {
		t.Fatalf("error = %v, want a ValidationError on name", err)
	}
}

func TestCreateRefusesADuplicateName(t *testing.T) {
	repo, _, create, _, _, _, _ := newWorld()
	repo.takenNames["OSDE"] = true

	_, err := create.Execute(context.Background(), insurerapp.CreateInput{
		TenantID:     tenantID,
		ProfileInput: insurerapp.ProfileInput{Name: "OSDE"},
	})
	if !errors.Is(err, insurer.ErrNameTaken) {
		t.Fatalf("error = %v, want insurer.ErrNameTaken", err)
	}
}

func TestEditingAnInactiveMutualIsRefused(t *testing.T) {
	_, _, create, update, deactivate, _, _ := newWorld()
	created, err := create.Execute(context.Background(), insurerapp.CreateInput{
		TenantID:     tenantID,
		ProfileInput: insurerapp.ProfileInput{Name: "OSDE"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := deactivate.Execute(context.Background(), insurerapp.LifecycleInput{TenantID: tenantID, ID: created.ID}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	// An inactive mutual must be reactivated before it can be edited.
	if _, err := update.Execute(context.Background(), insurerapp.UpdateInput{
		TenantID: tenantID, ID: created.ID, ProfileInput: insurerapp.ProfileInput{Name: "OSDE Binario"},
	}); !errors.Is(err, insurer.ErrInvalidTransition) {
		t.Fatalf("editing an inactive mutual = %v, want insurer.ErrInvalidTransition", err)
	}
}

func TestDeactivateThenActivateAndListFiltering(t *testing.T) {
	_, _, create, _, deactivate, activate, list := newWorld()
	created, err := create.Execute(context.Background(), insurerapp.CreateInput{
		TenantID: tenantID, ProfileInput: insurerapp.ProfileInput{Name: "OSDE"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	off, err := deactivate.Execute(context.Background(), insurerapp.LifecycleInput{TenantID: tenantID, ID: created.ID})
	if err != nil || off.IsActive {
		t.Fatalf("deactivate = (%+v, %v), want inactive", off, err)
	}
	active, err := list.Execute(context.Background(), insurerapp.ListInput{TenantID: tenantID, ActiveOnly: true})
	if err != nil || len(active) != 0 {
		t.Fatalf("active list = (%d, %v), want none", len(active), err)
	}
	all, err := list.Execute(context.Background(), insurerapp.ListInput{TenantID: tenantID})
	if err != nil || len(all) != 1 {
		t.Fatalf("full list = (%d, %v), want the inactive mutual too", len(all), err)
	}

	if _, err := activate.Execute(context.Background(), insurerapp.LifecycleInput{TenantID: tenantID, ID: created.ID}); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, err := activate.Execute(context.Background(), insurerapp.LifecycleInput{TenantID: tenantID, ID: created.ID}); !errors.Is(err, insurer.ErrInvalidTransition) {
		t.Fatalf("activating twice = %v, want insurer.ErrInvalidTransition", err)
	}
}

func TestLifecycleOfAMissingMutualIsNotFound(t *testing.T) {
	_, _, _, _, deactivate, _, _ := newWorld()
	if _, err := deactivate.Execute(context.Background(), insurerapp.LifecycleInput{TenantID: tenantID, ID: "insurer-missing"}); !errors.Is(err, insurer.ErrNotFound) {
		t.Fatalf("error = %v, want insurer.ErrNotFound", err)
	}
}
